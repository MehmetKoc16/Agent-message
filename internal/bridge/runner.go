package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/config"
)

var sessionRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const maxRunnerOutput = 4 << 20
const maxCodexPrompt = 200 << 10

var errPromptTooLong = errors.New("mesaj ajan için çok uzun")

type Options struct {
	Runner, Exe, Workdir, Model, Effort, Sandbox string
	Room, Agent, StatePath                       string
	DebugLog                                     string
	SandboxExplicit, Once                        bool
	// IncludePublic false iken yalnız köprü ajanına yönlü (to == Agent) mesajlar işlenir.
	// Herkese açık soruyu her oda üyesi yazabilir; köprüye dosya okutmanın yolu olmamalı.
	IncludePublic bool
	// AllowFrom boş değilse yalnız bu hesaplardan gelen mesajlar işlenir.
	AllowFrom []string
	MaxRuns   int
	Timeout   time.Duration
}

func (o *Options) prepare() error {
	for _, name := range o.AllowFrom {
		if !config.NameRe.MatchString(name) {
			return errors.New("--allow-from adları geçersiz (küçük harf, rakam, . _ -; virgülle ayrılır)")
		}
	}
	if o.Runner != "codex" && o.Runner != "agy" {
		return errors.New("--runner codex ya da agy olmalı")
	}
	if o.MaxRuns <= 0 || o.Timeout <= 0 {
		return errors.New("çalıştırma sınırı ve zaman aşımı pozitif olmalı")
	}
	if o.Runner == "agy" && o.SandboxExplicit {
		return errors.New("--sandbox yalnız Codex için kullanılabilir")
	}
	if o.Sandbox == "" {
		o.Sandbox = "read-only"
	}
	if o.Sandbox != "read-only" && o.Sandbox != "workspace-write" {
		return errors.New("--sandbox read-only ya da workspace-write olmalı")
	}
	if o.Model == "" {
		if o.Runner == "codex" {
			o.Model = "gpt-6.1-sol"
		} else {
			o.Model = "gemini-3.8-flash-medium"
		}
	}
	if o.Runner == "codex" {
		if o.Effort == "" {
			o.Effort = "medium"
		}
		switch o.Effort {
		case "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra":
		default:
			return errors.New("Codex eforu geçersiz")
		}
	}
	if o.Exe == "" {
		home, err := os.UserHomeDir() // standart API; ortam değişkenlerine doğrudan erişim yok
		if err != nil {
			return errors.New("çalıştırıcı için ev dizini bulunamadı; --exe kullanın")
		}
		o.Exe = discover(home, o.Runner)
		if o.Exe == "" {
			return fmt.Errorf("%s çalıştırıcısı standart kurulumda bulunamadı; --exe kullanın", o.Runner)
		}
	}
	var err error
	if o.Exe, err = filepath.Abs(o.Exe); err != nil {
		return errors.New("çalıştırıcı yolu geçersiz")
	}
	if st, err := os.Stat(o.Exe); err != nil || !st.Mode().IsRegular() {
		return errors.New("çalıştırıcı dosyası bulunamadı; --exe yolunu kontrol edin")
	}
	// cmd.exe, toplu iş dosyasının argümanlarını yeniden ayrıştırır: -p ile verilen
	// güvenilmeyen mesaj metni komut olarak çalışabilir. Codex istemi stdin'den alır.
	if o.Runner == "agy" {
		switch strings.ToLower(filepath.Ext(o.Exe)) {
		case ".bat", ".cmd":
			return errors.New("agy için .cmd/.bat çalıştırıcı kullanılamaz; agy.exe yolunu verin")
		}
	}
	if o.Workdir == "" {
		o.Workdir = "."
	}
	if o.Workdir, err = filepath.Abs(o.Workdir); err != nil {
		return errors.New("çalışma dizini geçersiz")
	}
	if st, err := os.Stat(o.Workdir); err != nil || !st.IsDir() {
		return errors.New("çalışma dizini bulunamadı")
	}
	return nil
}

func discover(home, runner string) string {
	base := filepath.Join(home, "AppData", "Local")
	if runner == "agy" {
		p := filepath.Join(base, "agy", "bin", "agy.exe")
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
		return ""
	}
	paths, _ := filepath.Glob(filepath.Join(base, "OpenAI", "Codex", "bin", "*", "codex.exe"))
	var newest time.Time
	var selected string
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && (selected == "" || st.ModTime().After(newest)) {
			selected, newest = p, st.ModTime()
		}
	}
	return selected
}

// arguments yalnız izinli seçenekleri üretir. Kabuk ve serbest ek bayrak alanı yoktur.
func arguments(o Options, session, output, prompt string) ([]string, error) {
	if session != "" && !sessionRE.MatchString(session) {
		return nil, errors.New("çalıştırıcı oturum kimliği geçersiz")
	}
	if o.Runner == "agy" {
		a := []string{"-p", prompt, "--model", o.Model, "--output-format", "json", "--print-timeout", fmt.Sprintf("%ds", max(1, int(o.Timeout/time.Second)))}
		if session != "" {
			a = append(a, "--conversation", session)
		}
		if !agyCommandFits(o.Exe, a) {
			return nil, errPromptTooLong
		}
		return a, nil
	}
	if len(prompt) > maxCodexPrompt {
		return nil, errPromptTooLong
	}
	a := []string{"exec"}
	if session != "" {
		a = append(a, "resume", session)
	}
	a = append(a, "--json", "-o", output, "-m", o.Model, "-c", `model_reasoning_effort="`+o.Effort+`"`)
	if session == "" {
		a = append(a, "--sandbox", o.Sandbox, "--skip-git-repo-check", "-C", o.Workdir)
	} else {
		a = append(a, "-c", `sandbox_mode="`+o.Sandbox+`"`, "--skip-git-repo-check")
	}
	return append(a, "-"), nil
}

type runResult struct {
	Session, Response string
	Duration          time.Duration
	ExitCode          int
	Stderr            string
}

// cappedOutput tüketmeye devam ederek pipe tıkanmasını önler, belleği sınırlı tutar.
type cappedOutput struct {
	buf      bytes.Buffer
	overflow bool
}

func (b *cappedOutput) Len() int      { return b.buf.Len() }
func (b *cappedOutput) Bytes() []byte { return b.buf.Bytes() }

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	left := maxRunnerOutput - b.Len()
	if n > left {
		b.overflow = true
		p = p[:left]
	}
	_, _ = b.buf.Write(p)
	return n, nil
}

func invoke(parent context.Context, o Options, session, prompt string) (result runResult, err error) {
	start := time.Now()
	result.ExitCode = -1
	var stderr stderrTail
	defer func() {
		result.Duration = time.Since(start)
		if err != nil && o.DebugLog != "" {
			result.Stderr = stderr.complete()
		}
	}()
	var output string
	if o.Runner == "codex" {
		f, e := os.CreateTemp("", "relay-bridge-*.txt")
		if e != nil {
			return result, errors.New("ajan çıktı dosyası oluşturulamadı")
		}
		output = f.Name()
		defer os.Remove(output)
		if f.Close() != nil {
			return result, errors.New("ajan çıktı dosyası kapatılamadı")
		}
	}
	a, err := arguments(o, session, output, prompt)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(parent, o.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.Exe, a...)
	cmd.Dir = o.Workdir // resume -C desteklemese de aynı çalışma dizini
	// Env nil: ebeveyn ortamı aynen geçer; okunmaz ve değiştirilmez.
	if o.Runner == "codex" {
		cmd.Stdin = strings.NewReader(prompt) // EOF'ta pipe kapanır
	} else {
		f, e := os.Open(os.DevNull)
		if e != nil {
			return result, errors.New("boş stdin açılamadı")
		}
		defer f.Close()
		cmd.Stdin = f
	}
	var stdout cappedOutput
	cmd.Stdout, cmd.Stderr = &stdout, io.Discard
	if o.DebugLog != "" {
		cmd.Stderr = &stderr
	}
	// Çocukların açık tuttuğu pipe'lar nedeniyle Wait sonsuza kadar asılı kalmasın.
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		if parent.Err() != nil {
			return result, parent.Err()
		}
		if ctx.Err() != nil {
			return result, errors.New("ajan zaman aşımına uğradı")
		}
		return result, errors.New("ajan çalıştırılamadı")
	}
	if stdout.overflow {
		return result, errors.New("ajan çıktısı sınırı aşıyor")
	}
	if o.Runner == "agy" {
		var v struct {
			Session  string `json:"conversation_id"`
			Status   string `json:"status"`
			Response string `json:"response"`
		}
		if json.Unmarshal(stdout.Bytes(), &v) != nil || v.Status != "SUCCESS" || !sessionRE.MatchString(v.Session) {
			return result, errors.New("agy yanıtı geçersiz ya da başarısız")
		}
		result.Session, result.Response = v.Session, v.Response
		return result, nil
	}
	result.Session = session
	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for {
		var v struct {
			Type    string `json:"type"`
			Session string `json:"thread_id"`
			Item    struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if e := dec.Decode(&v); e == io.EOF {
			break
		} else if e != nil {
			return result, errors.New("Codex JSONL yanıtı geçersiz")
		}
		if v.Type == "thread.started" {
			result.Session = v.Session
		}
		if v.Type == "item.completed" && v.Item.Type == "agent_message" {
			result.Response = v.Item.Text
		}
	}
	if !sessionRE.MatchString(result.Session) {
		return result, errors.New("Codex oturum kimliği bulunamadı")
	}
	f, e := os.Open(output)
	if e == nil {
		b, readErr := io.ReadAll(io.LimitReader(f, maxRunnerOutput+1))
		f.Close()
		if readErr != nil || len(b) > maxRunnerOutput {
			return result, errors.New("ajan son cevabı okunamadı ya da çok büyük")
		}
		if len(b) > 0 {
			result.Response = string(b)
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, errors.New("ajan son cevabı okunamadı")
	}
	return result, nil
}

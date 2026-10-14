package client

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/flagx"
	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

// Çıkış kodları (Faz 0 ile aynı).
const (
	ExitOK          = 0   // mesaj geldi / işlem tamam
	ExitError       = 1   // hata, kilitli konuşma, kullanım hatası
	ExitTimeout     = 2   // zaman aşımı: yeni mesaj yok
	ExitUnreachable = 3   // sunucuya ulaşılamıyor
	ExitSecret      = 4   // sır taraması gönderimi engelledi
	ExitCanceled    = 130 // kullanıcı iptali (Ctrl+C)
)

// Testlerin kısaltabilmesi için değişkendir.
var (
	pollSeconds = 50
	retryDelay  = 2 * time.Second
	maxStdin    = int64(1 << 20)
)

// Env komutun dış dünyasıdır.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// DefaultConfig --config verilmediğinde kullanılacak yol ("" ise ~/.agent-relay/config.json).
	DefaultConfig string
}

const usage = `kullanım: relay [--config <json>] <komut>
  setup --url <URL> --room <oda> --agent <ad> [--config <yol>] [--force] [--no-check]
  version                   sürüm, commit ve Go sürümü
  send --type <tip> [--to <ajan>] [--thread <T-n>] [--reply-to <id>] <metin|->
  wait [--timeout 600]      yeni mesaj gelene kadar bekler
  inbox                     okunmamış mesajları basar
  agents                    odadaki ajanlar
  thread <T-n>              bir konuşmanın geçmişi
  close <T-n>               konuşmayı kapatır
çıkış kodları: 0 mesaj, 1 hata/kilitli, 2 zaman aşımı, 3 ulaşılamıyor, 4 sır taraması`

type runner struct {
	env Env
	cfg *Config
	api *api
	cur cursor
}

// Run bir relay komutunu çalıştırır ve çıkış kodunu döndürür.
func Run(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("relay", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	cfgPath := fs.String("config", env.DefaultConfig, "ayar dosyası")
	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(env.Stderr, usage)
		return ExitError
	}
	cmd, rest := fs.Arg(0), fs.Args()[1:]
	switch cmd {
	case "setup":
		return runSetup(ctx, rest, env, *cfgPath)
	case "version":
		return runVersion(rest, env)
	case "send", "wait", "inbox", "agents", "thread", "close":
	default:
		fmt.Fprintf(env.Stderr, "bilinmeyen komut: %s\n%s\n", cmd, usage)
		return ExitError
	}
	path := *cfgPath
	if path == "" {
		p, err := DefaultConfigPath()
		if err != nil {
			fmt.Fprintln(env.Stderr, "hata:", err)
			return ExitError
		}
		path = p
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		fmt.Fprintln(env.Stderr, "hata:", err)
		return ExitError
	}
	r := &runner{env: env, cfg: cfg, api: newAPI(cfg), cur: cursor{path: cfg.statePath}}

	cfs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	cfs.SetOutput(env.Stderr)
	switch cmd {
	case "send":
		return r.cmdSend(ctx, cfs, rest)
	case "wait":
		return r.cmdWait(ctx, cfs, rest)
	case "inbox":
		return r.cmdInbox(ctx, cfs, rest)
	case "agents":
		return r.cmdAgents(ctx, cfs, rest)
	case "thread":
		return r.cmdThread(ctx, cfs, rest)
	default:
		return r.cmdClose(ctx, cfs, rest)
	}
}

func (r *runner) fail(code int, format string, a ...any) int {
	fmt.Fprintf(r.env.Stderr, format+"\n", a...)
	return code
}

// apiFail bir API/ağ hatasını kullanıcıya anlaşılır yazar ve çıkış kodunu verir.
func (r *runner) apiFail(err error) int {
	if errors.Is(err, ErrCanceled) {
		return r.fail(ExitCanceled, "iptal edildi")
	}
	if errors.Is(err, ErrUnreachable) {
		return r.fail(ExitUnreachable, "sunucuya ulaşılamadı")
	}
	if errors.Is(err, ErrUnavailable) {
		return r.fail(ExitUnreachable, "sunucu geçici olarak yanıt vermiyor")
	}
	var ae *APIError
	if errors.As(err, &ae) {
		if ae.Code == "gecersiz_yanit" {
			return r.fail(ExitError, "sunucu geçersiz yanıt verdi")
		}
		switch ae.Status {
		case 401:
			return r.fail(ExitError, "yetkisiz: token geçersiz ya da iptal edilmiş")
		case 403:
			return r.fail(ExitError, "yasak: bu oda ya da işlem için yetkiniz yok")
		case 404:
			return r.fail(ExitError, "bulunamadı: konuşma yok")
		case 409:
			return r.fail(ExitError, "konuşma kilitli: tur sınırına ulaşıldı ya da konuşma kapatıldı")
		case 413:
			return r.fail(ExitError, "mesaj çok büyük")
		case 429:
			return r.fail(ExitError, "çok fazla istek: hız sınırı, biraz sonra deneyin")
		case 400:
			return r.fail(ExitError, "sunucu isteği reddetti (%s)", ae.Code)
		}
		return r.fail(ExitError, "sunucu hatası: HTTP %d", ae.Status)
	}
	return r.fail(ExitError, "hata: %v", err)
}

func (r *runner) cmdSend(ctx context.Context, fs *flag.FlagSet, args []string) int {
	typ := fs.String("type", "", "soru|gorev|cevap|sonuc|bilgi|kapat")
	to := fs.String("to", "", "alıcı ajan (boş: odadaki herkes)")
	thread := fs.String("thread", "", "konuşma kimliği")
	replyTo := fs.Int64("reply-to", 0, "yanıtlanan mesaj id'si")
	pos, err := flagx.Parse(fs, args)
	if err != nil {
		return ExitError
	}
	if *typ == "" || len(pos) != 1 {
		return r.fail(ExitError, "kullanım: relay send --type <tip> [--to ..] [--thread ..] [--reply-to ..] <metin|->")
	}
	text := pos[0]
	if text == "-" {
		b, err := io.ReadAll(io.LimitReader(r.env.Stdin, maxStdin+1))
		if err != nil {
			return r.fail(ExitError, "stdin okunamadı")
		}
		if int64(len(b)) > maxStdin {
			return r.fail(ExitError, "mesaj çok büyük")
		}
		text = strings.ToValidUTF8(string(b), "�")
	}
	if name, found := ScanSecrets(text); found {
		return r.fail(ExitSecret, "GÖNDERİLMEDİ: mesajda sır deseni bulundu (%s). Sırları mesaja yazmayın.", name)
	}
	in := sendInput{To: *to, Type: *typ, Thread: *thread, Body: text}
	if *replyTo > 0 {
		in.ReplyTo = replyTo
	}
	m, err := r.api.send(ctx, in)
	if err != nil {
		return r.apiFail(err)
	}
	fmt.Fprintf(r.env.Stdout, "gönderildi id=%d thread=%s turn=%d/%d\n", m.ID, m.Thread, m.Turn, m.MaxTurns)
	return ExitOK
}

// show çerçeveli mesajları basar, sonra imleci en büyük id'ye ilerletir.
func (r *runner) show(msgs []proto.Message) int {
	parts := make([]string, len(msgs))
	var maxID int64
	for i, m := range msgs {
		parts[i] = Render(m)
		maxID = max(maxID, m.ID)
	}
	fmt.Fprintln(r.env.Stdout, strings.Join(parts, "\n"))
	fmt.Fprintln(r.env.Stdout, Reminder)
	if err := r.cur.Advance(maxID); err != nil {
		return r.fail(ExitError, "hata: %v", err)
	}
	return ExitOK
}

func (r *runner) fetchOnce(ctx context.Context, wait int) ([]proto.Message, error) {
	after, err := r.cur.Read()
	if err != nil {
		return nil, err
	}
	return r.api.fetch(ctx, after, wait)
}

func (r *runner) cmdInbox(ctx context.Context, fs *flag.FlagSet, args []string) int {
	if _, err := flagx.Parse(fs, args); err != nil {
		return ExitError
	}
	msgs, err := r.fetchOnce(ctx, 0)
	if err != nil {
		return r.fetchFail(err)
	}
	if len(msgs) == 0 {
		fmt.Fprintln(r.env.Stdout, "yeni mesaj yok")
		return ExitOK
	}
	return r.show(msgs)
}

// fetchFail imleç hatasını (bozuk dosya) ve API hatalarını ayırır.
func (r *runner) fetchFail(err error) int {
	var ae *APIError
	if isTransient(err) || errors.Is(err, ErrCanceled) || errors.As(err, &ae) {
		return r.apiFail(err)
	}
	return r.fail(ExitError, "hata: %v", err)
}

func (r *runner) cmdWait(ctx context.Context, fs *flag.FlagSet, args []string) int {
	timeout := fs.Float64("timeout", 600, "en fazla bekleme (sn); <=0: tek beklemesiz istek")
	if _, err := flagx.Parse(fs, args); err != nil {
		return ExitError
	}
	if _, err := r.cur.Read(); err != nil { // bozuk imleç ağdan önce yakalanır
		return r.fail(ExitError, "hata: %v", err)
	}
	if *timeout <= 0 {
		msgs, err := r.fetchOnce(ctx, 0)
		if err != nil {
			return r.fetchFail(err)
		}
		if len(msgs) > 0 {
			return r.show(msgs)
		}
		return r.fail(ExitTimeout, "zaman aşımı: yeni mesaj yok")
	}
	deadline := time.Now().Add(time.Duration(*timeout * float64(time.Second)))
	lastOK := false
	for ctx.Err() == nil {
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		wait := max(1, min(pollSeconds, int(left/time.Second)))
		msgs, err := r.fetchOnce(ctx, wait)
		if err == nil {
			lastOK = true
			if len(msgs) > 0 {
				return r.show(msgs)
			}
			continue
		}
		if errors.Is(err, ErrCanceled) {
			return r.apiFail(err)
		}
		if !isTransient(err) {
			return r.fetchFail(err) // 401, 403, bozuk imleç...: yeniden denemenin anlamı yok
		}
		lastOK = false
		select {
		case <-time.After(min(retryDelay, max(0, time.Until(deadline)))):
		case <-ctx.Done():
		}
	}
	if ctx.Err() != nil {
		return r.fail(ExitCanceled, "iptal edildi")
	}
	if !lastOK {
		return r.fail(ExitUnreachable, "sunucuya ulaşılamadı")
	}
	return r.fail(ExitTimeout, "zaman aşımı: yeni mesaj yok")
}

func (r *runner) cmdAgents(ctx context.Context, fs *flag.FlagSet, args []string) int {
	if _, err := flagx.Parse(fs, args); err != nil {
		return ExitError
	}
	list, err := r.api.agents(ctx)
	if err != nil {
		return r.apiFail(err)
	}
	tw := tabwriter.NewWriter(r.env.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "AD\tTÜR\tSON_GÖRÜLME\tBEKLİYOR")
	for _, a := range list {
		seen, waiting := a.LastSeen, "hayır"
		if seen == "" {
			seen = "-"
		}
		if a.Waiting {
			waiting = "evet"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", sanitize(a.Name), sanitize(a.Kind), sanitize(seen), waiting)
	}
	tw.Flush()
	return ExitOK
}

// sanitize tablo çıktısına giren sunucu metninden kontrol karakterlerini atar.
func sanitize(s string) string {
	return strings.Map(func(c rune) rune {
		if c < 0x20 || c == 0x7f {
			return -1
		}
		return c
	}, s)
}

func (r *runner) cmdThread(ctx context.Context, fs *flag.FlagSet, args []string) int {
	pos, err := flagx.Parse(fs, args)
	if err != nil {
		return ExitError
	}
	if len(pos) != 1 {
		return r.fail(ExitError, "kullanım: relay thread <T-n>")
	}
	t, err := r.api.thread(ctx, pos[0])
	if err != nil {
		return r.apiFail(err)
	}
	fmt.Fprintf(r.env.Stdout, "konuşma %s durum=%s tur=%d/%d\n", sanitize(t.ID), sanitize(t.Status), t.Turns, t.TurnLimit)
	if len(t.Messages) == 0 {
		fmt.Fprintln(r.env.Stdout, "görebildiğiniz mesaj yok")
		return ExitOK
	}
	parts := make([]string, len(t.Messages))
	for i, m := range t.Messages {
		parts[i] = Render(m)
	}
	fmt.Fprintln(r.env.Stdout, strings.Join(parts, "\n"))
	fmt.Fprintln(r.env.Stdout, Reminder)
	return ExitOK
}

func (r *runner) cmdClose(ctx context.Context, fs *flag.FlagSet, args []string) int {
	pos, err := flagx.Parse(fs, args)
	if err != nil {
		return ExitError
	}
	if len(pos) != 1 {
		return r.fail(ExitError, "kullanım: relay close <T-n>")
	}
	if err := r.api.closeThread(ctx, pos[0]); err != nil {
		return r.apiFail(err)
	}
	fmt.Fprintf(r.env.Stdout, "kapatıldı: %s\n", pos[0])
	return ExitOK
}

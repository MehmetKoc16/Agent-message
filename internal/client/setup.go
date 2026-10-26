package client

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MehmetKoc16/Agent-message/internal/flagx"
)

var (
	setupNameRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	setupTokenRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{20,200}$`)
	errConfigExists = errors.New("ayar dosyası zaten var; üzerine yazmak için --force kullanın")
)

func runSetup(ctx context.Context, args []string, env Env, path string) int {
	r := &runner{env: env}
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	url := fs.String("url", "", "sunucu URL'si")
	room := fs.String("room", "", "oda adı")
	agent := fs.String("agent", "", "ajan adı")
	configPath := fs.String("config", path, "ayar dosyası")
	force := fs.Bool("force", false, "mevcut ayarın üzerine yaz")
	noCheck := fs.Bool("no-check", false, "bağlantıyı denetleme")
	pos, err := flagx.Parse(fs, args)
	if err != nil {
		return ExitError
	}
	if len(pos) != 0 || *url == "" || *room == "" || *agent == "" {
		return r.fail(ExitError, "kullanım: relay setup --url <URL> --room <oda> --agent <ad> [--config <yol>] [--force] [--no-check]")
	}
	checkedURL, err := checkURL(*url)
	if err != nil {
		return r.fail(ExitError, "hata: %v", err)
	}
	if !setupNameRe.MatchString(*room) || !setupNameRe.MatchString(*agent) {
		return r.fail(ExitError, "room ve agent adları geçersiz (1–64 karakter: küçük harf, rakam, . _ -; ilk karakter harf ya da rakam)")
	}
	path = *configPath
	if path == "" {
		path, err = DefaultConfigPath()
		if err != nil {
			return r.fail(ExitError, "ev dizini bulunamadı")
		}
	}
	if !*force {
		if err := setupDestination(path); err != nil {
			return r.fail(ExitError, "hata: %v", err)
		}
	}
	token, err := readSetupToken(ctx, env)
	if err != nil {
		if errors.Is(err, ErrCanceled) {
			return r.apiFail(err)
		}
		return r.fail(ExitError, "hata: %v", err)
	}
	if token == "" {
		return r.fail(ExitError, "token boş olamaz")
	}
	if !setupTokenRe.MatchString(token) {
		return r.fail(ExitError, "token geçersiz: 20–200 base64url karakteri (harf, rakam, _ ve -) olmalı")
	}
	if ctx.Err() != nil {
		return r.apiFail(ErrCanceled)
	}
	cfg := &Config{URL: checkedURL, Room: *room, Agent: *agent, Token: token}
	backup, err := writeSetupConfig(path, cfg, *force, *force && !*noCheck)
	if err != nil {
		return r.fail(ExitError, "hata: %v", err)
	}
	// Yedek yalnız 401'de geri konur; diğer her sonuçta yeni ayar kalır, yedek silinir.
	keepBackup := false
	defer func() {
		if backup != "" && !keepBackup {
			os.Remove(backup)
		}
	}()
	if *noCheck {
		fmt.Fprintln(env.Stdout, "ayar dosyası yazıldı")
		return ExitOK
	}
	r.api = newAPI(cfg)
	defer r.api.hc.CloseIdleConnections()
	list, err := r.api.agents(ctx)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == 401 {
			if backup != "" {
				// --force yanlış token'la çalışan eski ayarı silmemeli.
				if err := renameRetry(backup, path); err != nil {
					keepBackup = true // elle geri yüklenebilsin diye yedek bırakılır
					return r.fail(ExitError, "token geçersiz; önceki ayar geri yüklenemedi, yedek dosyası: %s", backup)
				}
				return r.fail(ExitError, "token geçersiz; önceki ayar geri yüklendi")
			}
			if err := os.Remove(path); err != nil {
				return r.fail(ExitError, "token geçersiz; ayar dosyası silinemedi, elle silin")
			}
			return r.fail(ExitError, "token geçersiz; ayar dosyası silindi")
		}
		if isTransient(err) {
			return r.fail(ExitUnreachable, "uyarı: sunucuya ulaşılamadı; ayar dosyası bırakıldı, relay agents ile yeniden deneyin")
		}
		// Sunucunun hata kodu token içeriyor olabilir; setup bunu çıktıya taşımaz.
		if errors.As(err, &ae) && ae.Code != "gecersiz_yanit" {
			ae.Code = ""
		}
		return r.apiFail(err)
	}
	fmt.Fprintf(env.Stdout, "bağlantı tamam: %s odasında %d ajan\n", cfg.Room, len(list))
	return ExitOK
}

func setupDestination(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return errConfigExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("ayar dosyası denetlenemedi")
	}
	return nil
}

// writeSetupConfig ayarı atomik yazar. withBackup ise mevcut normal dosyanın 0600
// kopyası aynı dizinde alınır ve yolu döner; bağlantı denetimi 401 verirse geri konur.
func writeSetupConfig(path string, cfg *Config, force, withBackup bool) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errors.New("ayar klasörü oluşturulamadı")
	}
	// İki setup'ın aynı dosyayı eşzamanlı değiştirmesini engeller.
	lock, err := os.OpenFile(path+".setup.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", errors.New("ayar kilidi alınamadı; başka bir setup çalışıyor olabilir")
	}
	lock.Close()
	defer os.Remove(path + ".setup.lock")
	if !force {
		if err := setupDestination(path); err != nil {
			return "", err
		}
	}
	var backup string
	if withBackup {
		if backup, err = backupSetupConfig(path); err != nil {
			return "", err
		}
	}
	tmp, err := os.CreateTemp(dir, ".relay-*.tmp")
	if err != nil {
		if backup != "" {
			os.Remove(backup)
		}
		return "", errors.New("geçici ayar dosyası oluşturulamadı")
	}
	defer os.Remove(tmp.Name())
	data, _ := json.MarshalIndent(cfg, "", "  ")
	data = append(data, '\n')
	err = tmp.Chmod(0o600)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = renameRetry(tmp.Name(), path)
	}
	if err != nil {
		if backup != "" {
			os.Remove(backup)
		}
		return "", errors.New("ayar dosyası yazılamadı")
	}
	return backup, nil
}

// backupSetupConfig mevcut normal ayar dosyasının birebir kopyasını 0600 geçici dosyaya
// alır. Dosya yoksa ya da normal dosya değilse yedek alınmaz ("" döner).
func backupSetupConfig(path string) (string, error) {
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !st.Mode().IsRegular()) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("ayar dosyası denetlenemedi")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("önceki ayar yedeklenemedi")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".relay-yedek-*.tmp")
	if err != nil {
		return "", errors.New("önceki ayar yedeklenemedi")
	}
	err = tmp.Chmod(0o600)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", errors.New("önceki ayar yedeklenemedi")
	}
	return tmp.Name(), nil
}

func readSetupToken(ctx context.Context, env Env) (string, error) {
	if ctx.Err() != nil {
		return "", ErrCanceled
	}
	restore := func() error { return nil }
	terminal := false
	if f, ok := env.Stdin.(*os.File); ok {
		var err error
		terminal, restore, err = setupEcho(f)
		if err != nil && terminal {
			fmt.Fprintln(env.Stderr, "uyarı: terminal yansıtması kapatılamadı; token ekranda görünebilir")
		}
	}
	defer func() {
		if err := restore(); err != nil {
			fmt.Fprintln(env.Stderr, "uyarı: terminal modu geri yüklenemedi")
		}
		if terminal {
			fmt.Fprintln(env.Stderr)
		}
	}()
	if terminal {
		fmt.Fprint(env.Stderr, "Token: ")
	}
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	// Konsol Read'i satır beklerken Ctrl+C context'i iptal eder. Run geri döner,
	// defer modu geri yükler; main süreçten çıkar (bekleyen okuma süreçle biter).
	go func() {
		text, err := readTokenLine(env.Stdin)
		done <- result{text, err}
	}()
	select {
	case <-ctx.Done():
		return "", ErrCanceled
	case got := <-done:
		if ctx.Err() != nil {
			return "", ErrCanceled
		}
		return strings.TrimSpace(got.text), got.err
	}
}

// Bir satır okur; ikinci satırı tüketmez ve sınırsız girdiyi belleğe almaz.
func readTokenLine(in io.Reader) (string, error) {
	var line strings.Builder
	var one [1]byte
	for line.Len() <= 4096 {
		n, err := in.Read(one[:])
		if n > 0 {
			if one[0] == '\n' {
				return line.String(), nil
			}
			line.WriteByte(one[0])
		}
		if errors.Is(err, io.EOF) {
			return line.String(), nil
		}
		if err != nil {
			return "", errors.New("stdin okunamadı")
		}
	}
	return "", errors.New("token satırı çok uzun")
}

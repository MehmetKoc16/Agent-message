// Package client, `relay` komut satırı istemcisidir.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MehmetKoc16/Agent-message/internal/config"
)

// Config istemci ayarıdır. Token yalnız bu dosyadan okunur; argüman ya da
// ortam değişkeninden asla alınmaz.
type Config struct {
	URL   string `json:"url"`
	Room  string `json:"room"`
	Agent string `json:"agent"`
	Token string `json:"token"`

	statePath       string
	legacyStatePath string // '-' ayırıcılı eski ad; yalnız geçiş için
}

// DefaultConfigPath ~/.agent-relay/config.json yoludur.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ev dizini bulunamadı: %w", err)
	}
	return filepath.Join(home, ".agent-relay", "config.json"), nil
}

// LoadConfig ayar dosyasını okur ve doğrular.
func LoadConfig(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("ayar dosyası okunamadı: %s", abs)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("ayar dosyası normal bir dosya olmalı")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("token dosyası yalnız sahibi tarafından erişilebilir olmalı; chmod 600 ile izinleri düzeltin")
	}
	var c Config
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("ayar dosyası bozuk: %s (%v)", abs, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("ayar dosyası bozuk: %s (fazladan veri)", abs)
	}
	for k, v := range map[string]string{"url": c.URL, "room": c.Room, "agent": c.Agent, "token": c.Token} {
		if v == "" {
			return nil, fmt.Errorf("ayar dosyasında '%s' eksik: %s", k, abs)
		}
	}
	if !config.NameRe.MatchString(c.Room) || !config.NameRe.MatchString(c.Agent) {
		return nil, errors.New("room ve agent adları geçersiz (küçük harf, rakam, . _ -)")
	}
	if c.URL, err = checkURL(c.URL); err != nil {
		return nil, err
	}
	c.statePath, c.legacyStatePath = stateFileNames(filepath.Dir(abs), "state", c.Room, c.Agent)
	return &c, nil
}

// stateFileNames yeni ve eski durum dosyası adlarını üretir. Adlarda '-' izinli
// olduğundan eski ayırıcı ("a-b"+"c" ile "a"+"b-c") çakışıyordu; '@' adlarda
// izinli değildir ve Windows dosya adlarında da geçerlidir.
func stateFileNames(dir, prefix string, parts ...string) (string, string) {
	return filepath.Join(dir, prefix+"@"+strings.Join(parts, "@")+".json"),
		filepath.Join(dir, prefix+"-"+strings.Join(parts, "-")+".json")
}

// migrateLegacy eski adlı durum dosyasını yeni ada taşır; mevcut kurulumların imleci
// ve köprü oturumları sıfırlanmaz. Yeni dosya varsa dokunulmaz. Taşıma başarısızsa
// (ör. Windows'ta dosya açık) içerik kopyalanır. Kopyalanamıyorsa sessizce sıfırdan
// başlamak eski mesajları yeniden işleteceği için hata döner.
func migrateLegacy(oldPath, newPath string) error {
	if _, err := os.Lstat(newPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("durum dosyası denetlenemedi: %s", newPath)
	}
	st, err := os.Lstat(oldPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("eski durum dosyası taşınamadı: %s", oldPath)
	}
	if os.Rename(oldPath, newPath) == nil {
		return nil
	}
	if _, err := os.Lstat(newPath); err == nil {
		return nil // eşzamanlı başka bir relay süreci taşıdı
	}
	b, err := os.ReadFile(oldPath)
	if err == nil {
		err = writePrivateFile(newPath, b)
	}
	if err != nil {
		if _, serr := os.Lstat(newPath); serr == nil {
			return nil
		}
		return fmt.Errorf("eski durum dosyası taşınamadı: %s", oldPath)
	}
	return nil
}

// writePrivateFile içeriği aynı dizinde 0600 geçici dosyaya yazıp atomik olarak yerine koyar.
func writePrivateFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = tmp.Chmod(0o600)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = renameRetry(tmp.Name(), path)
	}
	return err
}

// checkURL https zorunlu; düz http yalnız döngü adresi (localhost) için kabul edilir,
// çünkü token düz metin gitmemeli.
func checkURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("url geçersiz")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("url kullanıcı bilgisi, sorgu ya da parça içeremez")
	}
	switch u.Scheme {
	case "https":
	case "http":
		switch u.Hostname() {
		case "127.0.0.1", "localhost", "::1":
		default:
			return "", errors.New("url https olmalı (düz http yalnız 127.0.0.1/localhost/::1 için)")
		}
	default:
		return "", errors.New("url https olmalı")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

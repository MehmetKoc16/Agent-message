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

	statePath string
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
	c.statePath = filepath.Join(filepath.Dir(abs), "state-"+c.Room+"-"+c.Agent+".json")
	return &c, nil
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

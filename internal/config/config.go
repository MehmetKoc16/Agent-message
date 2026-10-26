// Package config, sunucunun JSON ayar dosyasını okur ve doğrular.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// NameRe ajan, oda ve benzeri adların tek geçerli biçimidir.
var NameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Server sunucu ayarlarıdır. Bilinmeyen alan hata sayılır.
type Server struct {
	Listen             string `json:"listen"`
	TrustForwardedFor  bool   `json:"trust_forwarded_for"`
	UnauthPerMin       int    `json:"unauth_per_min"`
	UnauthGlobalPerMin int    `json:"unauth_global_per_min"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	DB                 string `json:"db"`
	MaxTurns           int    `json:"max_turns"`
	MaxBody            int    `json:"max_body"`
	RatePerMin         int    `json:"rate_per_min"`
	RatePerDay         int    `json:"rate_per_day"`
	AnnouncePerMin     int    `json:"announce_per_min"`
	AnnouncePerDay     int    `json:"announce_per_day"`
	RetentionDays      int    `json:"retention_days"`
	MaxWaitSec         int    `json:"max_wait_sec"`
	MaxConnsPerAgent   int    `json:"max_conns_per_agent"`
}

// Default varsayılan değerleri döndürür (DB boş: dosyada verilmesi zorunlu).
func Default() Server {
	return Server{
		Host: "127.0.0.1", Port: 8790,
		UnauthPerMin: 20, UnauthGlobalPerMin: 300,
		MaxTurns: 20, MaxBody: 16384, RatePerMin: 30, RatePerDay: 500,
		AnnouncePerMin: 3, AnnouncePerDay: 50,
		RetentionDays: 14, MaxWaitSec: 50, MaxConnsPerAgent: 4,
	}
}

// LoadServer dosyayı okur, varsayılanların üstüne yazar ve doğrular.
func LoadServer(path string) (Server, error) {
	f, err := os.Open(path)
	if err != nil {
		return Server{}, fmt.Errorf("ayar dosyası açılamadı: %w", err)
	}
	defer f.Close()
	c := Default()
	dec := json.NewDecoder(f)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return Server{}, fmt.Errorf("ayar dosyası geçersiz: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Server{}, errors.New("ayar dosyası geçersiz: JSON'dan sonra fazladan veri var")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return Server{}, fmt.Errorf("ayar dosyası geçersiz: %w", err)
	}
	if c.Listen != "" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return Server{}, err
		}
		if _, ok := fields["host"]; ok {
			return Server{}, errors.New("listen ile host verilemez")
		}
		if _, ok := fields["port"]; ok {
			return Server{}, errors.New("listen ile port verilemez")
		}
		c.Host, c.Port = "", 0
	}
	if err := c.Validate(); err != nil {
		return Server{}, err
	}
	return c, nil
}

// Validate değerlerin güvenli ve anlamlı olduğunu denetler.
func (c Server) Validate() error {
	if c.Listen != "" {
		if c.Host != "" || c.Port != 0 {
			return errors.New("listen ile host/port verilemez")
		}
		if c.Listen != "systemd" && !(strings.HasPrefix(c.Listen, "unix:") && filepath.IsAbs(strings.TrimPrefix(c.Listen, "unix:"))) {
			return errors.New("listen systemd veya unix:<mutlak yol> olmalı")
		}
	} else {
		if c.TrustForwardedFor {
			return errors.New("trust_forwarded_for yalnız unix/systemd dinlemede kullanılabilir")
		}
		if c.Host != "127.0.0.1" && c.Host != "::1" {
			return fmt.Errorf("host yalnız 127.0.0.1 ya da ::1 olabilir (verilen: %q)", c.Host)
		}
		if c.Port < 1 || c.Port > 65535 {
			return errors.New("port 1-65535 arasında olmalı")
		}
	}
	if c.DB == "" {
		return errors.New("db yolu zorunlu")
	}
	switch {
	case c.UnauthPerMin < 1 || c.UnauthGlobalPerMin < 1:
		return errors.New("unauth_per_min ve unauth_global_per_min en az 1 olmalı")
	case c.MaxTurns < 1:
		return errors.New("max_turns en az 1 olmalı")
	case c.MaxBody < 1 || c.MaxBody > 1<<20:
		return errors.New("max_body 1 ile 1048576 arasında olmalı")
	case c.RatePerMin < 1 || c.RatePerDay < 1:
		return errors.New("rate_per_min ve rate_per_day en az 1 olmalı")
	case c.AnnouncePerMin < 1 || c.AnnouncePerDay < 1:
		return errors.New("announce_per_min ve announce_per_day en az 1 olmalı")
	case c.RetentionDays < 1:
		return errors.New("retention_days en az 1 olmalı")
	case c.MaxWaitSec < 1 || c.MaxWaitSec > 120:
		return errors.New("max_wait_sec 1 ile 120 arasında olmalı")
	case c.MaxConnsPerAgent < 1:
		return errors.New("max_conns_per_agent en az 1 olmalı")
	}
	return nil
}

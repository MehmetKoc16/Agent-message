package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// NEDEN: sunucu yalnız loopback'te dinlemeli (Caddy arkasında); yanlışlıkla 0.0.0.0 yazılırsa
// relay internete çıplak açılırdı. Başlamayı reddetmeli.
func TestHostDisariAcilamaz(t *testing.T) {
	for _, h := range []string{"0.0.0.0", "", "localhost", "::", "192.168.1.5", "relay.example.com", "127.0.0.2"} {
		p := write(t, `{"host":"`+h+`","db":"/var/lib/agent-relay/relay.db"}`)
		if _, err := LoadServer(p); err == nil {
			t.Fatalf("host %q reddedilmeli", h)
		}
	}
	for _, h := range []string{"127.0.0.1", "::1"} {
		p := write(t, `{"host":"`+h+`","db":"/var/lib/agent-relay/relay.db"}`)
		if _, err := LoadServer(p); err != nil {
			t.Fatalf("host %q kabul edilmeli: %v", h, err)
		}
	}
}

// NEDEN: yazım hatası (ör. "max_bodyy") sessizce varsayılana düşerse güvenlik sınırı
// ayarlandığı sanılır ama uygulanmaz. Bilinmeyen alan hata olmalı.
func TestBilinmeyenConfigAlaniReddedilir(t *testing.T) {
	p := write(t, `{"db":"/x/relay.db","max_bodyy":5}`)
	_, err := LoadServer(p)
	if err == nil || !strings.Contains(err.Error(), "max_bodyy") {
		t.Fatalf("hata bekleniyordu: %v", err)
	}
	if _, err := LoadServer(write(t, `{"db":"/x/relay.db"} {"a":1}`)); err == nil {
		t.Fatal("fazladan JSON reddedilmeli")
	}
}

// NEDEN: varsayılanlar görev metnindeki değerlerle aynı kalmalı ve saçma değerler reddedilmeli.
func TestVarsayilanlarVeSinirlar(t *testing.T) {
	c, err := LoadServer(write(t, `{"db":"/x/relay.db","port":9000}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxTurns != 20 || c.MaxBody != 16384 || c.RatePerMin != 30 || c.RatePerDay != 500 ||
		c.RetentionDays != 14 || c.MaxWaitSec != 50 || c.MaxConnsPerAgent != 4 || c.Port != 9000 {
		t.Fatalf("varsayılanlar: %+v", c)
	}
	for _, bad := range []string{`{}`, `{"db":"x","port":0}`, `{"db":"x","max_turns":0}`, `{"db":"x","max_body":99999999}`, `{"db":"x","max_wait_sec":0}`} {
		if _, err := LoadServer(write(t, bad)); err == nil {
			t.Fatalf("%s reddedilmeli", bad)
		}
	}
}

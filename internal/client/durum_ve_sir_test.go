package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// NEDEN: adlarda '-' izinli olduğundan eski "state-oda-ajan" adı ("a-b","c") ile
// ("a","b-c") için aynı dosyaydı; köprü bekleyen cevabı yanlış odaya gönderebilirdi.
func TestDurumDosyasiAdlariOdaAjanCiftlerindeCakismaz(t *testing.T) {
	dir := t.TempDir()
	load := func(name, room, agent string) *Config {
		p := filepath.Join(dir, name)
		b, _ := json.Marshal(map[string]string{"url": "https://relay.example.com", "room": room, "agent": agent, "token": "sahte-kimlik-0123456789abcdef"})
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := LoadConfig(p)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	x, y := load("x.json", "a-b", "c"), load("y.json", "a", "b-c")
	if x.statePath == y.statePath || filepath.Base(x.statePath) != "state@a-b@c.json" || filepath.Base(y.statePath) != "state@a@b-c.json" {
		t.Fatalf("imleç dosyaları çakışıyor: %s %s", x.statePath, y.statePath)
	}
	if x.legacyStatePath != y.legacyStatePath {
		t.Fatal("test varsayımı: eski adlar çakışmalıydı")
	}
	bx, _ := stateFileNames(dir, "bridge", "a-b", "c", "codex")
	by, _ := stateFileNames(dir, "bridge", "a", "b-c", "codex")
	if bx == by {
		t.Fatal("köprü durum dosyaları çakışıyor")
	}
}

// NEDEN: aynı dizindeki başka relay ayarının token'ı önek taşımaz; relay send onu da
// engellemeli. Bozuk/kısa JSON dosyaları taramayı ya da normal gönderimi bozmamalı.
func TestSendAyniDizindekiBaskaAyarinTokeniniEngeller(t *testing.T) {
	var posts atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":1,"room":"r1","from":"veli","to":"","type":"bilgi","thread":"T-1","turn":1,"max_turns":20,"body":"x"}`))
	}))
	defer ts.Close()
	cfg := fakeConfig(t, ts.URL)
	dir := filepath.Dir(cfg)
	const other = "other-fake-credential-abcdef0123"
	for name, body := range map[string]string{
		"a-bozuk.json": "{bozuk",
		"diger.json":   `{"url":"https://relay.example.com","room":"r1","agent":"insan","token":"` + other + `"}`,
		"kisa.json":    `{"token":"kisa"}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, text := range []string{"bak: " + other, "bak: other-fake-\u200bcredential-abcdef0123"} {
		code, out, errOut := runWith(context.Background(), cfg, "send", "--type", "bilgi", text)
		if code != ExitSecret || !strings.Contains(errOut, "relay kimlik bilgisi") || strings.Contains(out+errOut, other) {
			t.Fatalf("başka ayarın token'ı engellenmedi: %d %q", code, errOut)
		}
	}
	if posts.Load() != 0 {
		t.Fatal("engellenen mesaj sunucuya gitti")
	}
	if code, _, errOut := runWith(context.Background(), cfg, "send", "--type", "bilgi", "kisa ve normal mesaj"); code != ExitOK || posts.Load() != 1 {
		t.Fatalf("normal gönderim bozuldu: %d %q", code, errOut)
	}
}

// NEDEN: kendi ayarının token'ı araya sıfır genişlik karakteri konarak gönderilememeli.
func TestSendArayaBicimKarakteriKonmusTokeniEngeller(t *testing.T) {
	cfg := fakeConfig(t, "http://127.0.0.1:1")
	code, _, errOut := runWith(context.Background(), cfg, "send", "--type", "bilgi", "client-test-\u200bcredential-0123456789")
	if code != ExitSecret || !strings.Contains(errOut, "relay kimlik bilgisi") {
		t.Fatalf("araya U+200B konmuş token engellenmedi: %d %q", code, errOut)
	}
}

// NEDEN: '@' ayırıcısına geçişte mevcut kurulumun imleci sıfırlanırsa eski mesajlar
// yeniden teslim edilir; eski adlı dosya yeni ada taşınmalı, yeni dosya varsa dokunulmamalı.
func TestEskiImlecDosyasiYeniAdaTasinir(t *testing.T) {
	dir := t.TempDir()
	oldPath, newPath := filepath.Join(dir, "state-r1-veli.json"), filepath.Join(dir, "state@r1@veli.json")
	if err := os.WriteFile(oldPath, []byte(`{"after":42}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacy(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if n, err := (cursor{path: newPath}).Read(); err != nil || n != 42 {
		t.Fatalf("imleç korunmadı: %d %v", n, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("eski dosya taşınmadı")
	}
	// Yeni dosya varken eski adlı dosya onu ezmemeli.
	if err := os.WriteFile(oldPath, []byte(`{"after":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacy(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if n, _ := (cursor{path: newPath}).Read(); n != 42 {
		t.Fatal("yeni imleç eski dosyayla ezildi")
	}
}

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeConfig, verilen adrese bakan bir istemci ayarı yazar.
func fakeConfig(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	b, _ := json.Marshal(map[string]string{"url": url, "room": "r1", "agent": "veli", "token": "t"})
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runWith(ctx context.Context, cfg string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Run(ctx, append([]string{"--config", cfg}, args...), Env{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb})
	return code, out.String(), errb.String()
}

// NEDEN: sunucu yeniden başlarken Caddy 502/503/504 döner. wait bunu kalıcı hata sayıp
// çıkarsa her güncellemede bekleyen tüm ajanlar düşer. Geçici sayılıp yeniden denenmeli.
func TestWait503SonrasiMesajiAlir(t *testing.T) {
	retryDelay = 20 * time.Millisecond
	var n atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := n.Add(1); c <= 2 {
			codes := []int{http.StatusServiceUnavailable, http.StatusBadGateway}
			http.Error(w, "geçici", codes[c-1])
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"messages":[{"id":7,"room":"r1","from":"ali","to":"","type":"bilgi","thread":"T-1","reply_to":null,"turn":1,"max_turns":20,"body":"geldi","created_at":"x"}]}`))
	}))
	defer ts.Close()
	code, out, errOut := runWith(context.Background(), fakeConfig(t, ts.URL), "wait", "--timeout", "10")
	if code != 0 || !strings.Contains(out, "geldi") || n.Load() != 3 {
		t.Fatalf("çıkış %d, istek %d, out=%q err=%q", code, n.Load(), out, errOut)
	}
}

// NEDEN: wait süresi boyunca sunucu hep 503 verirse "zaman aşımı" (2) değil "ulaşılamıyor" (3).
func TestWait503SureDolarsaCikis3(t *testing.T) {
	retryDelay = 20 * time.Millisecond
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "x", http.StatusGatewayTimeout)
	}))
	defer ts.Close()
	cfg := fakeConfig(t, ts.URL)
	if code, _, er := runWith(context.Background(), cfg, "wait", "--timeout", "1"); code != 3 {
		t.Fatalf("çıkış %d err=%q", code, er)
	}
	if code, _, _ := runWith(context.Background(), cfg, "wait", "--timeout", "0"); code != 3 {
		t.Fatalf("tek istek: çıkış %d", code)
	}
}

// NEDEN: send otomatik yeniden denerse sunucu ilk isteği işlemiş olabilir ve aynı mesaj iki kez
// gider. Tek deneme, çıkış 3 ve anlaşılır mesaj olmalı.
func TestSend503TekrarDenemezCikis3(t *testing.T) {
	var n atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		http.Error(w, "x", http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	code, _, errOut := runWith(context.Background(), fakeConfig(t, ts.URL), "send", "--type", "bilgi", "merhaba")
	if code != 3 || !strings.Contains(errOut, "geçici olarak yanıt vermiyor") || n.Load() != 1 {
		t.Fatalf("çıkış %d, istek %d, err=%q", code, n.Load(), errOut)
	}
}

// NEDEN: Ctrl+C "sunucuya ulaşılamadı" (3) diye raporlanmamalı; iptal ayrı kodla (130) çıkmalı.
func TestIptalCikis130(t *testing.T) {
	e := newCEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	t0 := time.Now()
	code, _, errOut := runWith(ctx, e.cfgs["veli"], "wait", "--timeout", "8")
	if code != 130 || !strings.Contains(errOut, "iptal edildi") || time.Since(t0) > 4*time.Second {
		t.Fatalf("çıkış %d err=%q süre %v", code, errOut, time.Since(t0))
	}
	// Ağ isteği sırasında iptal (tek istek komutu) da 130 vermeli.
	done, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if code, _, _ := runWith(done, e.cfgs["veli"], "inbox"); code != 130 {
		t.Fatalf("inbox iptal: %d", code)
	}
}

// NEDEN: çöken bir sürecin bıraktığı kilit (5 sn'den eski) yeni işlemi düşürmemeli: bayat sayılıp
// silinmeli ve işlem hemen başarılı olmalı.
func TestBayatKilitBeklemeyiBozmaz(t *testing.T) {
	dir := t.TempDir()
	c := cursor{filepath.Join(dir, "state.json")}
	lock := c.path + ".lock"
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-6 * time.Second)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if err := c.Advance(5); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if n, _ := c.Read(); n != 5 || time.Since(t0) > 2*time.Second {
		t.Fatalf("imleç %d, süre %v", n, time.Since(t0))
	}
	if lockWait <= staleLock {
		t.Fatal("kilit bekleme süresi bayat eşiğinden uzun olmalı")
	}
}

// NEDEN: sunucu 200 dönüp gövdesi bozuksa kullanıcı "HTTP 200" yerine anlamlı bir mesaj görmeli.
func TestBozukYanitAnlamliMesaj(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>"))
	}))
	defer ts.Close()
	code, _, errOut := runWith(context.Background(), fakeConfig(t, ts.URL), "agents")
	if code != 1 || !strings.Contains(errOut, "geçersiz yanıt") || strings.Contains(errOut, "HTTP 200") {
		t.Fatalf("çıkış %d err=%q", code, errOut)
	}
}

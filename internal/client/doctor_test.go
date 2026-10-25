package client

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func doctorConfigFixture(t *testing.T, address string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-config.json")
	data, _ := json.Marshal(Config{URL: address, Room: "private-room", Agent: "private-agent", Token: setupTestToken()})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func doctorRun(t *testing.T, ctx context.Context, path string, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(ctx, append([]string{"doctor"}, args...), Env{DefaultConfig: path, Stdout: &out, Stderr: &errOut})
	text := out.String() + errOut.String()
	assertSetupNoToken(t, setupTestToken(), out.String(), errOut.String())
	for _, private := range []string{path, "private-room", "private-agent", "SERVER-PRIVATE", "\x1b"} {
		if private != "" && strings.Contains(text, private) {
			t.Fatalf("doctor özel veri yazdı: %q", private)
		}
	}
	return code, text
}

func doctorReply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/v1/health":
		io.WriteString(w, `{"ok":true}`)
	case "/v1/me":
		io.WriteString(w, `{"name":"private-agent","kind":"ajan","rooms":["private-room"]}`)
	case "/v1/rooms/private-room/agents":
		io.WriteString(w, `{"agents":[{"name":"private-agent","kind":"ajan"}]}`)
	default:
		w.WriteHeader(404)
	}
}

func TestDoctorGercekSunucudaMesajTuketmezVeDosyaDegistirmez(t *testing.T) {
	// NEDEN: teşhis komutu inbox/wait veya setup gibi davranırsa gerçek mesajlar kaybolabilir.
	e := newCEnv(t)
	if code, _, _ := e.run("ali", "", "send", "--type", "bilgi", "--to", "veli", "doctor öncesi okunmamış mesaj"); code != ExitOK {
		t.Fatal(code)
	}
	var mu sync.Mutex
	var paths []string
	wrapped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/v1/health" && r.Header.Get("Authorization") != "" {
			t.Error("sağlık isteği token taşımamalı")
		}
		e.srv.Handler().ServeHTTP(w, r)
	}))
	defer wrapped.Close()
	e.cfgs["veli"] = e.writeConfig("veli", wrapped.URL, e.toks["veli"])
	before, _ := os.ReadFile(e.cfgs["veli"])
	code, out, errOut := e.run("veli", "", "doctor")
	if code != ExitOK || errOut != "" || !strings.Contains(out, "[OK] SONUÇ") {
		t.Fatal(code, out, errOut)
	}
	assertSetupNoToken(t, e.toks["veli"], out, errOut)
	if strings.Contains(out, wrapped.URL) || strings.Contains(out, "doctor öncesi") {
		t.Fatal("özel veri rapora girdi")
	}
	mu.Lock()
	gotPaths := append([]string(nil), paths...)
	mu.Unlock()
	if !reflect.DeepEqual(gotPaths, []string{"GET /v1/health", "GET /v1/me", "GET /v1/rooms/r1/agents"}) {
		t.Fatal(gotPaths)
	}
	after, _ := os.ReadFile(e.cfgs["veli"])
	if !bytes.Equal(before, after) {
		t.Fatal("ayar değişti")
	}
	if _, err := os.Stat(e.statePath("veli")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("doctor imleç oluşturdu", err)
	}
	if code, out, _ := e.run("veli", "", "inbox"); code != ExitOK || !strings.Contains(out, "doctor öncesi okunmamış mesaj") {
		t.Fatal("doctor mesaj tüketti", code, out)
	}
	stateBefore, _ := os.ReadFile(e.statePath("veli"))
	if code, _, _ := e.run("veli", "", "doctor"); code != ExitOK {
		t.Fatal(code)
	}
	stateAfter, _ := os.ReadFile(e.statePath("veli"))
	if !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("doctor imleci ilerletti")
	}
}

func TestDoctorHataAsamasiniAyirirVeHamYanitBasmaz(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, want string
		status, code           int
	}{
		{"token", "/v1/me", "SERVER-PRIVATE" + setupTestToken(), "[HATA] KİMLİK", 401, ExitError},
		{"oda", "/v1/rooms/private-room/agents", "SERVER-PRIVATE", "[HATA] ODA", 403, ExitError},
		{"proxy", "/v1/health", "SERVER-PRIVATE", "[HATA] SAĞLIK", 502, ExitUnreachable},
		{"saglik-yetki", "/v1/health", "SERVER-PRIVATE", "Token henüz sınanmadı", 403, ExitError},
		{"limit", "/v1/me", "SERVER-PRIVATE", "sınırına", 429, ExitUnreachable},
		{"yol", "/v1/health", "SERVER-PRIVATE", "API yolu", 404, ExitError},
		{"hesap", "/v1/me", `{"name":"SERVER-PRIVATE","kind":"ajan","rooms":["private-room"]}`, "başka bir hesaba", 200, ExitError},
		{"uyelik", "/v1/me", `{"name":"private-agent","kind":"ajan","rooms":[]}`, "üye değil", 200, ExitError},
		{"json", "/v1/health", "SERVER-PRIVATE" + setupTestToken(), "sağlık yanıtı", 200, ExitError},
		{"false", "/v1/health", `{"ok":false}`, "sağlık yanıtı", 200, ExitError},
		{"kimlik-json", "/v1/me", `{"name":"private-agent","kind":"bozuk"}`, "Kimlik yanıtı", 200, ExitError},
		{"oda-json", "/v1/rooms/private-room/agents", `{}`, "Katılımcı yanıtı", 200, ExitError},
		{"buyuk", "/v1/health", strings.Repeat("x", doctorMaxBytes+1), "sağlık yanıtı", 200, ExitError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tc.path {
					w.WriteHeader(tc.status)
					io.WriteString(w, tc.body)
					return
				}
				doctorReply(w, r)
			}))
			defer srv.Close()
			code, text := doctorRun(t, context.Background(), doctorConfigFixture(t, srv.URL))
			if code != tc.code || !strings.Contains(text, tc.want) {
				t.Fatal(code, text)
			}
		})
	}
}

func TestDoctorYonlendirmedeTokeniTasimaz(t *testing.T) {
	for _, at := range []string{"/v1/health", "/v1/me"} {
		t.Run(at, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mu.Lock(); calls++; mu.Unlock(); doctorReply(w, r) }))
			defer target.Close()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == at {
					http.Redirect(w, r, target.URL+"/SERVER-PRIVATE", 307)
					return
				}
				doctorReply(w, r)
			}))
			defer srv.Close()
			code, text := doctorRun(t, context.Background(), doctorConfigFixture(t, srv.URL))
			if code != ExitError || !strings.Contains(text, "Yönlendirme izlenmedi") {
				t.Fatal(code, text)
			}
			mu.Lock()
			defer mu.Unlock()
			if calls != 0 {
				t.Fatal("yönlendirme izlendi")
			}
		})
	}
}

func TestDoctorAyarHatalarindaDegerleriYazmaz(t *testing.T) {
	for _, body := range []string{`SERVER-PRIVATE` + setupTestToken(), `{"SERVER-PRIVATE":"` + setupTestToken() + `"}`, `{"url":"https://` + setupTestToken() + `@example.invalid","room":"private-room","agent":"private-agent","token":"` + setupTestToken() + `"}`} {
		path := filepath.Join(t.TempDir(), "private-config.json")
		os.WriteFile(path, []byte(body), 0o600)
		code, text := doctorRun(t, context.Background(), path)
		if code != ExitError || !strings.Contains(text, "[HATA] AYAR") {
			t.Fatal(code, text)
		}
	}
	code, text := doctorRun(t, context.Background(), filepath.Join(t.TempDir(), "missing.json"))
	if code != ExitError || !strings.Contains(text, "relay setup") {
		t.Fatal(code, text)
	}
}

func TestDoctorBozukImleciRaporlarAmaOnarmaz(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(doctorReply))
	defer srv.Close()
	path := doctorConfigFixture(t, srv.URL)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte("SERVER-PRIVATE" + setupTestToken())
	os.WriteFile(cfg.statePath, bad, 0o600)
	code, text := doctorRun(t, context.Background(), path)
	if code != ExitError || !strings.Contains(text, "[HATA] İMLEÇ") || !strings.Contains(text, "[OK] ODA") {
		t.Fatal(code, text)
	}
	after, _ := os.ReadFile(cfg.statePath)
	if !bytes.Equal(after, bad) {
		t.Fatal("imleç değişti")
	}
}

func TestDoctorTLSVeTCPHatalariniAyirir(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(doctorReply))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	code, text := doctorRun(t, context.Background(), doctorConfigFixture(t, srv.URL))
	if code != ExitUnreachable || !strings.Contains(text, "[HATA] TLS") {
		t.Fatal(code, text)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := ln.Addr().String()
	ln.Close()
	code, text = doctorRun(t, context.Background(), doctorConfigFixture(t, "http://"+address))
	if code != ExitUnreachable || !strings.Contains(text, "[HATA] BAĞLANTI") {
		t.Fatal(code, text)
	}
}

func TestDoctorAgHataMetniniVeDNSAdiniGizler(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&net.DNSError{Name: setupTestToken(), Err: "SERVER-PRIVATE"}, "[HATA] DNS"},
		{x509.UnknownAuthorityError{}, "[HATA] TLS"},
		{errors.New("SERVER-PRIVATE" + setupTestToken()), "[HATA] BAĞLANTI"},
	} {
		var out bytes.Buffer
		code := (&doctorTrace{}).networkError(context.Background(), Env{Stdout: &out}, &url.Error{URL: "SERVER-PRIVATE" + setupTestToken(), Err: tc.err})
		if code != ExitUnreachable || !strings.Contains(out.String(), tc.want) {
			t.Fatal(code, out.String())
		}
		assertSetupNoToken(t, setupTestToken(), out.String(), "")
		if strings.Contains(out.String(), "SERVER-PRIVATE") {
			t.Fatal("ham hata yazıldı")
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	var out bytes.Buffer
	code := (&doctorTrace{tlsStarted: true}).networkError(ctx, Env{Stdout: &out}, context.DeadlineExceeded)
	if code != ExitUnreachable || !strings.Contains(out.String(), "[HATA] ZAMAN AŞIMI") {
		t.Fatal(code, out.String())
	}
}

func TestDoctorTekSureButcesiVeIptal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(60 * time.Millisecond):
			doctorReply(w, r)
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	code, text := doctorRun(t, context.Background(), doctorConfigFixture(t, srv.URL), "--timeout", "100ms")
	if code != ExitUnreachable || !strings.Contains(text, "ZAMAN AŞIMI") {
		t.Fatal(code, text)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code, text = doctorRun(t, ctx, doctorConfigFixture(t, srv.URL))
	if code != ExitCanceled || !strings.Contains(text, "İPTAL") {
		t.Fatal(code, text)
	}
}

func TestDoctorYardimVeBayraklar(t *testing.T) {
	for _, args := range [][]string{{"--timeout", "0s"}, {"--timeout", "3m"}, {"--timeout", setupTestToken()}, {"--unknown", setupTestToken()}, {"unexpected"}} {
		if code, _ := doctorRun(t, context.Background(), "", args...); code != ExitError {
			t.Fatal(code, args)
		}
	}
	if code, text := doctorRun(t, context.Background(), "", "--help"); code != ExitOK || !strings.Contains(text, "doctor") {
		t.Fatal(code, text)
	}
}

func TestDoctorBekleyenAgIstegiIptaldeHemenBiter(t *testing.T) {
	entered := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer srv.Close()
	path := doctorConfigFixture(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		code   int
		output string
	}
	done := make(chan result, 1)
	go func() {
		var out, errOut bytes.Buffer
		code := Run(ctx, []string{"doctor"}, Env{DefaultConfig: path, Stdout: &out, Stderr: &errOut})
		done <- result{code, out.String() + errOut.String()}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("sağlık isteği başlamadı")
	}
	cancel()
	select {
	case got := <-done:
		if got.code != ExitCanceled || !strings.Contains(got.output, "[HATA] İPTAL") {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("iptal edilen teşhis beklemede kaldı")
	}
}

package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// izleEnv ağ dinleyicisi açmadan aynı yapılandırma ve geçici DB ile çalışır.
func izleEnv(t *testing.T) *testEnv {
	t.Helper()
	cfg := testConfig(t.TempDir())
	st, err := store.Open(cfg.DB, cfg.MaxTurns)
	if err != nil {
		t.Fatal(err)
	}
	e := &testEnv{t: t, cfg: cfg, st: st, logs: &syncBuf{}, tokens: map[string]string{}}
	for _, name := range []string{"ali", "veli", "mehmet", "dis"} {
		kind, rooms := "ajan", []string{"r1", "r2"}
		if name == "mehmet" {
			kind, rooms = "insan", []string{"r1"}
		}
		if name == "dis" {
			rooms = []string{"r2"}
		}
		token := "test-izle-" + name
		e.tokens[name] = token
		if err := st.AddAgent(context.Background(), name, kind, store.HashToken(token), rooms); err != nil {
			t.Fatal(err)
		}
	}
	e.srv = New(cfg, st, slog.New(slog.NewTextHandler(e.logs, nil)))
	t.Cleanup(func() { e.srv.Close(); st.Close() })
	return e
}

func izleRequest(h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestIzleSayfasiBasliklari(t *testing.T) {
	// NEDEN: statik dosyalar kimliksizdir; tarayıcı sınırları her dosyada korunmalı.
	h := izleEnv(t).srv.Handler()
	const csp = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	for path, typ := range map[string]string{
		"/v1/izle":          "text/html; charset=utf-8",
		"/v1/izle/izle.js":  "text/javascript; charset=utf-8",
		"/v1/izle/izle.css": "text/css; charset=utf-8",
	} {
		for _, method := range []string{"GET", "HEAD"} {
			w := izleRequest(h, method, path, "")
			if w.Code != 200 {
				t.Fatalf("%s %s: %d", method, path, w.Code)
			}
			for key, want := range map[string]string{
				"Content-Type": typ, "Content-Security-Policy": csp,
				"X-Frame-Options": "DENY", "Cache-Control": "no-store",
				"X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer",
				"Cross-Origin-Opener-Policy": "same-origin", "Cross-Origin-Resource-Policy": "same-origin",
			} {
				if got := w.Header().Get(key); got != want {
					t.Errorf("%s %s: %q, beklenen %q", path, key, got, want)
				}
			}
			for key := range w.Header() {
				if strings.HasPrefix(strings.ToLower(key), "access-control-") || strings.EqualFold(key, "Set-Cookie") {
					t.Errorf("istenmeyen başlık: %s", key)
				}
			}
			if method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD gövde döndürmemeli")
			}
		}
	}
}

func TestIzleBilinmeyenYol404(t *testing.T) {
	// NEDEN: yol temizliği izleme dizinini başka uca yönlendirmemeli.
	h := izleEnv(t).srv.Handler()
	for _, path := range []string{"/v1/izle/yok.js", "/v1/izle/../v1/health", "/v1/izle/../health", "/v1/izle/index.html", "/v1/izle/"} {
		w := izleRequest(h, "GET", path, "")
		if w.Code != 404 || strings.TrimSpace(w.Body.String()) != `{"error":"bulunamadi"}` {
			t.Errorf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestIzleGomuluDosyalarGuvenli(t *testing.T) {
	// NEDEN: güvenilmeyen mesajlar kod olamaz; dosyalar dış kaynak kullanmamalı.
	err := fs.WalkDir(izleFiles, "izle", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := izleFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, banned := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "localStorage", "http://", "https://"} {
			if strings.Contains(s, banned) {
				t.Errorf("%s yasak ifade: %s", path, banned)
			}
		}
		if strings.HasSuffix(path, ".js") && regexp.MustCompile(`\bsetTimeout\s*\(\s*["'`+"`"+`]`).MatchString(s) {
			t.Fatal("string ile zamanlayıcı çalıştırılamaz")
		}
		if strings.HasSuffix(path, ".js") {
			// NEDEN: yönetim anahtarı yenilemede kaybolmalı, tarayıcı deposuna yazılmamalı.
			for _, line := range strings.Split(s, "\n") {
				if strings.Contains(line, "sessionStorage") && (strings.Contains(line, "management") || strings.Contains(line, "oturum")) {
					t.Fatal("yönetim oturumu depoya yazılamaz")
				}
			}
		}
		if strings.HasSuffix(path, ".html") {
			if regexp.MustCompile(`(?i)\b(method|action)\s*=`).MatchString(s) {
				t.Fatal("form method/action içermemeli")
			}
			if regexp.MustCompile(`(?is)<script\b[^>]*>\s*\S.*?</script>`).MatchString(s) || regexp.MustCompile(`(?i)style\s*=`).MatchString(s) {
				t.Fatal("satır içi betik veya stil var")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogYalnizInsan(t *testing.T) {
	// NEDEN: oda üyeliği ve insan türü birbirinden bağımsız yetki sınırlarıdır.
	e := izleEnv(t)
	h := e.srv.Handler()
	for _, tc := range []struct {
		name, room, code string
		status           int
	}{
		{"ali", "r1", "yalniz_insan", 403}, {"mehmet", "r1", "", 200},
		{"mehmet", "r2", "yasak", 403}, {"dis", "r1", "yasak", 403}, {"", "r1", "yetkisiz", 401},
	} {
		w := izleRequest(h, "GET", "/v1/rooms/"+tc.room+"/log", e.tokens[tc.name])
		if w.Code != tc.status || (tc.code != "" && !strings.Contains(w.Body.String(), `"error":"`+tc.code+`"`)) {
			t.Errorf("%s %s: %d %s", tc.name, tc.room, w.Code, w.Body.String())
		}
	}
}

func TestLogTumOdaMesajlari(t *testing.T) {
	// NEDEN: insan yönlü mesajları da görmeli; başka odanın verisi sızmamalı.
	e := izleEnv(t)
	var ids []int64
	for _, in := range []store.PostInput{
		{Room: "r1", From: "ali", To: "veli"}, {Room: "r1", From: "veli", To: "ali"},
		{Room: "r2", From: "ali"}, {Room: "r1", From: "ali"},
	} {
		in.Type, in.Body = "bilgi", "<script>güvenilmeyen veri</script>"
		m, err := e.st.Post(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if in.Room == "r1" {
			ids = append(ids, m.ID)
		}
	}
	h := e.srv.Handler()
	for _, after := range []int64{0, ids[0]} {
		w := izleRequest(h, "GET", "/v1/rooms/r1/log?after="+itoa(int(after)), e.tokens["mehmet"])
		var out struct {
			Messages []store.Message `json:"messages"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 {
			t.Fatalf("%d: %v", w.Code, err)
		}
		want := ids
		if after != 0 {
			want = ids[1:]
		}
		if len(out.Messages) != len(want) {
			t.Fatalf("mesaj sayısı: %d", len(out.Messages))
		}
		for i, m := range out.Messages {
			if m.ID != want[i] || m.Room != "r1" {
				t.Fatalf("yanlış mesaj: %+v", m)
			}
		}
	}
	for _, field := range []string{"after", "wait"} {
		for _, value := range []string{"-1", "yok", "999999999999999999999999"} {
			w := izleRequest(h, "GET", "/v1/rooms/r1/log?"+field+"="+value, e.tokens["mehmet"])
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"error":"gecersiz_`+field+`"`) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		}
	}
}

func TestLogBeklemesiBekliyorSayilmaz(t *testing.T) {
	// NEDEN: insan izleyici ajanlara dinleyen karşı taraf olarak görünmemeli.
	e := izleEnv(t)
	h := e.srv.Handler()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- izleRequest(h, "GET", "/v1/rooms/r1/log?wait=5", e.tokens["mehmet"]) }()
	eventually(t, time.Second, "log sorgusu kanalı aldı", func() bool {
		e.srv.hub.mu.Lock()
		defer e.srv.hub.mu.Unlock()
		return e.srv.hub.rooms["r1"] != nil
	})
	w := izleRequest(h, "GET", "/v1/rooms/r1/agents", e.tokens["ali"])
	var out struct {
		Agents []struct {
			Name    string `json:"name"`
			Waiting bool   `json:"waiting"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 {
		t.Fatalf("ajanlar: %d %v", w.Code, err)
	}
	found := false
	for _, a := range out.Agents {
		if a.Name == "mehmet" {
			found = true
			if a.Waiting {
				t.Fatal("insan bekliyor görünüyor")
			}
		}
	}
	if !found {
		t.Fatal("insan listede yok")
	}
	select {
	case <-done:
		t.Fatal("mesajsız erken döndü")
	default:
	}
	r := httptest.NewRequest("POST", "/v1/rooms/r1/messages", strings.NewReader(`{"to":"veli","type":"bilgi","body":"canlı"}`))
	r.Header.Set("Authorization", "Bearer "+e.tokens["ali"])
	post := httptest.NewRecorder()
	h.ServeHTTP(post, r)
	if post.Code != 201 {
		t.Fatalf("gönderim: %d %s", post.Code, post.Body.String())
	}
	select {
	case got := <-done:
		if got.Code != 200 || !strings.Contains(got.Body.String(), `"body":"canlı"`) {
			t.Fatalf("log: %d %s", got.Code, got.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("long-poll zamanında uyanmadı")
	}
}

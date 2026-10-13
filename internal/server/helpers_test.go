package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/admin"
	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// testEnv: geçici DB'li gerçek sunucu. Ajanlar:
// ali (r1,r2), veli (r1), can (r1), dis (r2), mehmet (insan, r1).
type testEnv struct {
	t      *testing.T
	cfg    config.Server
	st     *store.Store
	srv    *Server
	ts     *httptest.Server
	logs   *syncBuf
	tokens map[string]string
}

func testConfig(dir string) config.Server {
	c := config.Default()
	c.DB = filepath.Join(dir, "relay.db")
	c.MaxWaitSec = 10
	return c
}

func newEnv(t *testing.T, mut func(*config.Server)) *testEnv {
	t.Helper()
	cfg := testConfig(t.TempDir())
	if mut != nil {
		mut(&cfg)
	}
	st, err := store.Open(cfg.DB, cfg.MaxTurns)
	if err != nil {
		t.Fatal(err)
	}
	e := &testEnv{t: t, cfg: cfg, st: st, logs: &syncBuf{}, tokens: map[string]string{}}
	add := func(name, kind string, rooms ...string) {
		tok, err := admin.NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AddAgent(context.Background(), name, kind, store.HashToken(tok), rooms); err != nil {
			t.Fatal(err)
		}
		e.tokens[name] = tok
	}
	add("ali", "ajan", "r1", "r2")
	add("veli", "ajan", "r1")
	add("can", "ajan", "r1")
	add("dis", "ajan", "r2")
	add("mehmet", "insan", "r1")
	e.srv = New(cfg, st, slog.New(slog.NewTextHandler(e.logs, nil)))
	e.ts = httptest.NewServer(e.srv.Handler())
	t.Cleanup(func() {
		e.srv.Close()
		e.ts.CloseClientConnections()
		e.ts.Close()
		st.Close()
	})
	return e
}

// raw ham istek yapar.
func (e *testEnv) raw(method, path, token string, body []byte) (int, []byte) {
	e.t.Helper()
	status, b, err := e.rawCtx(context.Background(), method, path, token, body)
	if err != nil {
		e.t.Fatal(err)
	}
	return status, b
}

func (e *testEnv) rawCtx(ctx context.Context, method, path, token string, body []byte) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.ts.URL+path, rd)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

// call JSON istek yapar; yanıtı map olarak döndürür.
func (e *testEnv) call(method, path, agent string, payload any) (int, map[string]any) {
	e.t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	status, b := e.raw(method, path, e.tokens[agent], body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return status, out
}

// send mesaj gönderir.
func (e *testEnv) send(room, agent string, payload map[string]any) (int, map[string]any) {
	e.t.Helper()
	if _, ok := payload["body"]; !ok {
		payload["body"] = "merhaba"
	}
	if _, ok := payload["type"]; !ok {
		payload["type"] = "bilgi"
	}
	return e.call("POST", "/v1/rooms/"+room+"/messages", agent, payload)
}

// fetch mesajları okur.
func (e *testEnv) fetch(room, agent string, after int, wait int) []map[string]any {
	e.t.Helper()
	st, out := e.call("GET", "/v1/rooms/"+room+"/messages?after="+itoa(after)+"&wait="+itoa(wait), agent, nil)
	if st != 200 {
		e.t.Fatalf("fetch durum %d: %v", st, out)
	}
	return toList(out["messages"])
}

func toList(v any) []map[string]any {
	arr, _ := v.([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, x := range arr {
		out = append(out, x.(map[string]any))
	}
	return out
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func num(v any) int {
	f, _ := v.(float64)
	return int(f)
}

// eventually koşul sağlanana dek kısa aralıklarla dener.
func eventually(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("zamanında gerçekleşmedi: %s", what)
}

func (e *testEnv) isWaiting(room, who string) bool {
	_, out := e.call("GET", "/v1/rooms/"+room+"/agents", "ali", nil)
	for _, a := range toList(out["agents"]) {
		if a["name"] == who {
			return a["waiting"] == true
		}
	}
	return false
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// rawURL tam adrese GET atar (httptest dışındaki dinleyiciler için).
func rawURL(ctx context.Context, url, token string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

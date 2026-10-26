package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func dene(s *Server, ip, token string) int {
	r := httptest.NewRequest("GET", "/v1/rooms/r1/agents", nil)
	r.RemoteAddr = ip
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w.Code
}

// NEDEN: limit dolduğunda yanlış token bile DB doğrulamasına ulaşmamalı.
func Test_kimliksiz_istek_ip_basina_429(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.UnauthPerMin = 2 })
	calls := 0
	e.srv.authenticate = func(context.Context, string) (store.Agent, error) {
		calls++
		return store.Agent{}, store.ErrUnauthorized
	}
	for _, want := range []int{401, 401, 429} {
		if got := dene(e.srv, "[::1]:12", "yanlis"); got != want {
			t.Fatalf("%d != %d", got, want)
		}
	}
	if calls != 2 {
		t.Fatalf("DB çağrısı: %d", calls)
	}
	if got := dene(e.srv, "[::1]:99", ""); got != 429 {
		t.Fatal(got)
	}
	if got := dene(e.srv, "127.0.0.1:12", ""); got != 401 {
		t.Fatal(got)
	}
}

// NEDEN: başarılı istekler kotayı tüketmez; tanınan ajan dolu kovada doğrulanır.
func Test_kimliksiz_sinir_basarili_ajani_etkilemez(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.UnauthPerMin = 1; c.UnauthGlobalPerMin = 1 })
	for i := 0; i < 5; i++ {
		if got := dene(e.srv, "[::1]:12", e.tokens["ali"]); got != 200 {
			t.Fatal(got)
		}
	}
	if got := dene(e.srv, "[::1]:12", ""); got != 401 {
		t.Fatal(got)
	}
	if got := dene(e.srv, "[::1]:12", e.tokens["ali"]); got != 200 {
		t.Fatal(got)
	}
	e.srv.authenticate = func(context.Context, string) (store.Agent, error) { return store.Agent{}, store.ErrUnauthorized }
	if got := dene(e.srv, "[::1]:12", e.tokens["ali"]); got != 401 {
		t.Fatal(got)
	}
	if got := dene(e.srv, "[::1]:12", e.tokens["ali"]); got != 429 {
		t.Fatal(got)
	}
}

// NEDEN: istemci başlığı TCP'de veya güven kapalıyken IP kotasını aşamamalı.
func Test_xff_yalniz_guvenilirken_kullanilir(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "[::1]:12"
	r.Header.Add("X-Forwarded-For", "127.0.0.1, ::1")
	r.Header.Add("X-Forwarded-For", "127.0.0.1")
	for _, tc := range []struct {
		listen string
		trust  bool
		want   string
	}{
		{"", false, "::1"}, {"", true, "::1"}, {"systemd", false, "::1"}, {"systemd", true, "127.0.0.1"}, {"unix:/run/agent-relay/relay.sock", true, "127.0.0.1"},
	} {
		s.cfg.Listen, s.cfg.TrustForwardedFor = tc.listen, tc.trust
		if got := s.clientIP(r); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	r.Header.Set("X-Forwarded-For", "127.0.0.1, invalid")
	if got := s.clientIP(r); got != "::1" {
		t.Fatal(got)
	}
}

// NEDEN: farklı IP'ler global bütçeyi aşınca DB yükü durmalı; pencere açılmalı.
func Test_global_kimliksiz_sinir(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.UnauthGlobalPerMin = 2 })
	now := time.Now()
	e.srv.unauth.now = func() time.Time { return now }
	if dene(e.srv, "127.0.0.1:1", "") != 401 || dene(e.srv, "[::1]:1", "") != 401 {
		t.Fatal("401 bekleniyordu")
	}
	e.srv.authenticate = func(context.Context, string) (store.Agent, error) {
		t.Fatal("DB'ye gidildi")
		return store.Agent{}, nil
	}
	if got := dene(e.srv, "unknown", "yanlis"); got != 429 {
		t.Fatal(got)
	}
	now = now.Add(time.Minute)
	if got := dene(e.srv, "unknown", ""); got != 401 {
		t.Fatal(got)
	}
}

// A restart or expiry clears the recognition cache. Even a new account on the
// attacker's IP must authenticate while both failure buckets are full.
func TestColdTokenSurvivesSaturatedUnauthLimits(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.UnauthPerMin = 1; c.UnauthGlobalPerMin = 1 })
	if got := dene(e.srv, "127.0.0.1:1", ""); got != 401 {
		t.Fatal(got)
	}
	for _, ip := range []string{"127.0.0.1:2", "[::1]:3"} {
		e.srv.unauth.known = make(map[[32]byte]time.Time)
		if got := dene(e.srv, ip, e.tokens["ali"]); got != 200 {
			t.Fatalf("cold valid token at %s: %d", ip, got)
		}
	}
	if err := e.st.RevokeAgent(context.Background(), "veli"); err != nil {
		t.Fatal(err)
	}
	if got := dene(e.srv, "127.0.0.1:2", e.tokens["veli"]); got != 429 {
		t.Fatalf("revoked token bypassed limit: %d", got)
	}
}

// NEDEN: paralel denemeler ve sahte IP'ler belleği sınırsız büyütmemeli.
func Test_kimliksiz_harita_ve_rezervasyon_siniri(t *testing.T) {
	l := newUnauthLimiter(1, maxUnauthIPs+1)
	now := time.Now()
	l.now = func() time.Time { return now }
	done, ok := l.begin("ilk", "")
	if !ok {
		t.Fatal("ilk istek")
	}
	if _, ok := l.begin("ilk", ""); ok {
		t.Fatal("rezervasyon aşımı")
	}
	done(false)
	for i := 0; i < maxUnauthIPs; i++ {
		f, ok := l.begin(itoa(i), "")
		if ok {
			f(true)
		}
	}
	if len(l.ips) > maxUnauthIPs {
		t.Fatal("harita büyüdü")
	}
	if _, ok := l.begin("yeni", ""); ok {
		t.Fatal("harita sınırı aşımı")
	}
	now = now.Add(time.Minute)
	f, ok := l.begin("yeni", "")
	if !ok {
		t.Fatal("temizlenmedi")
	}
	f(false)
}

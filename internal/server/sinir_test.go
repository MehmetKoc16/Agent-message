package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/admin"
	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// sinirIstek ağ açmadan işleyiciyi çağırır; testin değiştirdiği Server alanları
// aynı goroutine'de okunur.
func sinirIstek(s *Server, method, path, ip, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = ip
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func sinirBekle(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || code != "" && !strings.Contains(w.Body.String(), `"error":"`+code+`"`) {
		t.Fatalf("durum=%d gövde=%s beklenen=%d %s", w.Code, w.Body.String(), status, code)
	}
}

// NEDEN: kimliği doğrulanmış tek hesap GET döngüsüyle (wait=0, /v1/unread)
// tek SQLite bağlantısını tıkayamamalı; bu kova POST kotasını harcamamalı ve
// başka hesabı etkilememeli.
func TestGetHizSiniriHesapBasina(t *testing.T) {
	e := newEnv(t, nil)
	e.srv.gets = newRateLimiter(2, 100)
	ip := "127.0.0.1:1"
	sinirBekle(t, sinirIstek(e.srv, "GET", "/v1/me", ip, e.tokens["ali"], ""), 200, "")
	sinirBekle(t, sinirIstek(e.srv, "GET", "/v1/rooms/r1/messages?wait=0", ip, e.tokens["ali"], ""), 200, "")
	sinirBekle(t, sinirIstek(e.srv, "GET", "/v1/unread", ip, e.tokens["ali"], ""), 429, "hiz_siniri")
	sinirBekle(t, sinirIstek(e.srv, "HEAD", "/v1/me", ip, e.tokens["ali"], ""), 429, "")
	// Ayrı kova: GET sınırı dolan hesap mesaj gönderebilir.
	sinirBekle(t, sinirIstek(e.srv, "POST", "/v1/rooms/r1/messages", ip, e.tokens["ali"], `{"type":"bilgi","body":"merhaba"}`), 201, "")
	// Başka hesap etkilenmez.
	sinirBekle(t, sinirIstek(e.srv, "GET", "/v1/me", ip, e.tokens["veli"], ""), 200, "")
}

// NEDEN: yavaş bir TOTP girişi (DB işlemi) sürerken süreç kilidi tutulursa
// tüm /v1/yonetim/* oturum denetimleri bekler. Giriş DB'de askıdayken mevcut
// oturumla yapılan istek hemen yanıt almalı.
func TestYonetimGirisiOturumDenetiminiBekletmez(t *testing.T) {
	e, now := yonetimEnv(t)
	session := yonetimLogin(t, e, *now)
	code, err := admin.TOTPCode(testTOTPSecret, now.Unix()/30+1)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	gercek := e.srv.attemptTOTP
	e.srv.attemptTOTP = func(ctx context.Context, name, kod string, m func(string, string, time.Time) (int64, bool)) (string, bool, bool, error) {
		close(entered)
		<-release
		return gercek(ctx, name, kod, m)
	}
	done := make(chan int, 1)
	go func() { done <- yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"`+code+`"}`).Code }()
	<-entered
	got := make(chan int, 1)
	go func() { got <- yonetimReq(e, "GET", "/hesaplar", "mehmet", session, "").Code }()
	select {
	case c := <-got:
		if c != 200 {
			close(release)
			t.Fatalf("oturum denetimi: %d", c)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("giriş sürerken oturum denetimi bloklandı")
	}
	close(release)
	if c := <-done; c != 200 {
		t.Fatalf("giriş: %d", c)
	}
	e.srv.yonetimMu.Lock()
	n := len(e.srv.yonetimSessions["mehmet"])
	e.srv.yonetimMu.Unlock()
	if n != 2 {
		t.Fatalf("oturum sayısı %d", n)
	}
}

// NEDEN: kilit artık DB işlemini kapsamıyor; eşzamanlı girişler haritayı
// bozmamalı, en fazla maxYonetimSessions oturum kalmalı ve en yeni iki giriş
// geçerli olmalı.
func TestEszamanliYonetimGirisiOturumTavani(t *testing.T) {
	e, _ := yonetimEnv(t)
	secret, _, err := e.st.TOTPState(context.Background(), "mehmet")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	barrier, arrived, all := false, 0, make(chan struct{})
	// Tekrar korumasını store testleri kapsar; burada yalnız oturum haritası sınanır.
	e.srv.attemptTOTP = func(context.Context, string, string, func(string, string, time.Time) (int64, bool)) (string, bool, bool, error) {
		mu.Lock()
		wait := barrier
		if wait {
			if arrived++; arrived == 2 {
				close(all)
			}
		}
		mu.Unlock()
		if wait {
			select {
			case <-all:
			case <-time.After(5 * time.Second):
				return "", false, false, context.DeadlineExceeded
			}
		}
		return secret, true, false, nil
	}
	login := func() (int, string) {
		w := yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"000000"}`)
		var v map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v["oturum"]
	}
	var old []string
	for i := 0; i < maxYonetimSessions; i++ {
		c, tok := login()
		if c != 200 {
			t.Fatalf("ön giriş %d", c)
		}
		old = append(old, tok)
	}
	mu.Lock()
	barrier = true
	mu.Unlock()
	type sonuc struct {
		code int
		tok  string
	}
	results := make(chan sonuc, 2)
	for i := 0; i < 2; i++ {
		go func() {
			c, tok := login()
			results <- sonuc{c, tok}
		}()
	}
	var fresh []string
	for i := 0; i < 2; i++ {
		r := <-results
		if r.code != 200 {
			t.Fatalf("eşzamanlı giriş %d (ikisi aynı anda DB'ye girememiş olabilir)", r.code)
		}
		fresh = append(fresh, r.tok)
	}
	e.srv.yonetimMu.Lock()
	n := len(e.srv.yonetimSessions["mehmet"])
	e.srv.yonetimMu.Unlock()
	if n != maxYonetimSessions {
		t.Fatalf("oturum sayısı %d", n)
	}
	for _, tok := range fresh {
		yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "mehmet", tok, ""), 200, "")
	}
	valid := 0
	for _, tok := range old {
		if yonetimReq(e, "GET", "/hesaplar", "mehmet", tok, "").Code == 200 {
			valid++
		}
	}
	if valid != maxYonetimSessions-2 {
		t.Fatalf("geçerli eski oturum %d", valid)
	}
}

// NEDEN: IPv6 abonesi /64'ün içinde adres değiştirerek kimliksiz kovayı
// aşamamalı; farklı /64 ve IPv4 adresleri ayrı sayılmalı.
func TestKimliksizSayacIPv6Onek(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2001:db8:1:2::1", "2001:db8:1:2::/64"},
		{"2001:db8:1:2:ffff:ffff:ffff:ffff", "2001:db8:1:2::/64"},
		{"2001:db8:1:3::1", "2001:db8:1:3::/64"},
		{"192.0.2.1", "192.0.2.1"},
		{"::ffff:192.0.2.1", "192.0.2.1"},
		{"unknown", "unknown"},
	} {
		if got := unauthKey(tc.in); got != tc.want {
			t.Fatalf("%s: %s != %s", tc.in, got, tc.want)
		}
	}
	e := newEnv(t, func(c *config.Server) { c.UnauthPerMin = 2 })
	e.srv.authenticate = func(context.Context, string) (store.Agent, error) {
		return store.Agent{}, store.ErrUnauthorized
	}
	for _, tc := range []struct {
		ip   string
		want int
	}{
		{"[2001:db8:1:2::1]:1", 401}, {"[2001:db8:1:2::2]:1", 401}, {"[2001:db8:1:2:abcd::9]:1", 429},
		{"[2001:db8:1:3::1]:1", 401},
		{"192.0.2.1:1", 401}, {"192.0.2.1:2", 401}, {"192.0.2.1:3", 429}, {"192.0.2.2:1", 401},
	} {
		if got := dene(e.srv, tc.ip, "yanlis"); got != tc.want {
			t.Fatalf("%s: %d != %d", tc.ip, got, tc.want)
		}
	}
}

// NEDEN: kovalar dolu değilken de rastgele token'larla paralel istekler tek
// DB bağlantısını kuyruğa sokabilir. Tanınmayan token'ların eşzamanlı kimlik
// denetimi tavanlı olmalı; tanınan geçerli token tavana takılmamalı.
func TestKimliksizEszamanlilikTavani(t *testing.T) {
	e := newEnv(t, nil)
	sinirBekle(t, sinirIstek(e.srv, "GET", "/v1/me", "127.0.0.1:1", e.tokens["ali"], ""), 200, "")
	e.srv.authSlots = make(chan struct{}, 1)
	slow, other := strings.Repeat("a", 30), strings.Repeat("b", 30)
	entered, release := make(chan struct{}), make(chan struct{})
	gercek := e.srv.authenticate
	e.srv.authenticate = func(ctx context.Context, tok string) (store.Agent, error) {
		if tok == e.tokens["ali"] {
			return gercek(ctx, tok)
		}
		if tok == slow {
			close(entered)
			<-release
		}
		return store.Agent{}, store.ErrUnauthorized
	}
	done := make(chan int, 1)
	go func() { done <- sinirIstek(e.srv, "GET", "/v1/me", "192.0.2.1:1", slow, "").Code }()
	<-entered
	sinirBekle(t, sinirIstek(e.srv, "GET", "/v1/me", "192.0.2.2:1", other, ""), 429, "kimliksiz_hiz_siniri")
	w := sinirIstek(e.srv, "GET", "/v1/me", "192.0.2.3:1", e.tokens["ali"], "")
	close(release)
	sinirBekle(t, w, 200, "")
	if c := <-done; c != 401 {
		t.Fatalf("yavaş istek: %d", c)
	}
	// Tavan dolu olduğu için reddedilen istek başarısızlık sayılmadı; yer boşalınca denetlenir.
	sinirBekle(t, sinirIstek(e.srv, "GET", "/v1/me", "192.0.2.2:1", other, ""), 401, "yetkisiz")
}

package server

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// NEDEN: temel akış çalışmazsa geri kalan her şey anlamsız. Gönderilen mesaj alıcıya
// ulaşmalı, göndericiye kendi mesajı geri gelmemeli.
func TestGonderVeBekle(t *testing.T) {
	e := newEnv(t, nil)
	st, m := e.send("r1", "ali", map[string]any{"type": "soru", "to": "veli", "body": "selam"})
	if st != 201 || m["thread"] != "T-1" || num(m["turn"]) != 1 || num(m["max_turns"]) != 20 || m["from"] != "ali" {
		t.Fatalf("beklenmeyen yanıt: %d %v", st, m)
	}
	got := e.fetch("r1", "veli", 0, 0)
	if len(got) != 1 || got[0]["body"] != "selam" || got[0]["reply_to"] != nil {
		t.Fatalf("veli mesajı görmeli: %v", got)
	}
	if own := e.fetch("r1", "ali", 0, 0); len(own) != 0 {
		t.Fatalf("gönderen kendi mesajını görmemeli: %v", own)
	}
	if after := e.fetch("r1", "veli", num(m["id"]), 0); len(after) != 0 {
		t.Fatalf("after sonrası boş olmalı: %v", after)
	}
}

// NEDEN: asıl amaç uyandırma. Bekleyen istemci, yoklama aralığını beklemeden mesajla
// birlikte hemen dönmeli.
func TestBeklemeMesajGelinceHemenDoner(t *testing.T) {
	e := newEnv(t, nil)
	type res struct {
		msgs []map[string]any
		took time.Duration
	}
	ch := make(chan res, 1)
	go func() {
		start := time.Now()
		msgs := e.fetch("r1", "veli", 0, 8)
		ch <- res{msgs, time.Since(start)}
	}()
	eventually(t, 2*time.Second, "veli bekliyor görünmeli", func() bool { return e.isWaiting("r1", "veli") })
	sent := time.Now()
	if st, _ := e.send("r1", "ali", map[string]any{"body": "uyan"}); st != 201 {
		t.Fatalf("gönderim %d", st)
	}
	select {
	case r := <-ch:
		if len(r.msgs) != 1 {
			t.Fatalf("mesaj bekleniyordu: %v", r.msgs)
		}
		if d := time.Since(sent); d > 2*time.Second {
			t.Fatalf("uyanma çok yavaş: %v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bekleyen uyanmadı")
	}
}

// NEDEN: "kontrol et, sonra bekle" arasında gelen mesaj kaçarsa ajan sonsuza dek uyumaz
// ama dakikalarca gecikir. Gönderim ve bekleme eşzamanlı başlatılıp çok kez denenir;
// hepsinde bekleyen hemen dönmeli.
func TestKacirilanBildirimOlmaz(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.RatePerMin = 1000; c.RatePerDay = 10000 })
	after := 0
	for i := 0; i < 40; i++ {
		done := make(chan []map[string]any, 1)
		start := make(chan struct{})
		go func() {
			<-start
			done <- e.fetch("r1", "veli", after, 8)
		}()
		go func() {
			<-start
			time.Sleep(time.Duration(i%4) * time.Millisecond)
			e.send("r1", "ali", map[string]any{"body": "x"})
		}()
		t0 := time.Now()
		close(start)
		select {
		case msgs := <-done:
			if len(msgs) != 1 {
				t.Fatalf("tur %d: 1 mesaj bekleniyordu: %v", i, msgs)
			}
			after = num(msgs[0]["id"])
			if d := time.Since(t0); d > 3*time.Second {
				t.Fatalf("tur %d: geç uyandı (%v), bildirim kaçmış olabilir", i, d)
			}
		case <-time.After(6 * time.Second):
			t.Fatalf("tur %d: bildirim kaçtı", i)
		}
	}
}

// NEDEN: kimlik taklidi (T7). Gövdedeki "from" yok sayılıp token'daki ad kullanılmalı.
func TestGonderenTaklitEdilemez(t *testing.T) {
	e := newEnv(t, nil)
	st, m := e.send("r1", "ali", map[string]any{"from": "veli", "body": "ben veli'yim"})
	if st != 201 || m["from"] != "ali" {
		t.Fatalf("from token'dan gelmeli: %d %v", st, m)
	}
	got := e.fetch("r1", "veli", 0, 0)
	if len(got) != 1 || got[0]["from"] != "ali" {
		t.Fatalf("kayıtlı from yanlış: %v", got)
	}
}

// NEDEN: yanlış, eksik ya da bozuk token hiçbir veri sızdırmadan 401 almalı.
func TestYanlisToken401(t *testing.T) {
	e := newEnv(t, nil)
	for _, tok := range []string{"", "yanlis", e.tokens["ali"] + "x", strings.Repeat("a", 300)} {
		st, b := e.raw("GET", "/v1/rooms/r1/messages", tok, nil)
		if st != 401 || strings.TrimSpace(string(b)) != `{"error":"yetkisiz"}` {
			t.Fatalf("token %.8q: %d %s", tok, st, b)
		}
	}
}

// NEDEN: iptal (T2) anında etkili olmalı; çalınan token'ın tek çaresi bu.
func TestIptalEdilenAjan401(t *testing.T) {
	e := newEnv(t, nil)
	if st, _ := e.call("GET", "/v1/rooms/r1/agents", "veli", nil); st != 200 {
		t.Fatalf("iptalden önce %d", st)
	}
	if err := e.st.RevokeAgent(context.Background(), "veli"); err != nil {
		t.Fatal(err)
	}
	if st, _ := e.call("GET", "/v1/rooms/r1/agents", "veli", nil); st != 401 {
		t.Fatalf("iptal edilen 401 almalı, %d", st)
	}
	if st, _ := e.send("r1", "veli", map[string]any{}); st != 401 {
		t.Fatalf("iptal edilen gönderemez, %d", st)
	}
}

// NEDEN: ajan yalnız kendi odalarına erişir; başka odanın mesajı okunmamalı/yazılmamalı.
func TestBaskaOda403(t *testing.T) {
	e := newEnv(t, nil)
	e.send("r2", "ali", map[string]any{"body": "r2 sırrı"})
	if st, _ := e.send("r1", "dis", map[string]any{}); st != 403 {
		t.Fatalf("başka odaya yazma: %d", st)
	}
	if st, _ := e.call("GET", "/v1/rooms/r1/messages", "dis", nil); st != 403 {
		t.Fatalf("başka odayı okuma: %d", st)
	}
	if st, _ := e.call("GET", "/v1/rooms/r1/agents", "dis", nil); st != 403 {
		t.Fatalf("başka odanın ajanları: %d", st)
	}
	if st, _ := e.call("GET", "/v1/rooms/olmayan-oda/agents", "ali", nil); st != 403 {
		t.Fatalf("olmayan oda: %d", st)
	}
}

// NEDEN: gövde sınırı (T6) kesin olmalı: sınırdaki kabul, bir bayt fazlası ve çok büyük
// gövde 413.
func TestGovdeBuyukseReddedilir413(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.MaxBody = 100 })
	if st, _ := e.send("r1", "ali", map[string]any{"body": strings.Repeat("a", 100)}); st != 201 {
		t.Fatalf("sınırdaki gövde kabul edilmeli: %d", st)
	}
	if st, m := e.send("r1", "ali", map[string]any{"body": strings.Repeat("a", 101)}); st != 413 {
		t.Fatalf("101 bayt: %d %v", st, m)
	}
	huge, _ := json.Marshal(map[string]any{"type": "bilgi", "body": strings.Repeat("b", 1<<20)})
	if st, _ := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], huge); st != 413 {
		t.Fatalf("1 MB gövde: %d", st)
	}
}

// NEDEN: var olmayan konuşmaya yazmak sessizce yeni konuşma açmamalı; ayrıca bozuk
// thread adı 400 almalı.
func TestOlmayanThread404(t *testing.T) {
	e := newEnv(t, nil)
	if st, _ := e.send("r1", "ali", map[string]any{"thread": "T-99"}); st != 404 {
		t.Fatalf("T-99: %d", st)
	}
	if st, _ := e.send("r1", "ali", map[string]any{"thread": "../x"}); st != 400 {
		t.Fatalf("bozuk thread: %d", st)
	}
	if st, _ := e.call("GET", "/v1/rooms/r1/threads/T-99", "ali", nil); st != 404 {
		t.Fatalf("thread okuma: %d", st)
	}
}

// NEDEN: reply_to aynı odada var olmalı; başka odadaki mesaja bağlanmak oda ayrımını delerdi.
func TestOlmayanReplyTo400(t *testing.T) {
	e := newEnv(t, nil)
	_, other := e.send("r2", "ali", map[string]any{})
	for _, rt := range []int{9999, num(other["id"])} {
		if st, _ := e.send("r1", "ali", map[string]any{"reply_to": rt}); st != 400 {
			t.Fatalf("reply_to=%d: %d", rt, st)
		}
	}
	_, ok := e.send("r1", "ali", map[string]any{})
	if st, m := e.send("r1", "veli", map[string]any{"reply_to": ok["id"], "thread": ok["thread"]}); st != 201 || num(m["reply_to"]) != num(ok["id"]) {
		t.Fatalf("geçerli reply_to: %d %v", st, m)
	}
}

// NEDEN: kayıtsız, iptal edilmiş ya da odada olmayan alıcıya gönderim, mesajın hiç
// kimseye ulaşmadan kaybolması demek; 400 almalı.
func TestKayitsizAlici400(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.st.RevokeAgent(context.Background(), "can"); err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"hayalet", "can", "dis", "ali", "BÜYÜK"} {
		if st, _ := e.send("r1", "ali", map[string]any{"to": to}); st != 400 {
			t.Fatalf("to=%q: %d", to, st)
		}
	}
}

// NEDEN: "to" doluysa mesaj yalnız o ajana görünmeli; boşsa odadakilerin hepsine.
func TestAliciFiltresi(t *testing.T) {
	e := newEnv(t, nil)
	e.send("r1", "ali", map[string]any{"to": "veli", "body": "yalnız veli"})
	e.send("r1", "ali", map[string]any{"body": "herkese"})
	bodies := func(agent string) string {
		var b []string
		for _, m := range e.fetch("r1", agent, 0, 0) {
			b = append(b, m["body"].(string))
		}
		return strings.Join(b, "|")
	}
	if got := bodies("veli"); got != "yalnız veli|herkese" {
		t.Fatalf("veli: %q", got)
	}
	if got := bodies("can"); got != "herkese" {
		t.Fatalf("can yalnız yayını görmeli: %q", got)
	}
	if got := bodies("ali"); got != "" {
		t.Fatalf("ali kendi mesajlarını görmemeli: %q", got)
	}
}

// NEDEN: POST hız sınırı (T4/T6): sınırdan sonrası 429; GET'ler sınırdan etkilenmez.
func TestHizSiniri429(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.RatePerMin = 3 })
	for i := 0; i < 3; i++ {
		if st, _ := e.send("r1", "ali", map[string]any{}); st != 201 {
			t.Fatalf("%d. gönderim: %d", i+1, st)
		}
	}
	if st, m := e.send("r1", "ali", map[string]any{}); st != 429 || m["error"] != "hiz_siniri" {
		t.Fatalf("4. gönderim: %d %v", st, m)
	}
	if st, _ := e.send("r1", "veli", map[string]any{}); st != 201 {
		t.Fatalf("başka ajan etkilenmemeli: %d", st)
	}
	if st, _ := e.call("GET", "/v1/rooms/r1/agents", "ali", nil); st != 200 {
		t.Fatalf("GET sınırlanmamalı: %d", st)
	}
}

// NEDEN: kayan pencere mantığı (dakika ve gün) saat enjekte edilerek doğrulanır; gerçek
// zamanı beklemeden pencerenin gerçekten kaydığı görülür.
func TestHizSiniriPencereKayar(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	l := newRateLimiter(2, 3)
	l.now = func() time.Time { return now }
	if !l.Allow("a") || !l.Allow("a") || l.Allow("a") {
		t.Fatal("dakika sınırı 2 olmalı")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("a") {
		t.Fatal("dakika penceresi kaymalı")
	}
	if l.Allow("a") {
		t.Fatal("gün sınırı (3) aşıldı, reddedilmeli")
	}
	now = now.Add(25 * time.Hour)
	if !l.Allow("a") {
		t.Fatal("gün penceresi kaymalı")
	}
}

// NEDEN: ajan başına eşzamanlı bağlantı sınırı (T6): takılı long-poll'lar sunucuyu
// tüketemesin.
func TestEszamanliBaglantiSiniri429(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.MaxConnsPerAgent = 2 })
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.rawCtx(ctx, "GET", "/v1/rooms/r1/messages?wait=8", e.tokens["veli"], nil)
		}()
	}
	eventually(t, 3*time.Second, "iki bekleme açık", func() bool {
		e.srv.conns.mu.Lock()
		defer e.srv.conns.mu.Unlock()
		return e.srv.conns.n["veli"] == 2
	})
	if st, m := e.call("GET", "/v1/rooms/r1/agents", "veli", nil); st != 429 || m["error"] != "cok_fazla_baglanti" {
		t.Fatalf("üçüncü eşzamanlı istek: %d %v", st, m)
	}
	if st, _ := e.call("GET", "/v1/rooms/r1/agents", "can", nil); st != 200 {
		t.Fatalf("sınır ajan başınadır, can etkilenmemeli: %d", st)
	}
	cancel()
	wg.Wait()
	eventually(t, 3*time.Second, "bağlantılar serbest kalmalı", func() bool {
		st, _ := e.call("GET", "/v1/rooms/r1/agents", "veli", nil)
		return st == 200
	})
}

// NEDEN: tur sınırı (T4) döngüleri keser: sınıra ulaşan konuşma kilitlenir; yalnız admin
// reopen ile (sınır max_turns kadar artarak) açılabilir.
func TestTurSiniriKilitlerVeReopenAcar(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.MaxTurns = 3 })
	_, m := e.send("r1", "ali", map[string]any{})
	th := m["thread"].(string)
	for i := 2; i <= 3; i++ {
		who := []string{"", "", "veli", "ali"}[i]
		if st, r := e.send("r1", who, map[string]any{"thread": th}); st != 201 || num(r["turn"]) != i {
			t.Fatalf("tur %d: %d %v", i, st, r)
		}
	}
	_, info := e.call("GET", "/v1/rooms/r1/threads/"+th, "ali", nil)
	if info["status"] != "kilitli" || num(info["turns"]) != 3 {
		t.Fatalf("kilitli olmalı: %v", info)
	}
	if st, r := e.send("r1", "veli", map[string]any{"thread": th}); st != 409 || r["error"] != "konusma_kilitli" {
		t.Fatalf("kilitli konuşma: %d %v", st, r)
	}
	if err := e.st.ReopenThread(context.Background(), "r1", th, 3); err != nil {
		t.Fatal(err)
	}
	if st, r := e.send("r1", "veli", map[string]any{"thread": th}); st != 201 || num(r["turn"]) != 4 || num(r["max_turns"]) != 6 {
		t.Fatalf("reopen sonrası: %d %v", st, r)
	}
}

// NEDEN: `kapat` mesajı konuşmayı bitirir; sonrasında yazmak 409 almalı (Faz 0 davranışı).
func TestKapatKilitler(t *testing.T) {
	e := newEnv(t, nil)
	_, m := e.send("r1", "ali", map[string]any{"type": "soru", "to": "veli"})
	th := m["thread"].(string)
	if st, _ := e.send("r1", "veli", map[string]any{"type": "kapat", "thread": th}); st != 201 {
		t.Fatalf("kapat: %d", st)
	}
	_, info := e.call("GET", "/v1/rooms/r1/threads/"+th, "ali", nil)
	if info["status"] != "kapali" {
		t.Fatalf("durum: %v", info)
	}
	if st, _ := e.send("r1", "ali", map[string]any{"thread": th}); st != 409 {
		t.Fatalf("kapalı konuşmaya yazma: %d", st)
	}
}

// NEDEN: konuşmayı yalnız tarafları ya da bir insan kapatabilir; üçüncü ajan kapatamaz.
func TestKapatmaYetkisi(t *testing.T) {
	e := newEnv(t, nil)
	_, m := e.send("r1", "ali", map[string]any{"to": "veli"})
	th := m["thread"].(string)
	path := "/v1/rooms/r1/threads/" + th + "/close"
	if st, _ := e.call("POST", path, "can", nil); st != 403 {
		t.Fatalf("taraf olmayan ajan: %d", st)
	}
	if st, _ := e.call("POST", "/v1/rooms/r1/threads/T-77/close", "ali", nil); st != 404 {
		t.Fatalf("olmayan konuşma: %d", st)
	}
	if st, r := e.call("POST", path, "veli", nil); st != 200 || r["status"] != "kapali" {
		t.Fatalf("hedef olan ajan: %d %v", st, r)
	}
	_, m2 := e.send("r1", "ali", map[string]any{})
	if st, _ := e.call("POST", "/v1/rooms/r1/threads/"+m2["thread"].(string)+"/close", "mehmet", nil); st != 200 {
		t.Fatalf("insan kapatabilmeli: %d", st)
	}
	_, m3 := e.send("r1", "ali", map[string]any{})
	if st, _ := e.call("POST", "/v1/rooms/r1/threads/"+m3["thread"].(string)+"/close", "ali", nil); st != 200 {
		t.Fatalf("gönderen kapatabilmeli: %d", st)
	}
}

// NEDEN: Faz 0 dersi: bekleyen long-poll'ları görebilmek lazım. waiting yalnız aktif
// bekleme sırasında true olmalı.
func TestAgentsWaitingGosterir(t *testing.T) {
	e := newEnv(t, nil)
	if e.isWaiting("r1", "veli") {
		t.Fatal("başta kimse beklemiyor")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.rawCtx(ctx, "GET", "/v1/rooms/r1/messages?wait=8", e.tokens["veli"], nil)
	}()
	eventually(t, 3*time.Second, "waiting=true", func() bool { return e.isWaiting("r1", "veli") })
	_, out := e.call("GET", "/v1/rooms/r1/agents", "ali", nil)
	names := []string{}
	for _, a := range toList(out["agents"]) {
		names = append(names, a["name"].(string))
		if a["name"] == "ali" && a["waiting"] != false {
			t.Fatal("ali beklemiyor")
		}
		if _, ok := a["last_seen"]; !ok || a["kind"] == nil {
			t.Fatalf("alanlar eksik: %v", a)
		}
	}
	if strings.Join(names, ",") != "ali,can,mehmet,veli" {
		t.Fatalf("odadaki ajanlar: %v", names)
	}
	cancel()
	<-done
	eventually(t, 3*time.Second, "waiting=false", func() bool { return !e.isWaiting("r1", "veli") })
}

// NEDEN: istemci kopunca (r.Context) sunucu beklemeyi bırakmalı; yoksa ölü bağlantılar
// bekleyen olarak birikir ve bağlantı sınırını doldurur.
func TestIstemciKoparsaBeklemeBirakilir(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.MaxConnsPerAgent = 1 })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.rawCtx(ctx, "GET", "/v1/rooms/r1/messages?wait=8", e.tokens["veli"], nil)
	}()
	eventually(t, 3*time.Second, "bekleme açıldı", func() bool { return e.isWaiting("r1", "veli") })
	cancel()
	<-done
	eventually(t, 3*time.Second, "bekleme bırakıldı ve bağlantı yeri serbest", func() bool {
		st, _ := e.call("GET", "/v1/rooms/r1/messages?wait=0", "veli", nil)
		return st == 200 && !e.isWaiting("r1", "veli")
	})
}

// NEDEN: saklama süresi dolan mesajlar silinmeli (gizlilik); yeniler kalmalı.
func TestSaklamaEskiMesajlariSiler(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.RetentionDays = 14 })
	ctx := context.Background()
	e.st.Now = func() time.Time { return time.Now().Add(-20 * 24 * time.Hour) }
	if _, err := e.st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "eski"}); err != nil {
		t.Fatal(err)
	}
	e.st.Now = func() time.Time { return time.Now().Add(-13 * 24 * time.Hour) }
	if _, err := e.st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "yeni"}); err != nil {
		t.Fatal(err)
	}
	e.st.Now = time.Now
	e.srv.PurgeOld(ctx)
	got := e.fetch("r1", "veli", 0, 0)
	if len(got) != 1 || got[0]["body"] != "yeni" {
		t.Fatalf("yalnız yeni mesaj kalmalı: %v", got)
	}
}

// NEDEN: loglar gizli veri (T2/T5) sızdırmamalı: gövde, token ve Authorization asla;
// query string de yok. Ama ajan adı, yol ve durum olmalı.
func TestLogGovdeVeTokenIcermez(t *testing.T) {
	e := newEnv(t, nil)
	e.send("r1", "ali", map[string]any{"body": "GIZLI-GOVDE-123"})
	e.raw("GET", "/v1/rooms/r1/messages?after=0&wait=0&gizli=SORGU-456", e.tokens["veli"], nil)
	e.raw("GET", "/v1/rooms/r1/messages", "YANLIS-TOKEN-789", nil)
	out := e.logs.String()
	for _, sec := range []string{"GIZLI-GOVDE-123", "SORGU-456", "YANLIS-TOKEN-789", e.tokens["ali"], e.tokens["veli"], "Authorization", "Bearer"} {
		if contains(out, sec) {
			t.Fatalf("log %q içermemeli:\n%s", sec, out)
		}
	}
	for _, want := range []string{"method=POST", "path=/v1/rooms/r1/messages", "agent=ali", "status=201", "status=401"} {
		if !contains(out, want) {
			t.Fatalf("log %q içermeli:\n%s", want, out)
		}
	}
}

// NEDEN: health token istemez ama sürüm, yol ya da sayaç gibi hiçbir şey sızdırmamalı.
func TestHealthBilgiSizdirmaz(t *testing.T) {
	e := newEnv(t, nil)
	st, b := e.raw("GET", "/v1/health", "", nil)
	if st != 200 || strings.TrimSpace(string(b)) != `{"ok":true}` {
		t.Fatalf("health: %d %s", st, b)
	}
	if st, b := e.raw("GET", "/v1/bilinmeyen", "", nil); st != 404 || strings.TrimSpace(string(b)) != `{"error":"bulunamadi"}` {
		t.Fatalf("bilinmeyen yol: %d %s", st, b)
	}
}

// NEDEN: sunucu yeniden başlayınca mesajlar, konuşmalar ve sayaçlar kalmalı (kalıcılık).
func TestSunucuYenidenBaslayincaMesajlarKalir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "relay.db")
	ctx := context.Background()
	st1, err := store.Open(dbPath, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := st1.AddAgent(ctx, "ali", "ajan", store.HashToken("tok-ali"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	m1, err := st1.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "soru", Body: "kalıcı mı?"})
	if err != nil {
		t.Fatal(err)
	}
	st1.Close()

	st2, err := store.Open(dbPath, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	if a, err := st2.Authenticate(ctx, "tok-ali"); err != nil || a.Name != "ali" {
		t.Fatalf("ajan kalıcı olmalı: %v %v", a, err)
	}
	m2, err := st2.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Thread: m1.Thread, Body: "evet"})
	if err != nil {
		t.Fatal(err)
	}
	if m2.ID <= m1.ID || m2.Turn != 2 || m2.Thread != m1.Thread {
		t.Fatalf("sayaçlar devam etmeli: %+v %+v", m1, m2)
	}
	msgs, err := st2.Fetch(ctx, "r1", "veli", 0, 10)
	if err != nil || len(msgs) != 2 || msgs[0].Body != "kalıcı mı?" {
		t.Fatalf("mesajlar kalmalı: %v %v", msgs, err)
	}
}

// NEDEN: aynı konuşmaya aynı anda yazan ajanlarda tur numaraları benzersiz ve ardışık
// olmalı, sınırı aşanlar 409 almalı, id'ler çakışmamalı (tur sayacı + ekleme tek transaction).
func TestEszamanliGonderimdeIdVeTurTutarli(t *testing.T) {
	e := newEnv(t, func(c *config.Server) {
		c.MaxTurns = 10
		c.RatePerMin = 1000
		c.RatePerDay = 10000
		c.MaxConnsPerAgent = 50
	})
	_, first := e.send("r1", "ali", map[string]any{})
	th := first["thread"].(string)

	const n = 20
	type res struct {
		status int
		turn   int
		id     int
	}
	results := make(chan res, n)
	start := make(chan struct{})
	agents := []string{"ali", "veli", "can"}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			st, m := e.send("r1", agents[i%3], map[string]any{"thread": th})
			results <- res{st, num(m["turn"]), num(m["id"])}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	turns, ids, ok, conflict := map[int]bool{1: true}, map[int]bool{num(first["id"]): true}, 0, 0
	for r := range results {
		switch r.status {
		case 201:
			ok++
			if turns[r.turn] || ids[r.id] {
				t.Fatalf("tekrarlanan tur/id: %+v", r)
			}
			turns[r.turn], ids[r.id] = true, true
		case 409:
			conflict++
		default:
			t.Fatalf("beklenmeyen durum %d", r.status)
		}
	}
	if ok != 9 || conflict != n-9 {
		t.Fatalf("201=%d 409=%d, beklenen 9/%d", ok, conflict, n-9)
	}
	for turn := 1; turn <= 10; turn++ {
		if !turns[turn] {
			t.Fatalf("tur %d eksik: %v", turn, turns)
		}
	}
	_, info := e.call("GET", "/v1/rooms/r1/threads/"+th, "ali", nil)
	if info["status"] != "kilitli" || num(info["turns"]) != 10 {
		t.Fatalf("konuşma durumu: %v", info)
	}
}

// NEDEN: kapanışta (SIGTERM) bekleyen long-poll'lar uyanıp yanıtlanmalı; Serve temiz dönmeli,
// yoksa systemd durdurma süresini aşar.
func TestKapanistaBekleyenlerUyanir(t *testing.T) {
	e := newEnv(t, nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	srv := New(e.cfg, e.st, e.srv.log)
	go func() { served <- srv.Serve(ctx, ln) }()

	got := make(chan int, 1)
	go func() {
		st, _, err := rawURL(context.Background(), "http://"+ln.Addr().String()+"/v1/rooms/r1/messages?wait=8", e.tokens["veli"])
		if err != nil {
			st = -1
		}
		got <- st
	}()
	eventually(t, 3*time.Second, "bekleme başladı", func() bool { return srv.hub.Waiting("r1")["veli"] })
	t0 := time.Now()
	cancel()
	select {
	case st := <-got:
		if st != 200 || time.Since(t0) > 3*time.Second {
			t.Fatalf("bekleyen düzgün yanıtlanmalı: %d %v", st, time.Since(t0))
		}
	case <-time.After(6 * time.Second):
		t.Fatal("bekleyen uyanmadı")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Serve dönmedi")
	}
}

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// sayfaSiniri tek sayfalı yanıtın bayt tavanıdır: gövde bütçesi + JSON zarfı payı.
const sayfaSiniri = store.MaxPageBytes + 4096

type sayfa struct {
	Messages []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	} `json:"messages"`
	HasMore bool `json:"has_more"`
}

// duyurular T-0'a n adet, her biri size baytlık '<' gövdeli duyuru yazar; id'leri döndürür.
func duyurular(t *testing.T, e *testEnv, n, size int) []int64 {
	t.Helper()
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		m, err := e.st.Post(context.Background(), store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: strings.Repeat("<", size), Announcement: true})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	return ids
}

func (e *testEnv) sayfa(path, who string) (sayfa, int) {
	e.t.Helper()
	status, raw := e.raw("GET", path, e.tokens[who], nil)
	if status != 200 {
		e.t.Fatalf("%s: durum %d: %.200s", path, status, raw)
	}
	var p sayfa
	if err := json.Unmarshal(raw, &p); err != nil {
		e.t.Fatal(err)
	}
	return p, len(raw)
}

// NEDEN: '<', '>' ve '&' 6 bayta kaçırılınca 16 KB gövde 98 KB yanıta dönüşüyordu.
// Yanıt JSON + nosniff olduğundan HTML kaçışı gereksizdir.
func Test_yanit_html_kacisiyla_buyumez(t *testing.T) {
	e := newEnv(t, nil)
	body := strings.Repeat("<>&", e.cfg.MaxBody/3)
	if _, err := e.st.Post(context.Background(), store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: body}); err != nil {
		t.Fatal(err)
	}
	status, raw := e.raw("GET", "/v1/rooms/r1/messages?after=0", e.tokens["veli"], nil)
	if status != 200 || len(raw) > len(body)+1024 {
		t.Fatalf("durum %d, %d bayt gövde için %d bayt yanıt", status, len(body), len(raw))
	}
	var p sayfa
	if err := json.Unmarshal(raw, &p); err != nil || len(p.Messages) != 1 || p.Messages[0].Body != body {
		t.Fatalf("gövde bozuldu: %v", err)
	}
}

// NEDEN: sayfa sınırı yalnız adetti (100); 100 x 16 KB tek yanıtta dönüp sunucuyu
// MemoryMax üstüne çıkarabiliyordu. Bayt sınırı sayfayı kısaltır ama imleçler
// (after / tail / before) hiçbir mesajı atlamamalı ya da yinelememeli.
func Test_sayfalar_bayt_sinirli_ve_eksiksiz(t *testing.T) {
	e := newEnv(t, nil)
	want := duyurular(t, e, 40, e.cfg.MaxBody) // 640 KB > sınır

	ileri := func(who, base string) {
		t.Helper()
		var got []int64
		pages := 0
		for after := int64(0); ; pages++ {
			p, size := e.sayfa(fmt.Sprintf("%s?after=%d", base, after), who)
			if size > sayfaSiniri {
				t.Fatalf("%s: sayfa %d bayt", base, size)
			}
			if len(p.Messages) == 0 {
				break
			}
			for _, m := range p.Messages {
				got = append(got, m.ID)
				after = m.ID // istemciler imleci son görülen id'ye ilerletir
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(want) || pages < 2 {
			t.Fatalf("%s: %d sayfada %v", base, pages, got)
		}
	}
	ileri("veli", "/v1/rooms/r1/messages")
	ileri("mehmet", "/v1/rooms/r1/log")

	// Geri yön: izle.js ilk sayfayı (tail / imleçsiz) alır, sonra before=<en eski id> ile iner.
	geri := func(who, first, base string) {
		t.Helper()
		var got []int64
		path := first
		for pages := 1; ; pages++ {
			p, size := e.sayfa(path, who)
			if size > sayfaSiniri || len(p.Messages) == 0 {
				t.Fatalf("%s: %d bayt, %d mesaj", path, size, len(p.Messages))
			}
			page := make([]int64, 0, len(p.Messages))
			for _, m := range p.Messages {
				page = append(page, m.ID)
			}
			got = append(page, got...)
			if !p.HasMore {
				if pages < 2 {
					t.Fatalf("%s: tek sayfa", first)
				}
				break
			}
			path = fmt.Sprintf("%s?before=%d", base, page[0])
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: %v", first, got)
		}
	}
	geri("mehmet", "/v1/rooms/r1/log?tail=1", "/v1/rooms/r1/log")
	geri("mehmet", "/v1/rooms/r1/threads/T-0/log", "/v1/rooms/r1/threads/T-0/log")
	geri("veli", "/v1/rooms/r1/threads/T-0", "/v1/rooms/r1/threads/T-0")
}

// NEDEN: tek mesaj bütçeyi aşsa bile sayfa boş dönmemeli; yoksa imleç ilerleyemez.
func Test_butceyi_asan_tek_mesaj_yine_doner(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.MaxBody = 2 * store.MaxPageBytes })
	want := duyurular(t, e, 3, 2*store.MaxPageBytes)
	for i, path := range []string{"/v1/rooms/r1/messages?after=0", fmt.Sprintf("/v1/rooms/r1/messages?after=%d", want[0]), "/v1/rooms/r1/threads/T-0"} {
		p, _ := e.sayfa(path, "veli")
		if len(p.Messages) != 1 || p.Messages[0].ID != want[i] {
			t.Fatalf("%s: %d mesaj", path, len(p.Messages))
		}
	}
}

// NEDEN: konuşma ucu LIMIT'sizdi; T-0'da tur sınırı olmadığından tek GET bütün
// saklama süresini döndürüyordu. Artık en yeni 100 mesaj ve has_more döner;
// eskiler before ile aynı uçtan okunur.
func Test_konusma_ucu_adet_sinirli(t *testing.T) {
	e := newEnv(t, nil)
	want := duyurular(t, e, maxFetch+50, 8)
	p, _ := e.sayfa("/v1/rooms/r1/threads/T-0", "veli")
	if len(p.Messages) != maxFetch || !p.HasMore || p.Messages[0].ID != want[50] || p.Messages[maxFetch-1].ID != want[len(want)-1] {
		t.Fatalf("%d mesaj, has_more=%v", len(p.Messages), p.HasMore)
	}
	p, _ = e.sayfa(fmt.Sprintf("/v1/rooms/r1/threads/T-0?before=%d", p.Messages[0].ID), "veli")
	if len(p.Messages) != 50 || p.HasMore || p.Messages[0].ID != want[0] || p.Messages[49].ID != want[49] {
		t.Fatalf("eski sayfa: %d mesaj, has_more=%v", len(p.Messages), p.HasMore)
	}
	for _, bad := range []string{"0", "-1", "x"} {
		if status, _ := e.raw("GET", "/v1/rooms/r1/threads/T-0?before="+bad, e.tokens["veli"], nil); status != 400 {
			t.Fatalf("before=%s: %d", bad, status)
		}
	}
}

// NEDEN: /v1/unread LIMIT'sizdi; binlerce konuşmada çağrı başına MB'larca JSON üretip
// tek SQLite bağlantısını tutuyordu.
func Test_okunmamis_ozeti_sinirli(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	baslik := strings.Repeat("ğ", 180)
	for i := 0; i < store.MaxSummaries+5; i++ {
		if _, err := e.st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: baslik}); err != nil {
			t.Fatal(err)
		}
	}
	status, raw := e.raw("GET", "/v1/unread", e.tokens["mehmet"], nil)
	var out struct {
		Threads []struct {
			Title string `json:"title"`
		} `json:"threads"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	if len(out.Threads) != store.MaxSummaries || !out.Truncated {
		t.Fatalf("%d konuşma, truncated=%v", len(out.Threads), out.Truncated)
	}
	for _, th := range out.Threads {
		if len(th.Title) > 256 || !utf8.ValidString(th.Title) || strings.ContainsRune(th.Title, utf8.RuneError) || strings.Trim(th.Title, "ğ") != "" {
			t.Fatalf("başlık %d bayt: %q", len(th.Title), th.Title)
		}
	}
}

// NEDEN: 106752 gün ve üstünde time.Duration taşıyor, kesim geleceğe düşüyor ve
// bütün mesajlar siliniyordu.
func Test_saklama_kesimi_tasmaz(t *testing.T) {
	e := newEnv(t, func(c *config.Server) { c.RetentionDays = 106752 })
	duyurular(t, e, 1, 8)
	e.srv.PurgeOld(context.Background())
	if p, _ := e.sayfa("/v1/rooms/r1/log", "mehmet"); len(p.Messages) != 1 {
		t.Fatalf("yeni mesaj silindi: %d kaldı", len(p.Messages))
	}
}

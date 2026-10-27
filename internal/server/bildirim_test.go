package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// bildirimEnv genel ve duyuru kotalarını yükseltir; testler yalnız biçimi sınar.
func bildirimEnv(t *testing.T) *testEnv {
	t.Helper()
	return newEnv(t, func(c *config.Server) {
		c.RatePerMin = 1000
		c.RatePerDay = 1000
		c.AnnouncePerMin = 1000
		c.AnnouncePerDay = 1000
	})
}

func bildirimBody(extra string, i int) []byte {
	return []byte(fmt.Sprintf(`{"type":"bildirim",%s"body":"bildirim %d"}`, extra, i))
}

func roomLogLen(t *testing.T, e *testEnv) int {
	t.Helper()
	msgs, err := e.st.RoomLog(context.Background(), "r1", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	return len(msgs)
}

// NEDEN: Bildirimler sekmesi T-0'daki "bildirim" mesajlarından oluşur; ajanın
// /announcements ucundan gönderdiği bildirim T-0'a düşmeli ve tur sınırı almamalı.
func TestBildirimAnnouncementGoesToT0(t *testing.T) {
	e := bildirimEnv(t)
	for _, who := range []string{"ali", "mehmet"} {
		status, raw := e.raw("POST", announcePath, e.tokens[who], bildirimBody("", 0))
		var m store.Message
		_ = json.Unmarshal(raw, &m)
		if status != 201 || m.Thread != "T-0" || m.Type != "bildirim" || m.MaxTurns != 0 {
			t.Fatal(who, status, string(raw))
		}
	}
}

// NEDEN: store T-0'ı /messages yolunda thread "T-0" ile de tanır; bildirim bu
// yoldan da kabul edilmeli, yoksa iki yolun davranışı ayrışır.
func TestBildirimViaMessagesThreadT0(t *testing.T) {
	e := bildirimEnv(t)
	if status, raw := e.raw("POST", announcePath, e.tokens["mehmet"], announceBody(0)); status != 201 {
		t.Fatal(status, string(raw))
	}
	for _, who := range []string{"ali", "mehmet"} {
		status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens[who], bildirimBody(`"thread":"T-0",`, 1))
		var m store.Message
		_ = json.Unmarshal(raw, &m)
		if status != 201 || m.Thread != "T-0" || m.Type != "bildirim" {
			t.Fatal(who, status, string(raw))
		}
	}
}

// NEDEN: bildirim dinleyen ajanı uyandırır; yeni konuda kabul edilirse herhangi
// bir ajan sıradan bir konuşmayı bildirim gibi gösterip herkesi uyandırabilirdi.
// Red mesaj yazılmadan ve konu numarası harcanmadan olmalı.
func TestBildirimRejectedForNewTopic(t *testing.T) {
	e := bildirimEnv(t)
	for _, who := range []string{"ali", "mehmet"} {
		status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens[who], bildirimBody("", 0))
		if status != 400 || !contains(string(raw), "gecersiz_duyuru") {
			t.Fatal(who, status, string(raw))
		}
	}
	if n := roomLogLen(t, e); n != 0 {
		t.Fatal("reddedilen bildirim yazıldı", n)
	}
	status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], []byte(`{"type":"soru","body":"normal"}`))
	var m store.Message
	_ = json.Unmarshal(raw, &m)
	if status != 201 || m.Thread != "T-1" {
		t.Fatal("konu numarası harcandı", status, string(raw))
	}
}

// NEDEN: var olan bir T-n konuşmasına bildirim de aynı uyandırma kötüye
// kullanımını açar; yalnız T-0 kabul edilmeli, tur harcanmamalı.
func TestBildirimRejectedForExistingTopic(t *testing.T) {
	e := bildirimEnv(t)
	status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], []byte(`{"type":"soru","body":"konu"}`))
	var m store.Message
	_ = json.Unmarshal(raw, &m)
	if status != 201 {
		t.Fatal(status, string(raw))
	}
	thread := fmt.Sprintf(`"thread":%q,`, m.Thread)
	for _, who := range []string{"ali", "veli", "mehmet"} {
		status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens[who], bildirimBody(thread, 1))
		if status != 400 || !contains(string(raw), "gecersiz_duyuru") {
			t.Fatal(who, status, string(raw))
		}
	}
	if n := roomLogLen(t, e); n != 1 {
		t.Fatal("reddedilen bildirim yazıldı", n)
	}
	th, err := e.st.GetThread(context.Background(), "r1", m.Thread)
	if err != nil || th.Turns != 1 {
		t.Fatal("tur harcandı", th, err)
	}
}

// NEDEN: bildirim de duyurudur; ayrı tür olması kotayı ikiye katlamamalı ya da
// atlatmamalı. bilgi ve bildirim (iki yoldan) aynı ajan kotasından düşer.
func TestBildirimSharesAgentAnnouncementQuota(t *testing.T) {
	e := quotaEnv(t) // AnnouncePerMin = 3
	sends := []struct {
		path string
		body []byte
	}{
		{announcePath, announceBody(0)},
		{announcePath, bildirimBody("", 1)},
		{"/v1/rooms/r1/messages", bildirimBody(`"thread":"T-0",`, 2)},
	}
	for i, s := range sends {
		if status, raw := e.raw("POST", s.path, e.tokens["ali"], s.body); status != 201 {
			t.Fatal(i, status, string(raw))
		}
	}
	for _, s := range sends {
		status, raw := e.raw("POST", s.path, e.tokens["ali"], s.body)
		if status != 429 || !contains(string(raw), "duyuru_siniri") {
			t.Fatal(status, string(raw))
		}
	}
	// İnsan hesabı muaf kalır.
	if status, raw := e.raw("POST", announcePath, e.tokens["mehmet"], bildirimBody("", 9)); status != 201 {
		t.Fatal(status, string(raw))
	}
}

// NEDEN: T-0 herkese açık tek akıştır; bildirim reply_to ya da to ile belirli
// bir ajana yönlendirilemez (bilgi için geçerli kural bildirim için de geçerli).
func TestBildirimRejectsReplyToAndRecipient(t *testing.T) {
	e := bildirimEnv(t)
	status, raw := e.raw("POST", announcePath, e.tokens["mehmet"], announceBody(0))
	var first store.Message
	_ = json.Unmarshal(raw, &first)
	if status != 201 {
		t.Fatal(status, string(raw))
	}
	reply := fmt.Sprintf(`"reply_to":%d,`, first.ID)
	cases := []struct {
		path, extra string
	}{
		{announcePath, reply},
		{announcePath, `"to":"veli",`},
		{"/v1/rooms/r1/messages", `"thread":"T-0",` + reply},
		{"/v1/rooms/r1/messages", `"thread":"T-0","to":"veli",`},
	}
	for _, who := range []string{"ali", "mehmet"} {
		for i, c := range cases {
			status, raw := e.raw("POST", c.path, e.tokens[who], bildirimBody(c.extra, i))
			if status != 400 || !contains(string(raw), "gecersiz_duyuru") {
				t.Fatal(who, i, status, string(raw))
			}
		}
	}
	if n := roomLogLen(t, e); n != 1 {
		t.Fatal("reddedilen bildirim yazıldı", n)
	}
}

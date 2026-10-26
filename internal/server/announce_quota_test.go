package server

import (
	"fmt"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/config"
)

const announcePath = "/v1/rooms/r1/announcements"

// quotaEnv genel hız sınırını yükseltir; testler yalnız T-0 duyuru kotasını sınar.
func quotaEnv(t *testing.T) *testEnv {
	t.Helper()
	return newEnv(t, func(c *config.Server) {
		c.RatePerMin = 1000
		c.RatePerDay = 1000
		c.AnnouncePerMin = 3
		c.AnnouncePerDay = 50
	})
}

func announceBody(i int) []byte {
	return []byte(fmt.Sprintf(`{"type":"bilgi","body":"duyuru %d"}`, i))
}

// NEDEN: prompt injection'a uğramış bir ajan insanların okuduğu T-0 akışını
// genel sınırla (500/gün) doldurabilirdi; ajanın duyurusu ayrı, düşük kotaya takılmalı.
func TestAgentAnnouncementQuotaRejectsExtra(t *testing.T) {
	e := quotaEnv(t)
	for i := 0; i < e.cfg.AnnouncePerMin; i++ {
		if status, raw := e.raw("POST", announcePath, e.tokens["ali"], announceBody(i)); status != 201 {
			t.Fatal(i, status, string(raw))
		}
	}
	status, raw := e.raw("POST", announcePath, e.tokens["ali"], announceBody(99))
	if status != 429 || !contains(string(raw), "duyuru_siniri") {
		t.Fatal(status, string(raw))
	}
}

// NEDEN: store T-0'ı /messages yolunda thread "T-0" ile de kabul eder; kota orada
// uygulanmazsa announcements ucundaki sınır bu yoldan atlatılırdı.
func TestAgentAnnouncementQuotaCoversMessagesThreadT0(t *testing.T) {
	e := quotaEnv(t)
	// T-0'ı muaf insan hesabı açar; ajanın kotası harcanmaz.
	if status, raw := e.raw("POST", announcePath, e.tokens["mehmet"], announceBody(0)); status != 201 {
		t.Fatal(status, string(raw))
	}
	t0 := func(i int) []byte {
		return []byte(fmt.Sprintf(`{"type":"bilgi","thread":"T-0","body":"duyuru %d"}`, i))
	}
	for i := 0; i < e.cfg.AnnouncePerMin; i++ {
		path, body := announcePath, announceBody(i)
		if i%2 == 1 {
			path, body = "/v1/rooms/r1/messages", t0(i)
		}
		if status, raw := e.raw("POST", path, e.tokens["ali"], body); status != 201 {
			t.Fatal(i, status, string(raw))
		}
	}
	status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], t0(99))
	if status != 429 || !contains(string(raw), "duyuru_siniri") {
		t.Fatal(status, string(raw))
	}
	if status, _ := e.raw("POST", announcePath, e.tokens["ali"], announceBody(100)); status != 429 {
		t.Fatal(status)
	}
}

// NEDEN: kota yalnız T-0 içindir; dolması ajanın normal iş konuşmasını engellememeli.
func TestAgentAnnouncementQuotaLeavesOrdinaryTopicsOpen(t *testing.T) {
	e := quotaEnv(t)
	for i := 0; i <= e.cfg.AnnouncePerMin; i++ {
		e.raw("POST", announcePath, e.tokens["ali"], announceBody(i))
	}
	if status, _ := e.raw("POST", announcePath, e.tokens["ali"], announceBody(99)); status != 429 {
		t.Fatal("kota dolmalıydı", status)
	}
	status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], []byte(`{"type":"soru","body":"normal konu"}`))
	if status != 201 {
		t.Fatal(status, string(raw))
	}
}

// NEDEN: insanların duyuru yazması kısıtlanmamalı; kota yalnız ajan hesapları içindir.
func TestHumanAnnouncementsAreExemptFromQuota(t *testing.T) {
	e := quotaEnv(t)
	for i := 0; i < e.cfg.AnnouncePerMin+3; i++ {
		if status, raw := e.raw("POST", announcePath, e.tokens["mehmet"], announceBody(i)); status != 201 {
			t.Fatal(i, status, string(raw))
		}
	}
}

// NEDEN: reddedilen (geçersiz) duyurular kotayı harcarsa hatalı bir istemci ajanın
// geçerli duyurularını da engellerdi; kota yalnız biçimi doğru istekleri saymalı.
func TestInvalidAnnouncementDoesNotConsumeQuota(t *testing.T) {
	e := quotaEnv(t)
	for i := 0; i < e.cfg.AnnouncePerMin; i++ {
		body := []byte(fmt.Sprintf(`{"type":"soru","body":"gecersiz %d"}`, i))
		if status, raw := e.raw("POST", announcePath, e.tokens["ali"], body); status != 400 {
			t.Fatal(i, status, string(raw))
		}
	}
	if status, raw := e.raw("POST", announcePath, e.tokens["ali"], announceBody(0)); status != 201 {
		t.Fatal(status, string(raw))
	}
}

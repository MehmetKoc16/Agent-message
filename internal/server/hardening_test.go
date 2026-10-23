package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func TestBekleyenIstekYetkiIptaliniGorur(t *testing.T) {
	// NEDEN: istek açıldıktan sonraki iptal veya oda değişimi yeni mesaj sızdıramaz.
	for _, kind := range []string{"messages", "log"} {
		for _, revoke := range []bool{false, true} {
			for _, notify := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/revoke=%t/message=%t", kind, revoke, notify), func(t *testing.T) {
					e := newEnv(t, nil)
					who := "veli"
					if kind == "log" {
						who = "mehmet"
					}
					ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
					defer cancel()
					type result struct {
						status int
						body   []byte
						err    error
					}
					done := make(chan result, 1)
					go func() {
						status, body, err := e.rawCtx(ctx, "GET", "/v1/rooms/r1/"+kind+"?wait=10", e.tokens[who], nil)
						done <- result{status, body, err}
					}()
					eventually(t, time.Second, "istek yetki denetiminden sonra beklemeye girdi", func() bool {
						e.srv.hub.mu.Lock()
						defer e.srv.hub.mu.Unlock()
						return e.srv.hub.rooms["r1"] != nil
					})
					want := 403
					var err error
					if revoke {
						err = e.st.RevokeAgent(ctx, who)
						want = 401
					} else {
						err = e.st.RemoveRoom(ctx, who, "r1")
					}
					if err != nil {
						t.Fatal(err)
					}
					if notify {
						if status, _ := e.send("r1", "ali", map[string]any{"body": "iptalden sonraki gizli mesaj"}); status != 201 {
							t.Fatal(status)
						}
					}
					select {
					case got := <-done:
						if got.err != nil || got.status != want || strings.Contains(string(got.body), "gizli mesaj") {
							t.Fatalf("iptal sonrası yanıt: %d %s %v", got.status, got.body, got.err)
						}
					case <-time.After(2 * time.Second):
						t.Fatal("iptal edilen istek açık kaldı")
					}
				})
			}
		}
	}
}

func TestMesajIstekKimligiTekrarGonderimiCogaltmaz(t *testing.T) {
	// NEDEN: yanıt kaybolup aynı istek yinelendiğinde yeni mesaj veya tur oluşmamalı.
	e := newEnv(t, nil)
	body := map[string]any{"type": "soru", "body": "tekrarsız", "request_id": "request-1"}
	status, first := e.send("r1", "ali", body)
	if status != 201 {
		t.Fatal(status)
	}
	for i := 0; i < 3; i++ {
		status, again := e.send("r1", "ali", body)
		if status != 201 || num(again["id"]) != num(first["id"]) || num(again["turn"]) != 1 {
			t.Fatal(status, again)
		}
	}
	body["body"] = "farklı içerik"
	if status, out := e.send("r1", "ali", body); status != 409 || out["error"] != "istek_cakismasi" {
		t.Fatal(status, out)
	}
	if got := e.fetch("r1", "veli", 0, 0); len(got) != 1 {
		t.Fatal("mesaj çoğaldı", got)
	}
	body["request_id"] = "bad id"
	if status, _ := e.send("r1", "ali", body); status != 400 {
		t.Fatal(status)
	}
}

func TestLogGecmisSayfalari(t *testing.T) {
	// NEDEN: ilk açılış son 100 mesajı vermeli; önceki sayfalar sıralı ve çakışmasız olmalı.
	e := newEnv(t, nil)
	for i := 0; i < 205; i++ {
		_, err := e.st.Post(context.Background(), store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "sayfa"})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query       string
		first, last int
		more        bool
	}{
		{"tail=1", 106, 205, true}, {"before=106", 6, 105, true}, {"before=6", 1, 5, false},
	} {
		status, raw := e.raw("GET", "/v1/rooms/r1/log?"+tc.query, e.tokens["mehmet"], nil)
		var out struct {
			Messages []store.Message `json:"messages"`
			More     bool            `json:"has_more"`
		}
		if err := json.Unmarshal(raw, &out); err != nil || status != 200 {
			t.Fatal(status, string(raw), err)
		}
		if len(out.Messages) != tc.last-tc.first+1 || out.More != tc.more {
			t.Fatalf("sayfa: %s", raw)
		}
		for i, m := range out.Messages {
			if m.ID != int64(tc.first+i) {
				t.Fatal("sıra bozuk", m.ID)
			}
		}
	}
	for _, query := range []string{"before=0", "before=-1", "before=abc", "tail=0", "tail=1&after=0", "tail=1&before=4", "tail=1&wait=1"} {
		status, _ := e.raw("GET", "/v1/rooms/r1/log?"+query, e.tokens["mehmet"], nil)
		if status != 400 {
			t.Fatal(query, status)
		}
	}
	if status, _ := e.raw("GET", "/v1/rooms/r2/log?tail=1", e.tokens["mehmet"], nil); status != 403 {
		t.Fatal(status)
	}
}

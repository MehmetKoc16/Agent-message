package server

import (
	"encoding/json"
	"fmt"
	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
	"testing"
)

func TestBroadcastFeedOnlyAcceptsInformationAndKeepsOneThread(t *testing.T) {
	// ali/veli ajan hesabıdır; bu test duyuru kotasını değil T-0 biçimini sınar.
	e := newEnv(t, func(c *config.Server) {
		c.RatePerMin = 1000
		c.RatePerDay = 1000
		c.AnnouncePerMin = 1000
		c.AnnouncePerDay = 1000
	})
	path := "/v1/rooms/r1/announcements"
	for i := 0; i < 23; i++ {
		who := "ali"
		if i%2 == 1 {
			who = "veli"
		}
		body := []byte(fmt.Sprintf(`{"type":"bilgi","body":"change %d","request_id":"a-%d"}`, i, i))
		status, raw := e.raw("POST", path, e.tokens[who], body)
		var m store.Message
		_ = json.Unmarshal(raw, &m)
		if status != 201 || m.Thread != "T-0" || m.MaxTurns != 0 {
			t.Fatal(status, string(raw))
		}
	}
	for _, body := range []string{`{"type":"soru","body":"no"}`, `{"type":"bilgi","to":"veli","body":"no"}`, `{"type":"kapat","body":"no"}`, `{"type":"bilgi","thread":"T-1","body":"no"}`} {
		if status, raw := e.raw("POST", path, e.tokens["ali"], []byte(body)); status != 400 {
			t.Fatal(status, string(raw))
		}
	}
	if status, _ := e.raw("POST", path, e.tokens["dis"], []byte(`{"type":"bilgi","body":"no"}`)); status != 403 {
		t.Fatal(status)
	}
	if status, _ := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], []byte(`{"type":"gorev","thread":"T-0","body":"no"}`)); status != 400 {
		t.Fatal(status)
	}
	if status, _ := e.raw("POST", "/v1/rooms/r1/threads/T-0/close", e.tokens["ali"], nil); status != 400 {
		t.Fatal(status)
	}
	status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], []byte(`{"type":"soru","body":"ordinary topic"}`))
	if status != 201 {
		t.Fatal(status, string(raw))
	}
	var m store.Message
	_ = json.Unmarshal(raw, &m)
	if m.Thread != "T-1" || m.MaxTurns != 20 {
		t.Fatal(m)
	}
}

package server

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestUnreadAPIRequiresHumanAndOnlyMarksExactConversation(t *testing.T) {
	e := newEnv(t, nil)
	status, raw := e.raw("POST", "/v1/rooms/r1/messages", e.tokens["ali"], []byte(`{"type":"bilgi","body":"hello"}`))
	if status != 201 {
		t.Fatal(status, string(raw))
	}
	var m struct {
		ID     int64  `json:"id"`
		Thread string `json:"thread"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if status, _ := e.raw("GET", "/v1/unread", e.tokens["ali"], nil); status != 403 {
		t.Fatal(status)
	}
	if status, _ := e.raw("GET", "/v1/unread", "", nil); status != 401 {
		t.Fatal(status)
	}
	path := "/v1/rooms/r1/threads/" + m.Thread + "/read"
	for _, tc := range []struct {
		who, body string
		status    int
	}{{"ali", fmt.Sprintf(`{"last_id":%d}`, m.ID), 403}, {"mehmet", `{"last_id":99999}`, 400}, {"mehmet", `{"last_id":0}`, 400}, {"mehmet", fmt.Sprintf(`{"last_id":%d}`, m.ID), 200}} {
		if status, raw := e.raw("POST", path, e.tokens[tc.who], []byte(tc.body)); status != tc.status {
			t.Fatal(status, string(raw))
		}
	}
	if status, _ := e.raw("POST", "/v1/rooms/r2/threads/"+m.Thread+"/read", e.tokens["mehmet"], []byte(fmt.Sprintf(`{"last_id":%d}`, m.ID))); status != 403 {
		t.Fatal(status)
	}
	if status, raw := e.raw("GET", "/v1/rooms/r1/threads/"+m.Thread+"/log", e.tokens["mehmet"], nil); status != 200 {
		t.Fatal(status, string(raw))
	}
	if status, _ := e.raw("GET", "/v1/rooms/r1/threads/"+m.Thread+"/log", e.tokens["ali"], nil); status != 403 {
		t.Fatal(status)
	}
	status, raw = e.raw("GET", "/v1/unread", e.tokens["mehmet"], nil)
	var out struct {
		Threads []struct {
			ReadID         int64 `json:"read_id"`
			LastIncomingID int64 `json:"last_incoming_id"`
		} `json:"threads"`
	}
	_ = json.Unmarshal(raw, &out)
	if status != 200 || len(out.Threads) != 1 || out.Threads[0].ReadID != m.ID || out.Threads[0].LastIncomingID != m.ID {
		t.Fatal(status, string(raw))
	}
}

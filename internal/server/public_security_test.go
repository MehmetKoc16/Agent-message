package server

import (
	"context"
	"github.com/MehmetKoc16/Agent-message/internal/store"
	"testing"
	"time"
)

func TestPublicViewersCannotCloseOrExhaustThread(t *testing.T) {
	e := newEnv(t, nil)
	_, m := e.send("r1", "ali", map[string]any{})
	thread := m["thread"].(string)
	path := "/v1/rooms/r1/threads/" + thread
	for _, who := range []string{"veli", "can"} {
		if status, _ := e.call("POST", path+"/close", who, nil); status != 403 {
			t.Fatal("public close", status)
		}
		if status, _ := e.send("r1", who, map[string]any{"thread": thread, "type": "kapat"}); status != 403 {
			t.Fatal("public kapat", status)
		}
	}
	for i := 0; i < 10; i++ {
		who := []string{"veli", "can"}[i%2]
		if status, _ := e.send("r1", who, map[string]any{"thread": thread, "to": "mehmet"}); status != 201 {
			t.Fatalf("public reply %d: %d", i, status)
		}
	}
	if _, err := e.st.Purge(context.Background(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"veli", "can"} {
		if status, _ := e.send("r1", who, map[string]any{"thread": thread}); status != 403 {
			t.Fatal("public budget bypass", status)
		}
	}
	if status, _ := e.send("r1", "ali", map[string]any{"thread": thread}); status != 201 {
		t.Fatal("creator lost reserved turns", status)
	}
	if status, _ := e.call("POST", path+"/close", "mehmet", nil); status != 200 {
		t.Fatal("human cannot close", status)
	}
}

func TestTOTPLockSurvivesServerAndDatabaseRestart(t *testing.T) {
	e, now := yonetimEnv(t)
	for i := 0; i < 4; i++ {
		yonetimWant(t, yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 401, "")
	}
	other := New(e.cfg, e.st, e.srv.log)
	e.srv, other = other, e.srv
	defer e.srv.Close()
	defer other.Close()
	yonetimWant(t, yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 429, "cok_fazla_deneme")
	reopened, err := store.Open(e.cfg.DB, e.cfg.MaxTurns)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopened.Now = func() time.Time { return *now }
	*now = now.Add(time.Minute)
	third := New(e.cfg, reopened, e.srv.log)
	e.srv = third
	defer third.Close()
	yonetimWant(t, yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 429, "cok_fazla_deneme")
	*now = now.Add(14 * time.Minute)
	yonetimLogin(t, e, *now)
}

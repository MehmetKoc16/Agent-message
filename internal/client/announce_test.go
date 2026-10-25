package client

import (
	"context"
	"fmt"
	"github.com/MehmetKoc16/Agent-message/internal/store"
	"strings"
	"testing"
)

func TestAnnouncementsShareFeedAcrossAgentsAndLeaveInboxAlone(t *testing.T) {
	e := newCEnv(t)
	var firstID int64
	for i := 0; i < 25; i++ {
		who := "ali"
		if i%2 == 1 {
			who = "veli"
		}
		code, out, errout := e.run(who, "", "announce", "--room-title", "r1", "--request-id", fmt.Sprintf("change-%d", i), fmt.Sprintf("change %d", i))
		if code != 0 || !strings.Contains(out, "thread=T-0") {
			t.Fatal(code, out, errout)
		}
	}
	msgs, err := e.st.RoomLog(context.Background(), "r1", 0, 100)
	if err != nil || len(msgs) != 25 {
		t.Fatal(len(msgs), err)
	}
	firstID = msgs[0].ID
	if msgs[24].Thread != "T-0" || msgs[24].MaxTurns != 0 {
		t.Fatal(msgs[24])
	}
	code, out, errout := e.run("ali", "", "announce", "--room", "r1", "--request-id", "change-0", "change 0")
	if code != 0 || !strings.Contains(out, fmt.Sprintf("id=%d ", firstID)) {
		t.Fatal(code, out, errout)
	}
	msgs, _ = e.st.RoomLog(context.Background(), "r1", 0, 100)
	if len(msgs) != 25 {
		t.Fatal("duplicate delivery", len(msgs))
	}
	code, out, errout = e.run("ali", "", "announce", "--room", "r1", "--new-topic", "new user workflow")
	if code != 0 || !strings.Contains(out, "thread=T-1") {
		t.Fatal(code, out, errout)
	}
	th, _ := e.st.GetThread(context.Background(), "r1", "T-1")
	if th.TurnLimit != 20 {
		t.Fatal(th)
	}
	cfg, _ := LoadConfig(e.cfgs["ali"])
	n, err := (cursor{path: cfg.statePath}).Read()
	if err != nil || n != 0 {
		t.Fatal("announcement changed inbox cursor", n, err)
	}
}

func TestAnnouncementMissingOrAmbiguousRoomNeverFallsBack(t *testing.T) {
	e := newCEnv(t)
	code, _, _ := e.run("ali", "", "announce", "hello")
	if code == 0 {
		t.Fatal("missing default room accepted")
	}
	if err := e.st.AddRoom(context.Background(), "ali", "r2"); err != nil {
		t.Fatal(err)
	}
	code, _, _ = e.run("ali", "", "announce", "--room", "nonmember", "hello")
	if code == 0 {
		t.Fatal("nonmember accepted")
	}
	ctx := context.Background()
	if err := e.st.AddAgent(ctx, "admin", "insan", store.HashToken("test-admin"), nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetYonetici(ctx, "admin", "test"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := e.st.CreateRoom(ctx, "admin", "Update Messages", []string{"ali"}); err != nil {
			t.Fatal(err)
		}
	}
	code, _, _ = e.run("ali", "", "announce", "hello")
	if code == 0 {
		t.Fatal("ambiguous title accepted")
	}
	msgs, _ := e.st.RoomLog(ctx, "r1", 0, 100)
	if len(msgs) != 0 {
		t.Fatal("fallback sent a message", len(msgs))
	}
}

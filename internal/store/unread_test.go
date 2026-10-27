package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestReadMarkersArePerAccountMonotonicAndPersistent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reads.db")
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	for _, name := range []string{"owner", "other"} {
		if err := s.AddAgent(ctx, name, "insan", HashToken(name), []string{"r1"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddAgent(ctx, "bot", "ajan", HashToken("bot"), []string{"r1", "r2"}); err != nil {
		t.Fatal(err)
	}
	own, err := s.Post(ctx, PostInput{Room: "r1", From: "owner", Type: "bilgi", Body: "own"})
	if err != nil {
		t.Fatal(err)
	}
	summaries, _ := s.ThreadSummaries(ctx, "owner")
	if len(summaries) != 1 || summaries[0].LastIncomingID != 0 {
		t.Fatal(summaries)
	}
	first, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Thread: own.Thread, Type: "bilgi", Body: "incoming"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Thread: own.Thread, Type: "bilgi", Body: "incoming2"})
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.Post(ctx, PostInput{Room: "r2", From: "bot", Type: "bilgi", Body: "not visible"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkThreadRead(ctx, "owner", "r1", own.Thread, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkThreadRead(ctx, "owner", "r1", own.Thread, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkThreadRead(ctx, "owner", "r1", own.Thread, private.ID); !errors.Is(err, ErrBadRead) {
		t.Fatal(err)
	}
	if err := s.MarkThreadRead(ctx, "owner", "r2", private.Thread, private.ID); !errors.Is(err, ErrRoomForbidden) {
		t.Fatal(err)
	}
	if err := s.MarkThreadRead(ctx, "bot", "r1", own.Thread, first.ID); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	summaries, _ = s.ThreadSummaries(ctx, "other")
	if len(summaries) != 1 || summaries[0].ReadID != 0 || summaries[0].LastIncomingID != second.ID {
		t.Fatal(summaries)
	}
	s.Close()
	s, err = Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	summaries, err = s.ThreadSummaries(ctx, "owner")
	if err != nil || len(summaries) != 1 || summaries[0].ReadID != second.ID {
		t.Fatal(summaries, err)
	}
	third, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Thread: own.Thread, Type: "bilgi", Body: "new again"})
	if err != nil {
		t.Fatal(err)
	}
	summaries, _ = s.ThreadSummaries(ctx, "owner")
	if summaries[0].LastIncomingID != third.ID || summaries[0].ReadID != second.ID {
		t.Fatal(summaries)
	}
	if err := s.RemoveRoom(ctx, "owner", "r1"); err != nil {
		t.Fatal(err)
	}
	summaries, _ = s.ThreadSummaries(ctx, "owner")
	if len(summaries) != 0 {
		t.Fatal("removed room leaked", summaries)
	}
}

func TestUnreadSummaryAndThreadPagesIncludeOlderConversations(t *testing.T) {
	s := openTemp(t)
	s.maxTurns = 200
	ctx := context.Background()
	if err := s.AddAgent(ctx, "reader", "insan", HashToken("reader"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAgent(ctx, "bot", "ajan", HashToken("bot"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	first, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Type: "bilgi", Body: "older topic"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		if _, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Type: "bilgi", Body: "newer topic"}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ThreadSummaries(ctx, "reader")
	if err != nil || len(all) != 106 || all[len(all)-1].Thread != first.Thread {
		t.Fatal(len(all), err)
	}
	for i := 0; i < 104; i++ {
		if _, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Thread: first.Thread, Type: "bilgi", Body: "same old topic"}); err != nil {
			t.Fatal(err)
		}
	}
	tail, err := s.ThreadLogPage(ctx, "r1", first.Thread, 0, 100)
	if err != nil || len(tail) != 100 {
		t.Fatal(len(tail), err)
	}
	older, err := s.ThreadLogPage(ctx, "r1", first.Thread, tail[0].ID, 100)
	if err != nil || len(older) != 5 || older[0].ID != first.ID {
		t.Fatal(len(older), err)
	}
	for _, m := range append(older, tail...) {
		if m.Thread != first.Thread {
			t.Fatal("other topic leaked", m)
		}
	}
}

// NEDEN: arayüz yeşil noktanın yanında bu sayıyı gösterir; kendi mesajlar ve
// okuma imlecine kadar olanlar sayılmamalı, sayı hesap başına ayrı olmalı.
func TestThreadSummaryUnreadCount(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "count.db"), 20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, a := range []struct{ name, kind string }{{"owner", "insan"}, {"other", "insan"}, {"bot", "ajan"}} {
		if err := s.AddAgent(ctx, a.name, a.kind, HashToken(a.name), []string{"r1"}); err != nil {
			t.Fatal(err)
		}
	}
	unread := func(who string) int {
		t.Helper()
		summaries, err := s.ThreadSummaries(ctx, who)
		if err != nil || len(summaries) != 1 {
			t.Fatal(summaries, err)
		}
		return summaries[0].Unread
	}
	first, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Type: "soru", Body: "1"})
	if err != nil {
		t.Fatal(err)
	}
	var last Message
	for _, body := range []string{"2", "3"} {
		if last, err = s.Post(ctx, PostInput{Room: "r1", From: "bot", Thread: first.Thread, Type: "bilgi", Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Post(ctx, PostInput{Room: "r1", From: "owner", Thread: first.Thread, Type: "bilgi", Body: "kendi"}); err != nil {
		t.Fatal(err)
	}
	if got := unread("owner"); got != 3 {
		t.Fatalf("okunmamış %d, beklenen 3 (kendi mesaj sayılmaz)", got)
	}
	if err := s.MarkThreadRead(ctx, "owner", "r1", first.Thread, first.ID); err != nil {
		t.Fatal(err)
	}
	if got := unread("owner"); got != 2 {
		t.Fatalf("okunmamış %d, beklenen 2", got)
	}
	if err := s.MarkThreadRead(ctx, "owner", "r1", first.Thread, last.ID); err != nil {
		t.Fatal(err)
	}
	if got := unread("owner"); got != 0 {
		t.Fatalf("okunmamış %d, beklenen 0", got)
	}
	if got := unread("other"); got != 4 {
		t.Fatalf("diğer hesabın okunmamışı %d, beklenen 4", got)
	}
}

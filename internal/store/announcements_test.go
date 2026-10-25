package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentAnnouncementsHaveOneFeedAndUniqueTurns(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	for _, who := range []string{"a", "b"} {
		if err := s.AddAgent(ctx, who, "ajan", HashToken(who), []string{"r1"}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			who := "a"
			if n%2 == 1 {
				who = "b"
			}
			m, err := s.Post(ctx, PostInput{Room: "r1", From: who, Type: "bilgi", Body: fmt.Sprint(n), Announcement: true, RequestID: fmt.Sprint(n)})
			if err != nil || m.Thread != "T-0" {
				t.Errorf("%+v %v", m, err)
			}
		}(i)
	}
	wg.Wait()
	messages, err := s.RoomLog(ctx, "r1", 0, 100)
	if err != nil || len(messages) != 30 {
		t.Fatal(len(messages), err)
	}
	for i, m := range messages {
		if m.Turn != i+1 || m.MaxTurns != 0 {
			t.Fatal(m)
		}
	}
	normal, err := s.Post(ctx, PostInput{Room: "r1", From: "a", Type: "soru", Body: "normal"})
	if err != nil || normal.Thread != "T-1" || normal.MaxTurns != 20 {
		t.Fatal(normal, err)
	}
}

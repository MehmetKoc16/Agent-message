package store

import (
	"context"
	"testing"
)

func TestOdaBasligindakiSQLYalnizMetindir(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "admin", "insan", HashToken("test-admin"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetYonetici(ctx, "admin", "test-only"); err != nil {
		t.Fatal(err)
	}
	title := `x'); DROP TABLE agents; -- <img src=x onerror=alert(1)>`
	room, err := s.CreateRoom(ctx, "admin", title, nil)
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := s.RoomsForAgent(ctx, "admin")
	if err != nil || len(rooms) != 1 || rooms[0] != room || room.Title != title {
		t.Fatalf("başlık SQL olarak işlenmemeli: %v %v", rooms, err)
	}
	if _, err := s.Authenticate(ctx, "test-admin"); err != nil {
		t.Fatalf("ajan tablosu korunmalı: %v", err)
	}
}

func TestPostTurBilgisiniVeritabanindanDogrular(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	for _, name := range []string{"ali", "veli", "diger"} {
		if err := s.AddAgent(ctx, name, "ajan", HashToken(name), []string{"r1"}); err != nil {
			t.Fatal(err)
		}
	}
	m, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", To: "veli", Type: "bilgi", Body: "özel"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Post(ctx, PostInput{Room: "r1", From: "diger", FromKind: "insan", Thread: m.Thread, Type: "bilgi", Body: "yanlış tür"})
	if err != ErrNotParty {
		t.Fatalf("eski/yanlış tür bilgisi yetki vermemeli: %v", err)
	}
}

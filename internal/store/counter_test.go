package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
)

func TestGocV2SayaciVeMesajlariKorur(t *testing.T) {
	// NEDEN: mevcut v2 verisi göçtüğünde T-1 yeniden kullanılmamalı ve mesaj korunmalı.
	path := filepath.Join(t.TempDir(), "v2.db")
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = s.AddAgent(ctx, "ali", "ajan", HashToken("test"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	first, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "eski veri"})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DROP INDEX messages_request`, `ALTER TABLE messages DROP COLUMN request_id`, `DROP TABLE room_counters`, `DROP TABLE rooms`, `PRAGMA user_version=2`} {
		if _, err = s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	next, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "yeni veri"})
	if err != nil || first.Thread != "T-1" || next.Thread != "T-2" {
		t.Fatal(first, next, err)
	}
	msgs, err := s.RoomLog(ctx, "r1", 0, 100)
	if err != nil || len(msgs) != 2 || msgs[0].Body != "eski veri" {
		t.Fatal(msgs, err)
	}
}

func TestEszamanliTekrarTekMesajOlur(t *testing.T) {
	// NEDEN: aynı request_id ile paralel teslimler sayaç ve tur tüketmemeli.
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "ali", "ajan", HashToken("test"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "soru", Body: "tek mesaj", RequestID: "aynı-istek"})
			if err != nil || m.ID != 1 || m.Thread != "T-1" {
				t.Errorf("tekrar=%+v %v", m, err)
			}
		}()
	}
	wg.Wait()
	m, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "sonraki"})
	if err != nil || m.ID != 2 || m.Thread != "T-2" {
		t.Fatal(m, err)
	}
}

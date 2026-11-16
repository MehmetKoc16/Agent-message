package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// NEDEN: T-0 duyuru akışıdır (turn_limit=0 işareti); reopen sınırı artırırsa akış
// 20 duyurudan sonra kilitlenir ya da normal konuşmaya döner.
func TestReopenDuyuruAkisiniBozmaz(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "ali", "ajan", HashToken("test-ali"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	duyur := func() error {
		_, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "d", Announcement: true})
		return err
	}
	for i := 0; i < 25; i++ {
		if err := duyur(); err != nil {
			t.Fatal(i, err)
		}
	}
	if err := s.ReopenThread(ctx, "r1", "T-0", 20); !errors.Is(err, ErrAnnouncementThread) {
		t.Fatalf("T-0 reopen reddedilmeli: %v", err)
	}
	th, err := s.GetThread(ctx, "r1", "T-0")
	if err != nil || th.TurnLimit != 0 || th.Status != "acik" {
		t.Fatalf("T-0 değişti: %+v %v", th, err)
	}
	if err := duyur(); err != nil {
		t.Fatalf("reopen denemesinden sonra duyuru: %v", err)
	}
	if _, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "soru", Body: "x", Thread: "T-0"}); !errors.Is(err, ErrBadAnnouncement) {
		t.Fatalf("T-0 normal konuşmaya dönmemeli: %v", err)
	}
	if err := s.ReopenThread(ctx, "r1", "T-9", 20); !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("olmayan konuşma: %v", err)
	}
	m, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReopenThread(ctx, "r1", m.Thread, 20); err != nil {
		t.Fatal(err)
	}
	if th, _ := s.GetThread(ctx, "r1", m.Thread); th.TurnLimit != 40 {
		t.Fatalf("normal konuşma sınırı artmalı: %+v", th)
	}
}

// NEDEN: saklama yalnız mesajı silerse kapalı konuşmaların threads/thread_parties/
// thread_reads satırları sonsuza dek birikir. T-0, açık ve mesajı kalan konuşmalar
// korunur; silinen T-n numarası yeniden verilmez.
func TestPurgeBitmisKonusmaSatirlariniSiler(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	for _, name := range []string{"ali", "veli"} {
		if err := s.AddAgent(ctx, name, "ajan", HashToken("test-"+name), []string{"r1"}); err != nil {
			t.Fatal(err)
		}
	}
	eski := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	yeni := eski.Add(30 * 24 * time.Hour)
	s.Now = func() time.Time { return eski }
	post := func(in PostInput) Message {
		t.Helper()
		in.Room, in.From, in.Body = "r1", "ali", "x"
		if in.Type == "" {
			in.Type = "bilgi"
		}
		m, err := s.Post(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	post(PostInput{Announcement: true})   // T-0
	kapali := post(PostInput{To: "veli"}) // T-1: eski, kapalı
	acik := post(PostInput{To: "veli"})   // T-2: eski, açık
	canli := post(PostInput{})            // T-3: eski, kapalı ama yeni mesajı var
	post(PostInput{Thread: kapali.Thread, Type: "kapat"})
	s.Now = func() time.Time { return yeni }
	post(PostInput{Thread: canli.Thread, Type: "kapat"})
	for _, th := range []string{"T-0", kapali.Thread, acik.Thread, canli.Thread} {
		if _, err := s.db.Exec(`INSERT INTO thread_reads(agent,room,thread,last_id) VALUES ('veli','r1',?,1)`, th); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.Purge(ctx, eski.Add(24*time.Hour))
	if err != nil || n != 5 {
		t.Fatalf("silinen mesaj=%d hata=%v", n, err)
	}
	for _, tc := range []struct {
		thread string
		kalir  bool
	}{{"T-0", true}, {kapali.Thread, false}, {acik.Thread, true}, {canli.Thread, true}} {
		for _, q := range []string{
			`SELECT COUNT(*) FROM threads WHERE room='r1' AND id=?`,
			`SELECT COUNT(*) FROM thread_parties WHERE room='r1' AND thread=?`,
			`SELECT COUNT(*) FROM thread_reads WHERE room='r1' AND thread=?`,
		} {
			var c int
			if err := s.db.QueryRow(q, tc.thread).Scan(&c); err != nil || (c > 0) != tc.kalir {
				t.Fatalf("%s kalır=%v adet=%d hata=%v: %s", tc.thread, tc.kalir, c, err, q)
			}
		}
	}
	// T-0 satırı silinseydi yeni duyuru onu sıfırdan kurar, eski durum kaybolurdu.
	if th, err := s.GetThread(ctx, "r1", "T-0"); err != nil || th.TurnLimit != 0 || th.CreatedAt != FormatTime(eski) {
		t.Fatalf("T-0: %+v %v", th, err)
	}
	if _, err := s.Post(ctx, PostInput{Room: "r1", From: "veli", Type: "bilgi", Body: "x", Thread: acik.Thread}); err != nil {
		t.Fatalf("açık konuşmada taraf yazabilmeli: %v", err)
	}
	if m := post(PostInput{}); m.Thread != "T-4" {
		t.Fatalf("silinen numara yeniden kullanıldı: %s", m.Thread)
	}
	// Her şeyi kapsayan kesimde bile T-0 kalır.
	if _, err := s.Purge(ctx, yeni.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if th, err := s.GetThread(ctx, "r1", "T-0"); err != nil || th.TurnLimit != 0 {
		t.Fatalf("T-0 silindi: %+v %v", th, err)
	}
	if _, err := s.GetThread(ctx, "r1", canli.Thread); !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("mesajı kalmayan kapalı konuşma silinmeli: %v", err)
	}
	if m := post(PostInput{}); m.Thread != "T-5" {
		t.Fatalf("numara geri sardı: %s", m.Thread)
	}
}

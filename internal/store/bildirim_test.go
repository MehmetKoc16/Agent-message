package store

import (
	"context"
	"errors"
	"testing"
)

// NEDEN: "bildirim" dinleyen ajanı uyandırır; sunucu ucu ne olursa olsun store
// onu yalnız T-0'da kabul etmeli. Yeni konu ya da T-n'de ErrBadAnnouncement
// dönmeli, mesaj yazılmamalı, konu numarası ve tur harcanmamalı.
func TestBildirimOnlyAllowedInT0(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "a", "ajan", HashToken("a"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAgent(ctx, "h", "insan", HashToken("h"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"a", "h"} {
		if _, err := s.Post(ctx, PostInput{Room: "r1", From: who, Type: "bildirim", Body: "yeni konu"}); !errors.Is(err, ErrBadAnnouncement) {
			t.Fatal("yeni konu:", who, err)
		}
	}
	topic, err := s.Post(ctx, PostInput{Room: "r1", From: "a", Type: "soru", Body: "konu"})
	if err != nil || topic.Thread != "T-1" {
		t.Fatal("konu numarası harcandı", topic, err)
	}
	for _, who := range []string{"a", "h"} {
		if _, err := s.Post(ctx, PostInput{Room: "r1", From: who, Thread: topic.Thread, Type: "bildirim", Body: "T-n"}); !errors.Is(err, ErrBadAnnouncement) {
			t.Fatal("T-n:", who, err)
		}
	}
	th, err := s.GetThread(ctx, "r1", topic.Thread)
	if err != nil || th.Turns != 1 {
		t.Fatal("tur harcandı", th, err)
	}

	ann, err := s.Post(ctx, PostInput{Room: "r1", From: "a", Type: "bildirim", Body: "duyuru", Announcement: true})
	if err != nil || ann.Thread != "T-0" || ann.MaxTurns != 0 {
		t.Fatal("duyuru:", ann, err)
	}
	viaThread, err := s.Post(ctx, PostInput{Room: "r1", From: "h", Thread: "T-0", Type: "bildirim", Body: "T-0"})
	if err != nil || viaThread.Thread != "T-0" {
		t.Fatal("thread T-0:", viaThread, err)
	}
	reply := ann.ID
	for i, in := range []PostInput{
		{Room: "r1", From: "a", Type: "bildirim", Body: "x", Announcement: true, To: "h"},
		{Room: "r1", From: "a", Type: "bildirim", Body: "x", Announcement: true, ReplyTo: &reply},
		{Room: "r1", From: "h", Thread: "T-0", Type: "bildirim", Body: "x", ReplyTo: &reply},
		{Room: "r1", From: "h", Thread: "T-0", Type: "bildirim", Body: "x", To: "a"},
	} {
		if _, err := s.Post(ctx, in); !errors.Is(err, ErrBadAnnouncement) {
			t.Fatal("yönlendirilmiş bildirim:", i, err)
		}
	}
	msgs, err := s.RoomLog(ctx, "r1", 0, 100)
	if err != nil || len(msgs) != 3 {
		t.Fatal("beklenmeyen mesaj sayısı", len(msgs), err)
	}
}

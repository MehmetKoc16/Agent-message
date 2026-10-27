package store

import (
	"context"
	"testing"
)

// NEDEN (#31): ajan, işi bir insan hesabının atadığını çerçeveden doğrulamalı.
// Tür sunucunun agents kaydından gelmeli; mesajı döndüren HER yol (Post,
// request_id tekrarı, Fetch, ThreadMessages ve log sayfaları) aynı türü taşımalı.
// Bir yol msgSelect/scanMessages dışında kalırsa tür boş döner ve ajan yanılır.
func TestMesajlarGonderenTurunuTasir(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	for _, a := range []struct{ name, kind string }{{"ali", "ajan"}, {"mehmet", "insan"}, {"veli", "ajan"}} {
		if err := s.AddAgent(ctx, a.name, a.kind, HashToken("test-"+a.name), []string{"r1"}); err != nil {
			t.Fatal(err)
		}
	}
	ajanIn := PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "ajan mesajı", RequestID: "rq-ajan"}
	ajanMsg, err := s.Post(ctx, ajanIn)
	if err != nil {
		t.Fatal(err)
	}
	insanIn := PostInput{Room: "r1", From: "mehmet", Type: "gorev", Thread: ajanMsg.Thread, Body: "insan görevi", RequestID: "rq-insan"}
	insanMsg, err := s.Post(ctx, insanIn)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{ajanMsg.ID: "ajan", insanMsg.ID: "insan"}

	// Post dönüşü.
	if ajanMsg.FromKind != "ajan" || insanMsg.FromKind != "insan" {
		t.Fatalf("Post dönüşü: ajan=%q insan=%q", ajanMsg.FromKind, insanMsg.FromKind)
	}
	// request_id tekrarı msgSelect yolundan döner; tür JOIN'den gelmeli.
	for _, in := range []PostInput{ajanIn, insanIn} {
		again, err := s.Post(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if again.ID != map[string]int64{"ali": ajanMsg.ID, "mehmet": insanMsg.ID}[in.From] || again.FromKind != want[again.ID] {
			t.Fatalf("request_id tekrarı: %+v", again)
		}
	}

	check := func(what string, msgs []Message, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if len(msgs) != len(want) {
			t.Fatalf("%s: %d mesaj, beklenen %d: %+v", what, len(msgs), len(want), msgs)
		}
		for _, m := range msgs {
			if m.FromKind != want[m.ID] {
				t.Fatalf("%s: id=%d from=%s from_kind=%q, beklenen %q", what, m.ID, m.From, m.FromKind, want[m.ID])
			}
		}
	}
	msgs, err := s.Fetch(ctx, "r1", "veli", 0, 100)
	check("Fetch", msgs, err)
	msgs, err = s.ThreadMessages(ctx, "r1", ajanMsg.Thread, "veli")
	check("ThreadMessages", msgs, err)
	msgs, err = s.RoomLog(ctx, "r1", 0, 100)
	check("RoomLog", msgs, err)
	msgs, err = s.RoomLogPage(ctx, "r1", 0, 100)
	check("RoomLogPage", msgs, err)
	msgs, err = s.ThreadLogPage(ctx, "r1", ajanMsg.Thread, 0, 100)
	check("ThreadLogPage", msgs, err)
}

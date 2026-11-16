package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// NEDEN: bayt bütçesi gövdenin JSON'daki gerçek boyutunu saymazsa kaçış gerektiren
// gövdelerle (kontrol karakteri, tırnak, U+2028) sınır yine aşılır.
func Test_jsonCost_gercek_kodlamadan_kucuk_degil(t *testing.T) {
	for _, s := range []string{"", "düz metin", strings.Repeat("\x01", 50), "\"\\\n\r\t\b\f", "a\u2028b\u2029c", "<>&", "\x7f€😀"} {
		var b strings.Builder
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(s); err != nil {
			t.Fatal(err)
		}
		if real := b.Len() - 1; jsonCost(s) < real { // -1: Encode'un satır sonu
			t.Errorf("%q: hesap %d < gerçek %d", s, jsonCost(s), real)
		}
	}
}

// NEDEN: özet sınırı en yeni konuşmaları seçseydi, eski ama okunmamış konuşmanın
// işareti kaybolurdu. Okunmamışlar önce gelir; sayıları kırpmadan etkilenmez.
func Test_ozet_siniri_okunmamisi_dusurmez(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "okur", "insan", HashToken("okur"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAgent(ctx, "bot", "ajan", HashToken("bot"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	eski, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Type: "soru", Body: "eski ama okunmamış"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Type: "bilgi", Thread: eski.Thread, Body: "ikinci"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxSummaries+5; i++ {
		m, err := s.Post(ctx, PostInput{Room: "r1", From: "bot", Type: "bilgi", Body: "okunmuş"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkThreadRead(ctx, "okur", "r1", m.Thread, m.ID); err != nil {
			t.Fatal(err)
		}
	}
	out, truncated, err := s.ThreadSummaries(ctx, "okur")
	if err != nil || len(out) != MaxSummaries || !truncated {
		t.Fatal(len(out), truncated, err)
	}
	// Çıktı en yeni mesaja göre azalan sırada kalır; okunmamış eski konuşma sondadır.
	last := out[len(out)-1]
	if last.Thread != eski.Thread || last.Unread != 2 || last.Total != 2 {
		t.Fatalf("okunmamış konuşma düştü: %+v", last)
	}
	for i := 1; i < len(out); i++ {
		if out[i-1].LastID <= out[i].LastID {
			t.Fatalf("sıra bozuk: %d, %d", out[i-1].LastID, out[i].LastID)
		}
	}
}

// NEDEN: bayt kırpma karakter ortasından keserse JSON'da U+FFFD'ye dönüşür.
func Test_cutUTF8_karakter_bolmez(t *testing.T) {
	for _, s := range []string{"abc", strings.Repeat("ğ", 200), "a" + strings.Repeat("€", 100), strings.Repeat("😀", 70)} {
		got := cutUTF8(s, maxTitleBytes)
		if len(got) > maxTitleBytes || !utf8.ValidString(got) || !strings.HasPrefix(s, got) || len(s)-len(got) > 0 && len(got) < maxTitleBytes-3 {
			t.Errorf("%d bayt -> %d bayt", len(s), len(got))
		}
	}
}

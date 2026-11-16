package store

import (
	"context"
	"testing"
	"time"
)

// NEDEN: 5 yanlış kodla kilitlenen hesaba SSH'den yeni sır atanınca eski kilit
// 15 dakika daha sürmemeli; sayaç eski sırra aittir.
func TestYeniSirTOTPKilidiniSifirlar(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if err := s.AddAgent(ctx, "y", "insan", HashToken("test-y"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	bad := func(string, string, time.Time) (int64, bool) { return 0, false }
	good := func(string, string, time.Time) (int64, bool) { return 99, true }
	lock := func() {
		t.Helper()
		if err := s.ChangeAccount(ctx, "ssh", "yonetici-yap", "y", "", secret, false); err != nil {
			t.Fatal(err)
		}
		locked := false
		for i := 0; i < 5; i++ {
			var err error
			if _, _, locked, err = s.AttemptTOTP(ctx, "y", "000000", bad); err != nil {
				t.Fatal(err)
			}
		}
		if !locked {
			t.Fatal("beş yanlış deneme kilitlemeli")
		}
	}
	for _, islemler := range [][]string{{"yonetici-yap"}, {"yonetici-al", "yonetici-yap"}} {
		lock()
		for _, islem := range islemler {
			if err := s.ChangeAccount(ctx, "ssh", islem, "y", "", secret, false); err != nil {
				t.Fatal(err)
			}
		}
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM yonetim_failures WHERE agent='y'`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%v sonrası sayaç kaldı: %d %v", islemler, n, err)
		}
		if _, ok, locked, err := s.AttemptTOTP(ctx, "y", "123456", good); err != nil || !ok || locked {
			t.Fatalf("%v sonrası doğru kod: ok=%v kilit=%v hata=%v", islemler, ok, locked, err)
		}
	}
}

// NEDEN: listeleme kayıtları asıl işlemleri 1000 satırlık pencereden itememeli; yine de
// araya giren gerçek işlemden sonraki okuma ayrı satır olmalı, hesaplar birbirini
// etkilememeli ve eski kayıtlara before imleciyle ulaşılabilmeli.
func TestOkumaKaydiTekSatirdaToplanir(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	if err := s.AddAgent(ctx, "kurban", "ajan", HashToken("test-kurban"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeAccount(ctx, "saldirgan", "revoke", "kurban", "", "", false); err != nil {
		t.Fatal(err)
	}
	oku := func(yapan, islem string) {
		t.Helper()
		if err := s.AddOkumaKayit(ctx, yapan, islem); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 1000; i++ {
		now = now.Add(time.Second)
		oku("saldirgan", "kayit_listelendi")
		oku("baska", "kayit_listelendi")
		oku("saldirgan", "hesaplar_listelendi")
	}
	rows, err := s.ListKayit(ctx, 1000)
	if err != nil || len(rows) != 4 || rows[3].Islem != "revoke" {
		t.Fatalf("revoke görünmeli, okumalar birer satır olmalı: %d satır %v", len(rows), err)
	}
	for _, k := range rows[:3] {
		if k.Zaman != FormatTime(now) {
			t.Fatalf("son okuma zamanı güncellenmeli: %+v", k)
		}
	}
	if err := s.AddKayit(ctx, "saldirgan", "oturum_acildi", "saldirgan", ""); err != nil {
		t.Fatal(err)
	}
	oku("saldirgan", "kayit_listelendi")
	oku("saldirgan", "kayit_listelendi")
	rows, err = s.ListKayit(ctx, 1000)
	if err != nil || len(rows) != 6 || rows[0].Islem != "kayit_listelendi" || rows[1].Islem != "oturum_acildi" {
		t.Fatalf("işlemden sonraki okuma bir kez yeniden yazılmalı: %+v %v", rows, err)
	}
	page, err := s.ListKayitPage(ctx, rows[1].ID, 2)
	if err != nil || len(page) != 2 || page[0].ID != rows[2].ID || page[1].ID != rows[3].ID {
		t.Fatalf("before sayfası: %+v %v", page, err)
	}
	if page, err := s.ListKayitPage(ctx, rows[5].ID, 100); err != nil || len(page) != 0 {
		t.Fatalf("en eskinin öncesi boş olmalı: %+v %v", page, err)
	}
}

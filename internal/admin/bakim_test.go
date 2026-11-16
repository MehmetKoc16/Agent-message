package admin

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func bakimEnv(t *testing.T) (config.Server, *store.Store) {
	t.Helper()
	cfg := config.Default()
	cfg.DB = filepath.Join(t.TempDir(), "relay.db")
	if code, _, errOut := runAdmin(t, cfg, "add-agent", "ali", "--rooms", "r1"); code != 0 {
		t.Fatal(errOut)
	}
	st, err := store.Open(cfg.DB, cfg.MaxTurns)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return cfg, st
}

// NEDEN: gün sayısı time.Duration'a çevrilirken taşarsa kesim geleceğe düşer ve purge
// bütün mesajları siler. Sınır dışı değer hata olmalı, sınırdaki değer taşmamalı.
func TestAdminPurgeBuyukSaklamaSuresiSilmez(t *testing.T) {
	cfg, st := bakimEnv(t)
	ctx := context.Background()
	if _, err := st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "taze"}); err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{200000, 106752, 3651, 0, -1} {
		cfg.RetentionDays = days
		if code, out, _ := runAdmin(t, cfg, "purge"); code == 0 {
			t.Fatalf("retention_days=%d kabul edildi: %q", days, out)
		}
	}
	for _, days := range []int{3650, 1} {
		cfg.RetentionDays = days
		if code, out, errOut := runAdmin(t, cfg, "purge"); code != 0 || !strings.Contains(out, "silinen mesaj: 0") {
			t.Fatalf("retention_days=%d: %q %q", days, out, errOut)
		}
	}
	if msgs, err := st.RoomLog(ctx, "r1", 0, 10); err != nil || len(msgs) != 1 {
		t.Fatalf("taze mesaj silindi: %v %v", msgs, err)
	}
	st.Now = func() time.Time { return time.Now().AddDate(0, 0, -2) }
	if _, err := st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "eski"}); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := runAdmin(t, cfg, "purge"); code != 0 || !strings.Contains(out, "silinen mesaj: 1") {
		t.Fatalf("bir günlük sürede eski mesaj silinmeli: %q", out)
	}
}

// NEDEN: reopen duyuru akışını (T-0) bozmamalı, biçimsiz oda/konuşma almamalı; reopen ve
// purge diğer SSH işlemleri gibi denetim kaydında görünmeli.
func TestAdminReopenVePurgeDenetlenir(t *testing.T) {
	cfg, st := bakimEnv(t)
	ctx := context.Background()
	if _, err := st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "d", Announcement: true}); err != nil {
		t.Fatal(err)
	}
	m, err := st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"r1", "T-0"}, {"Kötü Oda", m.Thread}, {"r1", "T-1x"}, {"r1", "t-1"}, {"r1", "T-00"}} {
		if code, out, _ := runAdmin(t, cfg, append([]string{"reopen"}, args...)...); code == 0 {
			t.Fatalf("reopen %v kabul edildi: %q", args, out)
		}
	}
	if th, err := st.GetThread(ctx, "r1", "T-0"); err != nil || th.TurnLimit != 0 {
		t.Fatalf("T-0 bozuldu: %+v %v", th, err)
	}
	if code, _, errOut := runAdmin(t, cfg, "reopen", "r1", m.Thread); code != 0 {
		t.Fatal(errOut)
	}
	if code, _, errOut := runAdmin(t, cfg, "purge"); code != 0 {
		t.Fatal(errOut)
	}
	rows, err := st.ListKayit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, k := range rows {
		if k.Yapan != "ssh" {
			t.Fatalf("yapan ssh olmalı: %+v", k)
		}
		if k.Islem == "reopen" && (k.Hedef != "r1" || !strings.HasPrefix(k.Ayrinti, m.Thread+" ")) {
			t.Fatalf("reopen kaydı: %+v", k)
		}
		seen[k.Islem]++
	}
	if seen["reopen"] != 1 || seen["purge"] != 1 {
		t.Fatalf("başarısız denemeler yazılmamalı, başarılılar yazılmalı: %v", seen)
	}
}

// NEDEN: art arda `kayit` çağrıları kendi satırlarıyla asıl işlemleri pencereden
// itmemeli; eski kayıtlara --before ile ulaşılabilmeli.
func TestAdminKayitGomulmezVeSayfalanir(t *testing.T) {
	cfg, st := bakimEnv(t)
	ctx := context.Background()
	if code, _, _ := runAdmin(t, cfg, "revoke", "ali"); code != 0 {
		t.Fatal("revoke")
	}
	var out string
	for i := 0; i < 5; i++ {
		var code int
		if code, out, _ = runAdmin(t, cfg, "kayit", "--son", "2"); code != 0 {
			t.Fatal("kayit")
		}
	}
	if !strings.Contains(out, "\trevoke\t") {
		t.Fatalf("revoke penceresinden itildi: %q", out)
	}
	rows, err := st.ListKayit(ctx, 100)
	if err != nil || len(rows) != 3 { // hesap_eklendi, revoke, kayit_listelendi
		t.Fatalf("satır sayısı=%d hata=%v: %+v", len(rows), err, rows)
	}
	code, out, _ := runAdmin(t, cfg, "kayit", "--son", "1", "--before", "3")
	if code != 0 || !strings.HasPrefix(out, "2\t") || strings.Count(out, "\n") != 1 {
		t.Fatalf("--before: %q", out)
	}
	for _, bad := range []string{"0", "-1", "x"} {
		if code, _, _ := runAdmin(t, cfg, "kayit", "--before", bad); code == 0 {
			t.Fatalf("--before %s kabul edildi", bad)
		}
	}
}

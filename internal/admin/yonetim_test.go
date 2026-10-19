package admin

import (
	"context"
	"encoding/base32"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// NEDEN: yetki yalnız SSH ile aktif insana verilir; sır bir kez çıkar, geri alma temizler.
func TestAdminYoneticiYap(t *testing.T) {
	cfg := config.Default()
	cfg.DB = filepath.Join(t.TempDir(), "relay.db")
	ctx := context.Background()
	for _, tc := range []struct{ name, kind string }{{"ali", "ajan"}, {"mehmet", "insan"}, {"iptal", "insan"}} {
		if code, _, _ := runAdmin(t, cfg, "add-agent", tc.name, "--rooms", "r1", "--kind", tc.kind); code != 0 {
			t.Fatal("hesap eklenemedi")
		}
	}
	if code, _, _ := runAdmin(t, cfg, "revoke", "iptal"); code != 0 {
		t.Fatal("iptal başarısız")
	}
	for _, name := range []string{"ali", "iptal", "yok"} {
		if code, out, _ := runAdmin(t, cfg, "yonetici-yap", name); code == 0 || out != "" {
			t.Fatal("geçersiz hedefe yetki")
		}
	}
	code, out, errout := runAdmin(t, cfg, "yonetici-yap", "mehmet")
	lines := strings.Fields(out)
	if code != 0 || len(lines) != 2 {
		t.Fatal("sır ve URI bekleniyordu")
	}
	secret := lines[0]
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(raw) != 20 {
		t.Fatal("160 bit sır gerekli")
	}
	uri, err := url.Parse(lines[1])
	if err != nil || uri.Scheme != "otpauth" || uri.Host != "totp" || uri.Path != "/Relay:mehmet" || uri.Query().Get("secret") != secret || uri.Query().Get("issuer") != "Relay" || uri.Query().Get("digits") != "6" || uri.Query().Get("period") != "30" || uri.Query().Get("algorithm") != "SHA1" {
		t.Fatal("URI geçersiz")
	}
	if strings.Contains(errout, secret) || !strings.Contains(errout, "Google Authenticator → Kurulum anahtarı gir → zamana dayalı") {
		t.Fatal("stderr açıklaması")
	}
	st, err := store.Open(cfg.DB, cfg.MaxTurns)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	yes, err := st.IsYonetici(ctx, "mehmet")
	stored, step, err2 := st.TOTPState(ctx, "mehmet")
	if err != nil || err2 != nil || !yes || stored != secret || step != 0 {
		t.Fatal("yetki kaydedilmedi")
	}
	for _, cmd := range []string{"odaya-ekle", "odaya-ekle", "odadan-cikar", "odadan-cikar"} {
		if code, _, _ := runAdmin(t, cfg, cmd, "ali", "r2"); code != 0 {
			t.Fatal(cmd)
		}
	}
	member, err := st.HasRoom(ctx, "ali", "r2")
	if err != nil || member {
		t.Fatal("üyelik çıkarılmadı")
	}
	if code, _, _ := runAdmin(t, cfg, "yonetici-al", "mehmet"); code != 0 {
		t.Fatal("yetki alınmadı")
	}
	yes, err = st.IsYonetici(ctx, "mehmet")
	stored, step, err2 = st.TOTPState(ctx, "mehmet")
	if err != nil || err2 != nil || yes || stored != "" || step != 0 {
		t.Fatal("sır temizlenmedi")
	}
	code, out, errout = runAdmin(t, cfg, "kayit", "--son", "100")
	if code != 0 || strings.Contains(out+errout, secret) || !strings.Contains(out, "ssh") || !strings.Contains(out, "yonetici-yap") || !strings.Contains(out, "yonetici-al") {
		t.Fatal("denetim kaydı")
	}
}

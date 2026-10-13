package admin

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func runAdmin(t *testing.T, cfg config.Server, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(context.Background(), cfg, args, &out, &errb)
	return code, out.String(), errb.String()
}

// NEDEN: token yalnız bir kez, yalnız stdout'a basılmalı; list/stderr'de görünmemeli;
// iptal sonrası doğrulama başarısız olmalı; reopen sınırı artırmalı.
func TestAdminAjanYasamDongusu(t *testing.T) {
	cfg := config.Default()
	cfg.DB = filepath.Join(t.TempDir(), "relay.db")

	code, out, errOut := runAdmin(t, cfg, "add-agent", "ali", "--rooms", "r1,r2", "--kind", "insan")
	token := strings.TrimSpace(out)
	if code != 0 || len(token) < 40 || strings.Contains(token, " ") || strings.Contains(errOut, token) {
		t.Fatalf("add-agent: %d out=%q err=%q", code, out, errOut)
	}
	if code, _, _ := runAdmin(t, cfg, "add-agent", "ali", "--rooms", "r1"); code == 0 {
		t.Fatal("aynı ad ikinci kez eklenmemeli")
	}
	if code, _, _ := runAdmin(t, cfg, "add-agent", "Kötü Ad", "--rooms", "r1"); code == 0 {
		t.Fatal("geçersiz ad reddedilmeli")
	}
	if code, _, _ := runAdmin(t, cfg, "add-agent", "x", "--rooms", ""); code == 0 {
		t.Fatal("odasız ajan reddedilmeli")
	}

	code, out, errOut = runAdmin(t, cfg, "list")
	if code != 0 || !strings.Contains(out, "ali") || !strings.Contains(out, "r1,r2") || !strings.Contains(out, "insan") ||
		strings.Contains(out+errOut, token) || strings.Contains(out, store.HashToken(token)) {
		t.Fatalf("list: %d %q", code, out)
	}

	st, err := store.Open(cfg.DB, cfg.MaxTurns)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if a, err := st.Authenticate(ctx, token); err != nil || a.Kind != "insan" {
		t.Fatalf("token geçerli olmalı: %v %v", a, err)
	}
	if code, _, _ := runAdmin(t, cfg, "revoke", "ali"); code != 0 {
		t.Fatal("revoke başarısız")
	}
	if _, err := st.Authenticate(ctx, token); err == nil {
		t.Fatal("iptal sonrası token geçmemeli")
	}
	if code, _, _ := runAdmin(t, cfg, "revoke", "yok"); code == 0 {
		t.Fatal("olmayan ajanı iptal etmek hata olmalı")
	}
	if code, out, _ := runAdmin(t, cfg, "list"); code != 0 || !strings.Contains(out, "iptal") {
		t.Fatalf("list iptali göstermeli: %q", out)
	}

	m, err := st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runAdmin(t, cfg, "reopen", "r1", m.Thread); code != 0 {
		t.Fatal("reopen başarısız")
	}
	th, _ := st.GetThread(ctx, "r1", m.Thread)
	if th.Status != "acik" || th.TurnLimit != 40 {
		t.Fatalf("reopen: %+v", th)
	}
	if code, _, _ := runAdmin(t, cfg, "reopen", "r1", "T-99"); code == 0 {
		t.Fatal("olmayan konuşma reopen hatası vermeli")
	}
	if code, out, _ := runAdmin(t, cfg, "purge"); code != 0 || !strings.Contains(out, "0") {
		t.Fatalf("purge: %q", out)
	}
}

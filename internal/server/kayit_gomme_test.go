package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// NEDEN: yönetim oturumunu ele geçiren, hız sınırı olmayan listeleme GET'leriyle
// iptal/silme kaydını görünür pencerenin dışına itememeli; eski kayıtlar da
// before imleciyle okunabilmeli.
func TestYonetimListelemeKaydiGomemez(t *testing.T) {
	e, now := yonetimEnv(t)
	session := yonetimLogin(t, e, *now)
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar/ali/iptal", "mehmet", session, ""), 200, "")
	for i := 0; i < 150; i++ {
		yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "mehmet", session, ""), 200, "")
		yonetimWant(t, yonetimReq(e, "GET", "/kayit?son=1", "mehmet", session, ""), 200, "")
	}
	list := func(query string) []store.Kayit {
		t.Helper()
		w := yonetimReq(e, "GET", "/kayit"+query, "mehmet", session, "")
		yonetimWant(t, w, 200, "")
		var rows []store.Kayit
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	rows := list("?son=100")
	count := map[string]int{}
	for _, k := range rows {
		count[k.Islem]++
	}
	if count["hesap_iptal"] != 1 || count["hesaplar_listelendi"] != 1 || count["kayit_listelendi"] != 1 {
		t.Fatalf("iptal görünmeli, listeleme tek satır olmalı: %v", count)
	}
	// Araya giren gerçek işlemden sonraki listeleme yeniden yazılır (sıra korunur).
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar/veli/odalar", "mehmet", session, `{"oda":"r3"}`), 200, "")
	if rows = list("?son=100"); rows[0].Islem != "kayit_listelendi" || rows[1].Islem != "odaya_eklendi" {
		t.Fatalf("işlemden sonraki listeleme yeni satır olmalı: %+v", rows[:2])
	}
	page := list(fmt.Sprintf("?son=2&before=%d", rows[1].ID))
	if len(page) != 2 || page[0].ID != rows[2].ID || page[1].ID != rows[3].ID {
		t.Fatalf("before sayfası: %+v", page)
	}
	if got := list(fmt.Sprintf("?before=%d", rows[len(rows)-1].ID)); len(got) != 0 {
		t.Fatalf("en eski kaydın öncesi boş olmalı: %+v", got)
	}
	for _, bad := range []string{"0", "-1", "x", ""} {
		w := yonetimReq(e, "GET", "/kayit?before="+bad, "mehmet", session, "")
		if w.Code != 400 || !strings.Contains(w.Body.String(), "gecersiz_before") {
			t.Fatalf("before=%q: %d %s", bad, w.Code, w.Body.String())
		}
	}
}

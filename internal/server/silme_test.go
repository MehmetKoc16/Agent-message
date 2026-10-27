package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func silmeMesaj(t *testing.T, e *testEnv, from, thread, body string) store.Message {
	t.Helper()
	m, err := e.st.Post(context.Background(), store.PostInput{Room: "r1", From: from, Type: "bilgi", Thread: thread, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// NEDEN: silme yalnız TOTP oturumu olan yöneticiye açık olmalı; gövde kayda düşmemeli.
func TestYoneticiMesajSiler(t *testing.T) {
	e, now := yonetimEnv(t)
	ctx := context.Background()
	first := silmeMesaj(t, e, "ali", "", "gizli ilk mesaj")
	silmeMesaj(t, e, "veli", first.Thread, "ikinci")
	other := silmeMesaj(t, e, "ali", "", "başka konuşma")
	path := "/odalar/r1/mesajlar/" + itoa(int(first.ID))
	yonetimWant(t, yonetimReq(e, "DELETE", path, "ali", "", ""), 403, "yalniz_yonetici")
	yonetimWant(t, yonetimReq(e, "DELETE", path, "mehmet", "", ""), 401, "yonetim_oturumu_gerekli")
	session := yonetimLogin(t, e, *now)
	yonetimWant(t, yonetimReq(e, "DELETE", "/odalar/R1/mesajlar/1", "mehmet", session, ""), 400, "gecersiz_oda")
	yonetimWant(t, yonetimReq(e, "DELETE", "/odalar/r1/mesajlar/0", "mehmet", session, ""), 400, "gecersiz_mesaj")
	yonetimWant(t, yonetimReq(e, "DELETE", "/odalar/yok/mesajlar/1", "mehmet", session, ""), 404, "oda_yok")
	yonetimWant(t, yonetimReq(e, "DELETE", "/odalar/r2"+path[len("/odalar/r1"):], "mehmet", session, ""), 404, "mesaj_yok")

	w := yonetimReq(e, "DELETE", path, "mehmet", session, "")
	yonetimWant(t, w, 200, "")
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out["thread"] != first.Thread {
		t.Fatalf("yanıt %s %v", w.Body.String(), err)
	}
	yonetimWant(t, yonetimReq(e, "DELETE", path, "mehmet", session, ""), 404, "mesaj_yok")
	log, err := e.st.RoomLog(ctx, "r1", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 2 || log[0].ID == first.ID {
		t.Fatalf("kalan mesajlar %+v", log)
	}

	tpath := "/odalar/r1/konusmalar/" + first.Thread
	yonetimWant(t, yonetimReq(e, "DELETE", "/odalar/r1/konusmalar/x", "mehmet", session, ""), 400, "gecersiz_thread")
	w = yonetimReq(e, "DELETE", tpath, "mehmet", session, "")
	yonetimWant(t, w, 200, "")
	if !strings.Contains(w.Body.String(), `"silinen":1`) {
		t.Fatalf("yanıt %s", w.Body.String())
	}
	yonetimWant(t, yonetimReq(e, "DELETE", tpath, "mehmet", session, ""), 404, "mesaj_yok")
	log, err = e.st.RoomLog(ctx, "r1", 0, 100)
	if err != nil || len(log) != 1 || log[0].ID != other.ID {
		t.Fatalf("kalan mesajlar %+v %v", log, err)
	}
	// Konuşma satırı kalır: numara yeniden kullanılmaz, tur sayacı sürer.
	next := silmeMesaj(t, e, "veli", first.Thread, "sonra")
	if next.Turn != 3 {
		t.Fatalf("tur %d", next.Turn)
	}

	kayit, err := e.st.ListKayit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, k := range kayit {
		if strings.Contains(k.Ayrinti, "gizli") {
			t.Fatalf("gövde kayda yazıldı: %+v", k)
		}
		if k.Islem == "mesaj_silindi" || k.Islem == "konusma_silindi" {
			got[k.Islem] = k.Hedef + " " + k.Ayrinti
		}
	}
	if got["mesaj_silindi"] != "r1 #"+itoa(int(first.ID))+" · "+first.Thread || got["konusma_silindi"] != "r1 "+first.Thread+" · 1 mesaj" {
		t.Fatalf("kayıt %v", got)
	}
}

// NEDEN: oturum açıldıktan sonra yetkisi alınan yönetici silememeli.
func TestYetkisiAlinanYoneticiSilemez(t *testing.T) {
	e, now := yonetimEnv(t)
	m := silmeMesaj(t, e, "ali", "", "kalmalı")
	session := yonetimLogin(t, e, *now)
	if err := e.st.ClearYonetici(context.Background(), "mehmet"); err != nil {
		t.Fatal(err)
	}
	yonetimWant(t, yonetimReq(e, "DELETE", "/odalar/r1/mesajlar/"+itoa(int(m.ID)), "mehmet", session, ""), 403, "yalniz_yonetici")
	if _, err := e.st.DeleteThread(context.Background(), "mehmet", "r1", m.Thread); err != store.ErrUnauthorized {
		t.Fatalf("hata %v", err)
	}
}

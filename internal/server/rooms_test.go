package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func TestYeniOdaYoneticiVeTOTPGerektirir(t *testing.T) {
	// NEDEN: public API üzerinden normal hesap veya yalnız çalınan token oda açamamalı.
	e, now := yonetimEnv(t)
	body := `{"baslik":"Admin Sohbeti","uyeler":[]}`
	yonetimWant(t, yonetimReq(e, "POST", "/odalar", "ali", "", body), 403, "yalniz_yonetici")
	yonetimWant(t, yonetimReq(e, "POST", "/odalar", "mehmet", "", body), 401, "yonetim_oturumu_gerekli")
	session := yonetimLogin(t, e, *now)
	*now = now.Add(10 * time.Minute)
	yonetimWant(t, yonetimReq(e, "POST", "/odalar", "mehmet", session, body), 401, "yonetim_oturumu_gerekli")
}

func TestYeniOdaBaslikUyeVeIzolasyon(t *testing.T) {
	// NEDEN: başlık kalıcı olmalı, diğer yöneticiler dahil seçilmeyen hesaplar odayı görmemeli.
	e, now := yonetimEnv(t)
	e.ts = httptest.NewServer(e.srv.Handler())
	t.Cleanup(e.ts.Close)
	ctx := context.Background()
	if err := e.st.AddAgent(ctx, "ece", "insan", store.HashToken("test-ece"), nil); err != nil {
		t.Fatal(err)
	}
	e.tokens["ece"] = "test-ece"
	if err := e.st.SetYonetici(ctx, "ece", testTOTPSecret); err != nil {
		t.Fatal(err)
	}
	if err := e.st.AddAgent(ctx, "diger-admin", "insan", store.HashToken("test-other-admin"), nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetYonetici(ctx, "diger-admin", testTOTPSecret); err != nil {
		t.Fatal(err)
	}
	e.tokens["diger-admin"] = "test-other-admin"
	session := yonetimLogin(t, e, *now)
	w := yonetimReq(e, "POST", "/odalar", "mehmet", session, `{"baslik":" Admin Sohbeti <b>Özel</b> ","uyeler":["ece","ali","ali"]}`)
	yonetimWant(t, w, 201, "")
	var room store.Room
	if err := json.Unmarshal(w.Body.Bytes(), &room); err != nil {
		t.Fatal(err)
	}
	if room.Title != "Admin Sohbeti <b>Özel</b>" || !strings.HasPrefix(room.Name, "oda-") {
		t.Fatal(room)
	}
	for _, name := range []string{"mehmet", "ece", "ali"} {
		status, raw := e.raw("GET", "/v1/rooms", e.tokens[name], nil)
		var out struct {
			Rooms []store.Room `json:"rooms"`
		}
		if err := json.Unmarshal(raw, &out); err != nil || status != 200 {
			t.Fatal(status, string(raw), err)
		}
		found := false
		for _, r := range out.Rooms {
			if r.Name == room.Name && r.Title == room.Title {
				found = true
			}
		}
		if !found {
			t.Fatal("seçilen üye odayı görmüyor", name)
		}
	}
	if status, _ := e.raw("GET", "/v1/rooms/"+room.Name+"/messages", e.tokens["veli"], nil); status != 403 {
		t.Fatal(status)
	}
	if status, _ := e.raw("GET", "/v1/rooms/"+room.Name+"/log", e.tokens["mehmet"], nil); status != 200 {
		t.Fatal(status)
	}
	if status, _ := e.send(room.Name, "ali", map[string]any{"body": "yeni odada mesaj"}); status != 201 {
		t.Fatal(status)
	}
	if status, raw := e.raw("GET", "/v1/rooms", e.tokens["veli"], nil); status != 200 || strings.Contains(string(raw), room.Name) {
		t.Fatal(status, string(raw))
	}
	if status, _ := e.raw("GET", "/v1/rooms", "", nil); status != 401 {
		t.Fatal(status)
	}
	if status, raw := e.raw("GET", "/v1/rooms", e.tokens["diger-admin"], nil); status != 200 || strings.Contains(string(raw), room.Name) {
		t.Fatal(status, string(raw))
	}
	if status, _ := e.raw("GET", "/v1/rooms/"+room.Name+"/log", e.tokens["diger-admin"], nil); status != 403 {
		t.Fatal(status)
	}
}

func TestYeniOdaGecersizBaslikVeUyeler(t *testing.T) {
	// NEDEN: sınırlar backend'de uygulanmalı; tarayıcı doğrulamasını atlamak yetmemeli.
	e, now := yonetimEnv(t)
	session := yonetimLogin(t, e, *now)
	for _, title := range []string{"", "  ", strings.Repeat("ş", 81), "iki\nsatır", "tab\tbaşlık", "nul\x00"} {
		body, _ := json.Marshal(map[string]any{"baslik": title, "uyeler": []string{}})
		yonetimWant(t, yonetimReq(e, "POST", "/odalar", "mehmet", session, string(body)), 400, "gecersiz_baslik")
	}
	for _, members := range [][]string{{"olmayan"}, {"Kotu"}, make([]string, 101)} {
		body, _ := json.Marshal(map[string]any{"baslik": "Başlık", "uyeler": members})
		yonetimWant(t, yonetimReq(e, "POST", "/odalar", "mehmet", session, string(body)), 400, "gecersiz_uye")
	}
	if err := e.st.RevokeAgent(context.Background(), "ali"); err != nil {
		t.Fatal(err)
	}
	yonetimWant(t, yonetimReq(e, "POST", "/odalar", "mehmet", session, `{"baslik":"Başlık","uyeler":["ali"]}`), 400, "gecersiz_uye")
}

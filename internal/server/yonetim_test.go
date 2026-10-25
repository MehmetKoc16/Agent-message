package server

import (
	"context"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/admin"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

const testTOTPSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func yonetimEnv(t *testing.T) (*testEnv, *time.Time) {
	t.Helper()
	e := izleEnv(t)
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	e.st.Now = func() time.Time { return now }
	if err := e.st.SetYonetici(context.Background(), "mehmet", testTOTPSecret); err != nil {
		t.Fatal(err)
	}
	return e, &now
}
func yonetimReq(e *testEnv, method, path, name, session, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/v1/yonetim"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+e.tokens[name])
	if session != "" {
		r.Header.Set("X-Yonetim-Oturum", session)
	}
	w := httptest.NewRecorder()
	e.srv.Handler().ServeHTTP(w, r)
	return w
}
func yonetimLogin(t *testing.T, e *testEnv, now time.Time) string {
	t.Helper()
	code, err := admin.TOTPCode(testTOTPSecret, now.Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	w := yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"`+code+`"}`)
	if w.Code != 200 {
		t.Fatalf("oturum %d %s", w.Code, w.Body.String())
	}
	var v map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v["oturum"]
}
func yonetimWant(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || code != "" && !strings.Contains(w.Body.String(), `"error":"`+code+`"`) {
		t.Fatalf("durum=%d gövde=%s beklenen=%d %s", w.Code, w.Body.String(), status, code)
	}
}

// NEDEN: standart vektörleri algoritma, endian, kırpma ve altı hane hatalarını yakalar.
func TestTOTPRFCVektorleri(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		at   int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		got, err := admin.TOTPCode(secret, tc.at/30)
		if err != nil || got != tc.want {
			t.Fatalf("%d: %s %v", tc.at, got, err)
		}
	}
}

// NEDEN: saat sapması yalnız ±1 adım olmalı; aynı kod eşzamanlı bile tekrar geçmemeli.
func TestTOTPPencereVeTekrar(t *testing.T) {
	e, now := yonetimEnv(t)
	for d := int64(-2); d <= 2; d++ {
		code, _ := admin.TOTPCode(testTOTPSecret, now.Unix()/30+d)
		_, ok := admin.MatchTOTP(testTOTPSecret, code, *now)
		if ok != (d >= -1 && d <= 1) {
			t.Fatalf("pencere %d", d)
		}
	}
	for _, code := range []string{"12345", "abcdef", "１２３４５６", "1234567"} {
		if _, ok := admin.MatchTOTP(testTOTPSecret, code, *now); ok {
			t.Fatal("kod biçimi")
		}
	}
	code, _ := admin.TOTPCode(testTOTPSecret, now.Unix()/30)
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"`+code+`"}`).Code
		}()
	}
	wg.Wait()
	close(results)
	count := map[int]int{}
	for status := range results {
		count[status]++
	}
	if count[200] != 1 || count[401] != 1 {
		t.Fatal(count)
	}
	old, _ := admin.TOTPCode(testTOTPSecret, now.Unix()/30-1)
	yonetimWant(t, yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"`+old+`"}`), 401, "kod_gecersiz")
}

// NEDEN: normal insan/ajan yönetemez; yönetici bile ikinci faktörü sunmalı.
func TestYonetimYalnizYonetici(t *testing.T) {
	e, _ := yonetimEnv(t)
	for _, name := range []string{"ali", "veli", "dis"} {
		yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", name, "", ""), 403, "yalniz_yonetici")
	}
	if err := e.st.AddAgent(context.Background(), "ece", "insan", store.HashToken("test-ece"), nil); err != nil {
		t.Fatal(err)
	}
	e.tokens["ece"] = "test-ece"
	yonetimWant(t, yonetimReq(e, "POST", "/oturum", "ece", "", `{"kod":"123456"}`), 403, "yalniz_yonetici")
	yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "mehmet", "", ""), 401, "yonetim_oturumu_gerekli")
}

// NEDEN: beş yanlış koddan sonraki kilit kod doğru olsa da 15 dakika sürmeli.
func TestYonetimOturumKilit(t *testing.T) {
	e, now := yonetimEnv(t)
	for i := 0; i < 5; i++ {
		want := 401
		if i == 4 {
			want = 429
		}
		yonetimWant(t, yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), want, "")
	}
	code, _ := admin.TOTPCode(testTOTPSecret, now.Unix()/30)
	yonetimWant(t, yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"`+code+`"}`), 429, "cok_fazla_deneme")
	*now = now.Add(15 * time.Minute)
	session := yonetimLogin(t, e, *now)
	if session == "" {
		t.Fatal("kilit kalkmadı")
	}
	*now = now.Add(30 * time.Second)
	yonetimWant(t, yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 401, "kod_gecersiz")
	yonetimLogin(t, e, *now)
	if len(e.srv.yonetimFailures) != 0 {
		t.Fatal("başarı sayacı temizlemedi")
	}
	// NEDEN: sabit pencere sınırının iki yanındaki beş deneme kilidi atlatmamalı.
	e2, clock := yonetimEnv(t)
	yonetimWant(t, yonetimReq(e2, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 401, "")
	*clock = clock.Add(14 * time.Minute)
	for i := 0; i < 3; i++ {
		yonetimWant(t, yonetimReq(e2, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 401, "")
	}
	*clock = clock.Add(2 * time.Minute)
	yonetimWant(t, yonetimReq(e2, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 401, "")
	yonetimWant(t, yonetimReq(e2, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`), 429, "cok_fazla_deneme")
}

// NEDEN: oturum hesap adına bağlıdır; süresi, yetki kaldırılması ve sır değişimi iptal eder.
func TestYonetimOturumSuresi(t *testing.T) {
	e, now := yonetimEnv(t)
	session := yonetimLogin(t, e, *now)
	ctx := context.Background()
	if err := e.st.AddAgent(ctx, "ece", "insan", store.HashToken("test-ece"), nil); err != nil {
		t.Fatal(err)
	}
	e.tokens["ece"] = "test-ece"
	if err := e.st.SetYonetici(ctx, "ece", testTOTPSecret); err != nil {
		t.Fatal(err)
	}
	yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "ece", session, ""), 401, "yonetim_oturumu_gerekli")
	*now = now.Add(10*time.Minute - time.Second)
	yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "mehmet", session, ""), 200, "")
	*now = now.Add(time.Second)
	yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "mehmet", session, ""), 401, "yonetim_oturumu_gerekli")
	session = yonetimLogin(t, e, *now)
	other := New(e.cfg, e.st, e.srv.log)
	defer other.Close()
	w := izleRequest(other.Handler(), "GET", "/v1/yonetim/hesaplar", e.tokens["mehmet"])
	yonetimWant(t, w, 401, "yonetim_oturumu_gerekli")
	if err := e.st.SetYonetici(ctx, "mehmet", "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}
	yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "mehmet", session, ""), 401, "yonetim_oturumu_gerekli")
	if err := e.st.ClearYonetici(ctx, "mehmet"); err != nil {
		t.Fatal(err)
	}
	yonetimWant(t, yonetimReq(e, "GET", "/hesaplar", "mehmet", session, ""), 403, "yalniz_yonetici")
}

// NEDEN: yeni token bir kez sunulur; web girdisiyle yönetici yetkisi kazanılamaz.
func TestYonetimHesapAc(t *testing.T) {
	e, now := yonetimEnv(t)
	session := yonetimLogin(t, e, *now)
	w := yonetimReq(e, "POST", "/hesaplar", "mehmet", session, `{"ad":"yeni","tur":"insan","odalar":["r1"],"yonetici":true}`)
	yonetimWant(t, w, 201, "")
	var v map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	token := v["token"]
	if len(token) != 43 {
		t.Fatal("token uzunluğu")
	}
	a, err := e.st.Authenticate(context.Background(), token)
	if err != nil || a.Name != "yeni" {
		t.Fatal("yeni hesap çalışmıyor")
	}
	yes, err := e.st.IsYonetici(context.Background(), "yeni")
	if err != nil || yes {
		t.Fatal("web yönetici yaptı")
	}
	db, err := sql.Open("sqlite", e.cfg.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var hash string
	if err := db.QueryRow(`SELECT token_hash FROM agents WHERE name='yeni'`).Scan(&hash); err != nil || hash != store.HashToken(token) || hash == token {
		t.Fatal("özet saklanmadı")
	}
	list := yonetimReq(e, "GET", "/hesaplar", "mehmet", session, "")
	yonetimWant(t, list, 200, "")
	for _, bad := range []string{token, hash, "totp_secret", "token_hash", testTOTPSecret} {
		if strings.Contains(list.Body.String(), bad) {
			t.Fatal("listede sır")
		}
	}
	for _, body := range []string{`{"ad":"Kotu","tur":"insan","odalar":["r"]}`, `{"ad":"a","tur":"insan","odalar":["Kotu"]}`, `{"ad":"a","tur":"insan","odalar":[]}`, `{"ad":"a","tur":"kotu","odalar":["r"]}`} {
		yonetimWant(t, yonetimReq(e, "POST", "/hesaplar", "mehmet", session, body), 400, "")
	}
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar", "mehmet", session, `{"ad":"yeni","tur":"ajan","odalar":["r1"]}`), 409, "zaten_var")
}

// NEDEN: üyelik ve iptal uygulanmalı; kendisi dahil yönetici hedefler dokunulmazdır.
func TestYonetimOdaEkleCikarIptal(t *testing.T) {
	e, now := yonetimEnv(t)
	session := yonetimLogin(t, e, *now)
	ctx := context.Background()
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar/ali/odalar", "mehmet", session, `{"oda":"r3"}`), 200, "")
	member, err := e.st.HasRoom(ctx, "ali", "r3")
	if err != nil || !member {
		t.Fatal("oda eklenmedi")
	}
	yonetimWant(t, yonetimReq(e, "DELETE", "/hesaplar/ali/odalar/r3", "mehmet", session, ""), 200, "")
	member, err = e.st.HasRoom(ctx, "ali", "r3")
	if err != nil || member {
		t.Fatal("oda çıkarılmadı")
	}
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar/ali/iptal", "mehmet", session, ""), 200, "")
	w := izleRequest(e.srv.Handler(), "GET", "/v1/me", e.tokens["ali"])
	yonetimWant(t, w, 401, "yetkisiz")
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar/ali/odalar", "mehmet", session, `{"oda":"r3"}`), 404, "hesap_yok")
	for _, tc := range []struct{ method, path, body string }{{"POST", "/hesaplar/mehmet/odalar", `{"oda":"r3"}`}, {"DELETE", "/hesaplar/mehmet/odalar/r1", ""}} {
		yonetimWant(t, yonetimReq(e, tc.method, tc.path, "mehmet", session, tc.body), 200, "")
	}
	for _, tc := range []struct{ method, path, body string }{{"POST", "/hesaplar/mehmet/iptal", ""}} {
		yonetimWant(t, yonetimReq(e, tc.method, tc.path, "mehmet", session, tc.body), 403, "yoneticiye_dokunulamaz")
	}
}

// NEDEN: audit işlemleri izlemeli; kod, sır, parola ve oturum anahtarı sızmamalı.
func TestYonetimKaydi(t *testing.T) {
	e, now := yonetimEnv(t)
	yonetimReq(e, "POST", "/oturum", "mehmet", "", `{"kod":"xxxxxx"}`)
	session := yonetimLogin(t, e, *now)
	yonetimReq(e, "POST", "/hesaplar/ali/odalar", "mehmet", session, `{"oda":"r3"}`)
	yonetimReq(e, "DELETE", "/hesaplar/ali/odalar/r3", "mehmet", session, "")
	yonetimReq(e, "POST", "/hesaplar/ali/iptal", "mehmet", session, "")
	w := yonetimReq(e, "GET", "/kayit?son=100", "mehmet", session, "")
	yonetimWant(t, w, 200, "")
	for _, operation := range []string{"oturum_acildi", "oturum_reddedildi", "odaya_eklendi", "odadan_cikarildi", "hesap_iptal"} {
		if !strings.Contains(w.Body.String(), operation) {
			t.Fatal("eksik kayıt", operation)
		}
	}
	code, _ := admin.TOTPCode(testTOTPSecret, now.Unix()/30)
	for _, bad := range []string{code, "xxxxxx", testTOTPSecret, session, e.tokens["mehmet"]} {
		if strings.Contains(w.Body.String()+e.logs.String(), bad) {
			t.Fatal("kayıtta sır")
		}
	}
}

// NEDEN: bozuk UTF-8 sessizce düzeltilmemeli, büyük gövde ve yazma sınırı aşılmamalı.
func TestYonetimGirdiVeHizSiniri(t *testing.T) {
	e, now := yonetimEnv(t)
	session := yonetimLogin(t, e, *now)
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar", "mehmet", session, "{\"ad\":\"\xff\"}"), 400, "gecersiz_govde")
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar", "mehmet", session, strings.Repeat("x", 17000)), 413, "govde_cok_buyuk")
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar", "mehmet", session, `{} {}`), 400, "gecersiz_json")
	e.srv.rate = newRateLimiter(1, 10)
	yonetimWant(t, yonetimReq(e, "POST", "/hesaplar/ali/odalar", "mehmet", session, `{"oda":"r3"}`), 200, "")
	yonetimWant(t, yonetimReq(e, "DELETE", "/hesaplar/ali/odalar/r3", "mehmet", session, ""), 429, "hiz_siniri")
}

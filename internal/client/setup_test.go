package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func setupTestToken() string { return strings.Repeat("Ab9_-xY2", 4) }

// Her setup çağrısı çıktıda token'ın tamamını ve ayırt edici parçalarını denetler.
func assertSetupNoToken(t *testing.T, token, out, errOut string) {
	t.Helper()
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return
	}
	parts := []string{trimmed}
	if len(trimmed) >= 16 {
		parts = append(parts, trimmed[:8], trimmed[len(trimmed)-8:])
	}
	for _, part := range parts {
		if strings.Contains(out, part) || strings.Contains(errOut, part) {
			t.Fatal("setup çıktısında token sızıntısı var")
		}
	}
}

func runSetupTest(t *testing.T, path, token string, args ...string) (int, string, string) {
	t.Helper()
	// Gerçek os.Pipe terminal olmayan platform yolunu da çalıştırır.
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	if _, err := io.WriteString(wr, token); err != nil {
		wr.Close()
		t.Fatal(err)
	}
	wr.Close()
	var out, errb bytes.Buffer
	full := append([]string{"setup", "--config", path}, args...)
	code := Run(context.Background(), full, Env{Stdin: rd, Stdout: &out, Stderr: &errb})
	assertSetupNoToken(t, token, out.String(), errb.String())
	if strings.Contains(errb.String(), "Token: ") || strings.Contains(errb.String(), "yansıtma") {
		t.Fatal("boruda istem ya da terminal uyarısı gösterilmemeli")
	}
	return code, out.String(), errb.String()
}

func setupArgs(url string) []string {
	return []string{"--url", url, "--room", "oda-1", "--agent", "ali.ajan"}
}

func assertSetupConfig(t *testing.T, path, url, token string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal("ayar JSON'u bozuk")
	}
	if len(fields) != 4 || fields["url"] != url || fields["room"] != "oda-1" || fields["agent"] != "ali.ajan" || fields["token"] != token {
		t.Fatal("ayar JSON alanları beklenen değerlerle uyuşmuyor")
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("mevcut okuyucuyla uyumsuz ayar: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("geçici dosya ya da setup kilidi kalmış")
	}
}

// NEDEN: borudan gelen token geçmişe/çıktıya sızmadan mevcut JSON biçiminde saklanmalı.
func Test_setup_dosyayi_dogru_yazar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ayar", "config.json")
	token := setupTestToken()
	args := append(setupArgs("https://relay.example.com/"), "--no-check")
	code, _, errOut := runSetupTest(t, path, " \t"+token+" \r\n", args...)
	if code != ExitOK || errOut != "" {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	assertSetupConfig(t, path, "https://relay.example.com", token)
	if runtime.GOOS != "windows" {
		for p, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
			st, err := os.Stat(p)
			if err != nil || st.Mode().Perm() != want {
				t.Fatal("ayar izinleri beklenen değerle uyuşmuyor")
			}
		}
	}
}

// NEDEN: kullanıcı --force demeden mevcut kimlik bilgileri değiştirilmemeli.
func Test_setup_mevcut_dosyanin_ustune_yazmaz(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("eski ayar"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runSetupTest(t, path, setupTestToken(), append(setupArgs("https://relay.example.com"), "--no-check")...)
	if code != ExitError || !strings.Contains(errOut, "--force") {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "eski ayar" {
		t.Fatal("mevcut dosya değişmiş")
	}
}

// NEDEN: açık --force isteği mevcut dosyayı tam ve okunabilir ayarla değiştirmeli.
func Test_setup_force_ustune_yazar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("eski ayar"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runSetupTest(t, path, setupTestToken(), append(setupArgs("https://relay.example.com"), "--force", "--no-check")...)
	if code != ExitOK {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	assertSetupConfig(t, path, "https://relay.example.com", setupTestToken())
}

// NEDEN: boş token ile kullanılamayan bir ayar dosyası oluşturulmamalı.
func Test_setup_bos_token_reddeder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	code, _, errOut := runSetupTest(t, path, " \t\r\n", append(setupArgs("https://relay.example.com"), "--no-check")...)
	if code != ExitError || !strings.Contains(errOut, "token boş") {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("boş token için dosya oluşturulmuş")
	}
}

// NEDEN: base64url dışındaki karakterler, padding ve uzunluk sınırı ihlalleri reddedilmeli.
func Test_setup_gecersiz_token_karakteri_reddeder(t *testing.T) {
	for _, token := range []string{setupTestToken() + "+", setupTestToken() + "/", setupTestToken() + "=", setupTestToken() + "ğ", strings.Repeat("x", 19), strings.Repeat("y", 201)} {
		path := filepath.Join(t.TempDir(), "config.json")
		code, _, errOut := runSetupTest(t, path, token, append(setupArgs("https://relay.example.com"), "--no-check")...)
		if code != ExitError || !strings.Contains(errOut, "token geçersiz") {
			t.Fatalf("setup: %d %q", code, errOut)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("geçersiz token için dosya oluşturulmuş")
		}
	}
}

// NEDEN: oda ve ajan aynı dar ad sözleşmesini izlemeli; URL yolu/metaveri bozulmamalı.
func Test_setup_gecersiz_ad_reddeder(t *testing.T) {
	for _, flag := range []string{"--room", "--agent"} {
		for _, name := range []string{"Ali", "-ali", "ali/veli", "ali veli", "", strings.Repeat("a", 65)} {
			path := filepath.Join(t.TempDir(), "config.json")
			args := append(setupArgs("https://relay.example.com"), flag, name, "--no-check")
			code, _, _ := runSetupTest(t, path, setupTestToken(), args...)
			if code != ExitError {
				t.Fatalf("%s geçersiz ad kabul edildi", flag)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("geçersiz ad için dosya oluşturulmuş")
			}
		}
	}
}

// NEDEN: token uzak bir adrese düz HTTP ile gönderilmemeli, --no-check bunu atlayamamalı.
func Test_setup_http_uzak_url_reddeder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	code, _, errOut := runSetupTest(t, path, setupTestToken(), append(setupArgs("http://relay.example.com"), "--no-check")...)
	if code != ExitError || !strings.Contains(errOut, "https olmalı") {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("uzak http için dosya oluşturulmuş")
	}
}

// NEDEN: 401 kimlik bilgisinin geçersiz olduğunu kanıtlar; kaydedilen token silinmeli.
func Test_setup_401_dosyayi_siler(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":%q}`, setupTestToken())
	}))
	defer ts.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	code, _, errOut := runSetupTest(t, path, setupTestToken(), setupArgs(ts.URL)...)
	if code != ExitError || !strings.Contains(errOut, "token geçersiz") {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("401 sonrası dosya silinmemiş")
	}
}

// NEDEN: bağlantı kesintisi token'ın yanlış olduğunu göstermez; ayar kalmalı, kod 3 olmalı.
func Test_setup_ulasilamazsa_dosyayi_birakir_cikis_3(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	ts.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	code, _, errOut := runSetupTest(t, path, setupTestToken(), setupArgs(ts.URL)...)
	if code != ExitUnreachable || !strings.Contains(errOut, "uyarı:") || !strings.Contains(errOut, "bırakıldı") {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	assertSetupConfig(t, path, ts.URL, setupTestToken())
}

// NEDEN: setup doğru odaya agents isteği yapmalı, token'ı yalnız Authorization'a koymalı.
func Test_setup_baglanti_tamam(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/rooms/oda-1/agents" || r.Header.Get("Authorization") != "Bearer "+setupTestToken() {
			t.Error("agents isteği beklenen alanlarla uyuşmuyor")
		}
		fmt.Fprintf(w, `{"agents":[{"name":%q},{"name":"veli"}]}`, setupTestToken())
	}))
	defer ts.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	code, out, errOut := runSetupTest(t, path, setupTestToken(), setupArgs(ts.URL)...)
	if code != ExitOK || out != "bağlantı tamam: oda-1 odasında 2 ajan\n" || errOut != "" {
		t.Fatalf("setup: %d %q %q", code, out, errOut)
	}
	assertSetupConfig(t, path, ts.URL, setupTestToken())
}

// NEDEN: geliştirme ikilisi ayar dosyası gerektirmeden sürüm ve Go bilgisini vermeli.
func Test_version_dev_yazar(t *testing.T) {
	var out, errb bytes.Buffer
	code := Run(context.Background(), []string{"version"}, Env{Stdout: &out, Stderr: &errb, DefaultConfig: filepath.Join(t.TempDir(), "yok.json")})
	want := "relay sürüm=dev commit=dev Go=" + runtime.Version() + "\n"
	if code != ExitOK || out.String() != want || errb.Len() != 0 {
		t.Fatalf("version: %d %q %q", code, out.String(), errb.String())
	}
}

// NEDEN: Ctrl+C, satır henüz gelmemişken de 130 vermeli ve ayar oluşturmamalı.
func Test_setup_stdin_beklerken_iptal(t *testing.T) {
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	defer wr.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	path := filepath.Join(t.TempDir(), "config.json")
	var out, errb bytes.Buffer
	args := append([]string{"setup", "--config", path}, setupArgs("https://relay.example.com")...)
	code := Run(ctx, args, Env{Stdin: rd, Stdout: &out, Stderr: &errb})
	assertSetupNoToken(t, setupTestToken(), out.String(), errb.String())
	if code != ExitCanceled || !strings.Contains(errb.String(), "iptal edildi") {
		t.Fatalf("setup iptal: %d %q", code, errb.String())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("iptal sonrası ayar oluşturulmuş")
	}
}

// NEDEN: --token ve konumsal token kabul edilmemeli; gizli değer yalnız stdin'den gelmeli.
func Test_setup_token_argumani_reddeder(t *testing.T) {
	for _, extra := range [][]string{{"--token", setupTestToken()}, {setupTestToken()}} {
		path := filepath.Join(t.TempDir(), "config.json")
		args := append(setupArgs("https://relay.example.com"), extra...)
		code, _, _ := runSetupTest(t, path, setupTestToken(), args...)
		if code != ExitError {
			t.Fatal("token argümanı kabul edilmiş")
		}
	}
}

// NEDEN: sunucunun hata gövdesi sır içerebilir; setup onu hiçbir hata yolunda basmamalı.
func Test_setup_sunucu_hata_metnini_basmaz(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status == http.StatusOK {
					fmt.Fprint(w, setupTestToken()) // bozuk JSON
					return
				}
				fmt.Fprintf(w, `{"error":%q}`, setupTestToken())
			}))
			defer ts.Close()
			path := filepath.Join(t.TempDir(), "config.json")
			code, _, errOut := runSetupTest(t, path, setupTestToken(), setupArgs(ts.URL)...)
			want := ExitError
			if status == http.StatusServiceUnavailable {
				want = ExitUnreachable
			}
			if code != want {
				t.Fatalf("setup: %d %q", code, errOut)
			}
			if status == http.StatusOK && !strings.Contains(errOut, "geçersiz yanıt") {
				t.Fatal("bozuk JSON anlamlı hata vermedi")
			}
			assertSetupConfig(t, path, ts.URL, setupTestToken())
		})
	}
}

// NEDEN: stdin tek satır sözleşmesine uymalı; ikinci satır token'a eklenmemeli/tüketilmemeli.
func Test_setup_yalniz_ilk_satiri_okur(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	stdin := strings.NewReader(setupTestToken() + "\r\nikinci satır\n")
	var out, errb bytes.Buffer
	args := append([]string{"--config", path, "setup"}, append(setupArgs("https://relay.example.com"), "--no-check")...)
	code := Run(context.Background(), args, Env{Stdin: stdin, Stdout: &out, Stderr: &errb})
	assertSetupNoToken(t, setupTestToken(), out.String(), errb.String())
	if code != ExitOK {
		t.Fatalf("setup: %d %q", code, errb.String())
	}
	remaining, err := io.ReadAll(stdin)
	if err != nil || string(remaining) != "ikinci satır\n" {
		t.Fatal("ikinci satır tüketilmiş")
	}
	assertSetupConfig(t, path, "https://relay.example.com", setupTestToken())
}

// NEDEN: --force ile girilen yanlış token, çalışan eski ayarı silmemeli; 401 sonrası
// önceki dosya bayt bayt geri gelmeli ve yedek/geçici dosya kalmamalı.
func Test_setup_force_401_eski_ayari_geri_yukler(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"yetkisiz"}`)
	}))
	defer ts.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	old := []byte(`{"url":"https://relay.example.com","room":"oda-1","agent":"ali.ajan","token":"eski-sahte-kimlik-0123456789"}` + "\n")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runSetupTest(t, path, setupTestToken(), append(setupArgs(ts.URL), "--force")...)
	if code != ExitError || !strings.Contains(errOut, "token geçersiz") || !strings.Contains(errOut, "önceki ayar geri yüklendi") {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, old) {
		t.Fatal("eski ayar geri yüklenmedi")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatal("yedek ya da geçici dosya kalmış")
	}
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
			t.Fatal("geri yüklenen ayarın izni 0600 değil")
		}
	}
}

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

// buyukSunucu sayfa sınırı olmayan (eski/başka) bir sunucudur: after'dan sonraki her mesajı tek yanıtta döner.
func buyukSunucu(t *testing.T, n, bodyBytes int, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	body := strings.Repeat("a", bodyBytes)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		msgs := []proto.Message{}
		for id := after + 1; id <= int64(n); id++ {
			msgs = append(msgs, proto.Message{ID: id, Room: "r1", From: "ali", Type: "bilgi", Thread: "T-1", Turn: 1, MaxTurns: 20, Body: body})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"messages": msgs})
	}))
	t.Cleanup(ts.Close)
	return ts
}

var cerceveID = regexp.MustCompile(`<relay-message id="(\d+)"`)

// NEDEN: 4 MiB'ı aşan yanıt eskiden sessizce kesilip "geçersiz yanıt" veriyordu; imleç ilerlemediği
// için inbox/wait/bridge kalıcı kilitleniyordu. Sınıra sığan tam mesajlar teslim edilmeli, imleç
// yalnız onlar kadar ilerlemeli; hiçbir mesaj atlanmamalı ya da yinelenmemeli.
func TestBuyukYanitParcaParcaEksiksizOkunur(t *testing.T) {
	const n = 60
	var calls atomic.Int32
	ts := buyukSunucu(t, n, 100<<10, &calls) // ~6 MiB
	cfg := fakeConfig(t, ts.URL)
	var got []int64
	for i := 0; ; i++ {
		if i > 10 {
			t.Fatalf("ilerleme yok: %d çağrıda %d mesaj", i, len(got))
		}
		code, out, errOut := runWith(context.Background(), cfg, "inbox")
		if code != 0 {
			t.Fatalf("çağrı %d: çıkış %d err=%q", i, code, errOut)
		}
		if strings.Contains(out, "yeni mesaj yok") {
			break
		}
		ids := cerceveID.FindAllStringSubmatch(out, -1)
		if len(ids) == 0 {
			t.Fatalf("çağrı %d: mesajsız çıktı", i)
		}
		for _, m := range ids {
			id, _ := strconv.ParseInt(m[1], 10, 64)
			got = append(got, id)
		}
	}
	if len(got) != n {
		t.Fatalf("%d mesaj bekleniyordu, %d geldi", n, len(got))
	}
	for i, id := range got {
		if id != int64(i+1) {
			t.Fatalf("sıra bozuk ya da mesaj atlandı: %d. sırada id=%d", i, id)
		}
	}
	if calls.Load() < 3 {
		t.Fatalf("yanıt sınırı hiç aşılmadı: %d istek", calls.Load())
	}
}

// NEDEN: wait de aynı yolu kullanır; kesilen yanıtta sığan mesajları basıp 0 dönmeli.
func TestBuyukYanitWaitIlerler(t *testing.T) {
	var calls atomic.Int32
	ts := buyukSunucu(t, 60, 100<<10, &calls)
	code, out, errOut := runWith(context.Background(), fakeConfig(t, ts.URL), "wait", "--timeout", "5")
	if code != 0 || !strings.Contains(out, `<relay-message id="1"`) || strings.Contains(out, `<relay-message id="60"`) {
		t.Fatalf("çıkış %d err=%q", code, errOut)
	}
}

// NEDEN: sınıra tek mesaj bile sığmıyorsa hata "geçersiz yanıt"tan ayırt edilmeli ve
// güvenilmeyen gövdeyi yazmamalı; yarım mesaj teslim edilmemeli.
func TestTekMesajSinirdanBuyukseAcikHata(t *testing.T) {
	var calls atomic.Int32
	ts := buyukSunucu(t, 1, 5<<20, &calls)
	cfg := fakeConfig(t, ts.URL)
	for _, args := range [][]string{{"inbox"}, {"wait", "--timeout", "0"}, {"thread", "T-1"}} {
		code, out, errOut := runWith(context.Background(), cfg, args...)
		if code != 1 || out != "" || !strings.Contains(errOut, "çok büyük") || strings.Contains(errOut, "geçersiz yanıt") || len(errOut) > 200 {
			t.Fatalf("%v: çıkış %d out=%d bayt err=%.200q", args, code, len(out), errOut)
		}
	}
}

// NEDEN: kesilen gövdenin sonundaki yarım öğe ya da sırasız id imleci ileri atlatmamalı.
func TestKesilmisMesajlarYalnizTamOgeleriAlir(t *testing.T) {
	full := `{"x":[1,{"messages":[]}],"messages":[{"id":1,"body":"a"},{"id":2,"body":"b"},{"id":3,"body":"c`
	for _, tc := range []struct {
		in   string
		want []int64
	}{
		{full, []int64{1, 2}},
		{full[:strings.Index(full, `,{"id":3`)], []int64{1, 2}},
		{full[:strings.Index(full, `"b"}`)+2], []int64{1}},
		{`{"messages":[{"id":5},{"id":4},{"id":6}],"z":`, []int64{5}}, // sırasız: imleç 4'ü atlamasın
		{`{"messages":[{"id":1,"body":"\xff`, nil},
		{`{"messages":{"id":1}`, nil},
		{`[{"id":1}`, nil},
		{``, nil},
	} {
		var got []int64
		for _, m := range partialMessages([]byte(tc.in)) {
			got = append(got, m.ID)
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%q: %v, beklenen %v", tc.in, got, tc.want)
		}
	}
}

// NEDEN: çıkış 3 ajana "dinlemeyi bırak" der. Taşan ya da sayı olmayan --timeout hiç istek
// yapılmadan 3 dönüyordu; bunlar kullanım hatasıdır (1).
func TestWaitGecersizSureKullanimHatasi(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"messages":[]}`))
	}))
	defer ts.Close()
	cfg := fakeConfig(t, ts.URL)
	for _, v := range []string{"1e300", "NaN", "Inf", "-Inf", "9300000000", "86401", "-1", "-1e-12"} {
		code, _, errOut := runWith(context.Background(), cfg, "wait", "--timeout="+v)
		if code != ExitError || !strings.Contains(errOut, "kullanım: relay wait") {
			t.Errorf("--timeout=%s: çıkış %d err=%q", v, code, errOut)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("geçersiz süreyle %d istek yapıldı", calls.Load())
	}
	// Geçerli ama istek yapmaya yetmeyen süre kesinti değil, zaman aşımıdır.
	for _, v := range []string{"1e-12", "1e-300"} {
		code, _, errOut := runWith(context.Background(), cfg, "wait", "--timeout="+v)
		if code != ExitTimeout || !strings.Contains(errOut, "zaman aşımı") {
			t.Errorf("--timeout=%s: çıkış %d err=%q", v, code, errOut)
		}
	}
	// Üst sınırın kendisi geçerlidir.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code, _, errOut := runWith(ctx, cfg, "wait", "--timeout=86400"); code != ExitCanceled {
		t.Errorf("--timeout=86400: çıkış %d err=%q", code, errOut)
	}
}

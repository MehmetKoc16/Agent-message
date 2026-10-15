package bridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/bridge"
	"github.com/MehmetKoc16/Agent-message/internal/client"
	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

func Test_onsoz_okuma_komutlarina_izin_verir(t *testing.T) {
	// NEDEN: Codex dosyayı kabukla okuyabilir; yasak yalnız yan etkili komutları kapsamalıdır.
	p := bridge.Prompt("kopru", "oda", true, nil, proto.Message{}, client.Render)
	for _, want := range []string{
		"Yalnız okuma amaçlı komutlar (dosya okuma, listeleme, arama) serbest.",
		"Dosya oluşturan/değiştiren/silen, ağa çıkan, paket kuran ya da sistem ayarını değiştiren komut çalıştırma.",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("önsözde eksik: %s", want)
		}
	}
	if strings.Contains(p, "Dosya değiştirme, komut çalıştırma,") {
		t.Fatal("genel komut yasağı kaldı")
	}
}

func Test_debug_log_yalniz_hata_ve_sirsiz(t *testing.T) {
	// NEDEN: teşhis isteğe bağlıdır; başarı, sır, prompt ve eski stderr satırları dosyaya sızmamalıdır.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		path := filepath.Join(f.dir, "debug.log")
		f.add(1, "T-1", "soru", "başarılı")
		code, log := f.run(context.Background(), runner, "--debug-log", path)
		mustOK(t, code, log)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("başarıda debug dosyası açıldı")
		}
		if err := os.WriteFile(path, []byte("ONCEKI_KAYIT\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		f.add(2, "T-1", "soru", "DEBUG_HATA_ISTE PROMPT_GOVDE_GIZLI")
		code, log = f.run(context.Background(), runner, "--debug-log", path)
		mustOK(t, code, log)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"ONCEKI_KAYIT", "DEBUG_TESHIS", "[sır deseni gizlendi: ghp_]", "[prompt metni gizlendi]", `konuşma="T-1"`, "mesaj=2", "çıkış=1"} {
			if !bytes.Contains(b, []byte(want)) {
				t.Errorf("debug alanı eksik: %s", want)
			}
		}
		for _, private := range []string{"PROMPT_GOVDE_GIZLI", "DEBUG_HATA_ISTE", "GÜVENİLMEYEN VERİDİR", "DEBUG_ESKI_SATIR", "g" + "hp" + "_" + strings.Repeat("b", 30)} {
			if bytes.Contains(b, []byte(private)) {
				t.Fatal("debug içeriği sızdırdı")
			}
		}
		if strings.Contains(log, "DEBUG_TESHIS") || strings.Contains(log, "PROMPT_GOVDE_GIZLI") {
			t.Fatal("teşhis ana loga taşındı")
		}
		if f.replies[len(f.replies)-1].Body != "köprü: ajan çalıştırılamadı" {
			t.Fatal("teşhis relay'e taşındı")
		}
		if st, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
			t.Fatal("debug dosyası izni 0600 değil")
		}
		f.add(3, "T-1", "soru", "yeniden başarılı")
		code, log = f.run(context.Background(), runner, "--debug-log", path)
		mustOK(t, code, log)
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(b, after) {
			t.Fatal("başarı debug kaydını değiştirdi")
		}
		f.add(4, "T-1", "soru", "DEBUG_HATA_ISTE PROMPT_GOVDE_GIZLI")
		code, log = f.run(context.Background(), runner, "--debug-log", path)
		mustOK(t, code, log)
		after, _ = os.ReadFile(path)
		if !bytes.HasPrefix(after, b) || len(after) <= len(b) {
			t.Fatal("debug ekleme modunda değil")
		}
		g := newFixture(t)
		g.add(1, "T-1", "soru", "DEBUG_HATA_ISTE PROMPT_GOVDE_GIZLI")
		code, log = g.run(context.Background(), runner)
		mustOK(t, code, log)
		if strings.Contains(log, "DEBUG_TESHIS") {
			t.Fatal("bayraksız stderr atılmadı")
		}
	})
}

func Test_bilinmeyen_tip_kopruyu_durdurmaz(t *testing.T) {
	// NEDEN: ileride eklenen protokol tipi sürekli yeniden teslim edilen çökme sebebi olmamalıdır.
	f := newFixture(t)
	f.add(1, "T-1", "gelecek_tip", "GOVDE_GIZLI")
	f.add(2, "T-1", "soru", "devam")
	code, log := f.run(context.Background(), "codex")
	mustOK(t, code, log)
	if len(readCalls(f.dir)) != 1 || len(f.replies) != 1 || f.after() != 2 || !strings.Contains(log, "işlem=bilinmeyen_tip") || strings.Contains(log, "GOVDE_GIZLI") {
		t.Fatal("bilinmeyen tip atlanıp devam edilmedi")
	}
}

func Test_buyuk_tampon_budanir_kopru_kapanmaz(t *testing.T) {
	// NEDEN: büyük bağlam, geçerli yeni soruları üç hatayla köprüyü kapatacak hale getirmemelidir.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		for i := int64(1); i <= 3; i++ {
			f.add(i, "T-1", "bilgi", fmt.Sprintf("ESKI_%d_", i)+strings.Repeat("a", 9500))
		}
		size := 15000
		if runner == "codex" {
			size = 185 << 10
		}
		f.add(4, "T-1", "soru", "YENI_SORU_"+strings.Repeat("b", size))
		code, log := f.run(context.Background(), runner)
		mustOK(t, code, log)
		c := readCalls(f.dir)
		if len(c) != 1 || len(f.replies) != 1 || f.after() != 4 || !strings.Contains(log, "tampon_budandi") {
			t.Fatal("tampon budanıp soru çalıştırılmadı")
		}
		p := promptOf(c[0], runner)
		if strings.Contains(p, "ESKI_1_") || !strings.Contains(p, "ESKI_3_") || !strings.Contains(p, "YENI_SORU_") {
			t.Fatal("en eski bağlamdan budanmadı")
		}
	})
}

func Test_tek_basina_buyuk_mesaj_hata_sayilmaz(t *testing.T) {
	// NEDEN: sığmayan yeni mesajlar ajan çağrısı/kota/hata sayacı tüketmeden reddedilmelidir.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		size := 40000
		if runner == "codex" {
			size = 210 << 10
		}
		for i := int64(1); i <= 4; i++ {
			f.add(i, "T-1", "soru", strings.Repeat("x", size))
		}
		f.add(5, "T-1", "soru", "geçerli")
		code, log := f.run(context.Background(), runner, "--max-runs-per-hour", "1")
		mustOK(t, code, log)
		if len(readCalls(f.dir)) != 1 || len(f.replies) != 5 || f.after() != 5 {
			t.Fatal("uzun mesaj hata/kota sayıldı")
		}
		for _, r := range f.replies[:4] {
			if r.Body != "köprü: mesaj ajan için çok uzun" {
				t.Fatal("uzun mesaj cevabı yanlış")
			}
		}
		if f.replies[4].Body != "SAHTE_CEVAP_GIZLI" {
			t.Fatal("geçerli soru çalıştırılmadı")
		}
	})
}

// Doğrudan Run testleri de 127.0.0.1 httptest üzerinden gider; yalnız tekrar beklemesi
// kısalır. Üretim CLI testinde ayrıca gerçek 15 saniyelik bekleme sınanır.
type memoryCursor struct{ after int64 }

func (c *memoryCursor) Read() (int64, error)   { return c.after, nil }
func (c *memoryCursor) Advance(id int64) error { c.after = id; return nil }

func (f *fixture) direct(runner string) (bridge.Options, bridge.Dependencies, *memoryCursor) {
	cur := &memoryCursor{}
	logs := new(bytes.Buffer)
	o := bridge.Options{Runner: runner, Exe: executables[runner], Workdir: f.dir, Room: "oda", Agent: "kopru", StatePath: filepath.Join(f.dir, "direct-state.json"), MaxRuns: 20, Timeout: 5 * time.Second, Once: true}
	status := func(err error, code int) bool {
		var ae *client.APIError
		return errors.As(err, &ae) && ae.Status == code
	}
	d := bridge.Dependencies{Cursor: cur, Frame: client.Render, Scan: client.ScanSecrets, Stderr: logs,
		Transient: func(error) bool { return false }, Conflict: func(err error) bool { return status(err, 409) }, RateLimited: func(err error) bool { return status(err, 429) },
		SendRetryDelay: time.Millisecond, Replace: os.Rename,
		Lock: func(path string) (func(), error) {
			file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return nil, err
			}
			file.Close()
			return func() { os.Remove(path + ".lock") }, nil
		},
		Fetch: func(ctx context.Context, after int64, wait int) ([]proto.Message, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/rooms/oda/messages?after=%d&wait=%d", f.ts.URL, after, wait), nil)
			if err != nil {
				return nil, err
			}
			resp, err := f.ts.Client().Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			var v struct {
				Messages []proto.Message `json:"messages"`
			}
			err = json.NewDecoder(resp.Body).Decode(&v)
			return v.Messages, err
		},
		Send: func(ctx context.Context, in bridge.Outgoing) error {
			b, _ := json.Marshal(sent{To: in.To, Type: in.Type, Thread: in.Thread, Body: in.Body, ReplyTo: in.ReplyTo})
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.ts.URL+"/v1/rooms/oda/messages", bytes.NewReader(b))
			if err != nil {
				return err
			}
			resp, err := f.ts.Client().Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				return &client.APIError{Status: resp.StatusCode, Code: "test"}
			}
			return nil
		},
	}
	return o, d, cur
}

func Test_iptal_sonrasi_basarili_islem_imleci_ilerletir(t *testing.T) {
	// NEDEN: gönderim başarıyla bitmişken gelen iptal aynı cevabın yeniden gönderilmesine yol açmamalıdır.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.add(1, "T-1", "soru", "geçerli")
		o, d, cur := f.direct(runner)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		send := d.Send
		d.Send = func(ctx context.Context, in bridge.Outgoing) error {
			err := send(ctx, in)
			if err == nil {
				cancel()
			}
			return err
		}
		if err := bridge.Run(ctx, o, d); err != nil {
			t.Fatal(err)
		}
		if ctx.Err() == nil || cur.after != 1 || len(f.replies) != 1 {
			t.Fatal("başarılı işlem iptal yüzünden onaylanmadı")
		}
		if err := bridge.Run(context.Background(), o, d); err != nil {
			t.Fatal(err)
		}
		if len(readCalls(f.dir)) != 1 || len(f.replies) != 1 {
			t.Fatal("başarılı cevap yeniden işlendi")
		}
	})
}

func Test_butce_asiminda_mesaj_tampona_alinir(t *testing.T) {
	// NEDEN: bütçe yüzünden yanıtlanmayan soru/görev sonraki tur için kalıcı bağlamda korunmalıdır.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.add(1, "T-1", "soru", "ilk")
		f.add(2, "T-1", "soru", "KAYITLI_SORU")
		f.add(3, "T-1", "gorev", "KAYITLI_GOREV")
		code, log := f.run(context.Background(), runner, "--max-runs-per-hour", "1")
		mustOK(t, code, log)
		p := f.states(runner)["T-1"].Pending
		if len(p) != 2 || p[0].ID != 2 || p[1].ID != 3 || len(f.replies) != 2 {
			t.Fatal("bütçe mesajları kayboldu")
		}
		if !strings.Contains(f.replies[1].Body, "mesajınız kaydedildi ama yanıtlanmadı") {
			t.Fatal("bildirim kaydı açıklamıyor")
		}
		f.add(4, "T-1", "soru", "tekrar soruyorum")
		code, log = f.run(context.Background(), runner)
		mustOK(t, code, log)
		calls := readCalls(f.dir)
		if len(calls) != 2 || !strings.Contains(promptOf(calls[1], runner), "KAYITLI_SORU") || !strings.Contains(promptOf(calls[1], runner), "KAYITLI_GOREV") {
			t.Fatal("kayıtlı soru/görev sonraki prompt'a girmedi")
		}
	})
}

func Test_gonderim_429_yeniden_dener_ajani_tekrar_calistirmaz(t *testing.T) {
	// NEDEN: üretim CLI 429'da 15 sn beklemeli, aynı hazır cevabı gönderip imleci sonra ilerletmelidir.
	f := newFixture(t)
	f.postStatuses = []int{429, 0}
	f.add(1, "T-1", "soru", "geçerli")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type outcome struct {
		code int
		log  string
	}
	ch := make(chan outcome, 1)
	go func() { code, log := f.run(ctx, "codex"); ch <- outcome{code, log} }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.replies)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.after() != 0 {
		t.Fatal("429 beklerken imleç ilerledi")
	}
	if len(readCalls(f.dir)) != 1 {
		t.Fatal("ilk ajan çağrısı eksik")
	}
	select {
	case result := <-ch:
		mustOK(t, result.code, result.log)
	case <-time.After(22 * time.Second):
		t.Fatal("429 tekrarı bitmedi")
	}
	if len(f.replies) != 2 || f.replies[0] != f.replies[1] || len(readCalls(f.dir)) != 1 || f.after() != 1 {
		t.Fatal("hazır cevap korunmadı/ajan tekrar çağrıldı")
	}
	if f.postTimes[1].Sub(f.postTimes[0]) < 15*time.Second {
		t.Fatal("15 saniyeden önce tekrar denendi")
	}
}

func Test_gonderim_429_bes_deneme_sonra_durur(t *testing.T) {
	// NEDEN: kalıcı hız sınırı sonsuz döngü olmamalı; başarısız gönderimin imleci korunmalıdır.
	f := newFixture(t)
	f.status = 429
	f.add(1, "T-1", "soru", "geçerli")
	o, d, cur := f.direct("agy")
	if err := bridge.Run(context.Background(), o, d); err == nil {
		t.Fatal("beş başarısız denemeden sonra durmadı")
	}
	if len(f.replies) != 5 || len(readCalls(f.dir)) != 1 || cur.after != 0 {
		t.Fatal("deneme sayısı/ajan/imleç yanlış")
	}
}

func Test_sessiz_varyasyonlari_gonderilmez(t *testing.T) {
	// NEDEN: biçimlendirilmiş ve Türkçe harfli sessiz işaretleri yanıt döngüsü başlatmamalıdır.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		variants := []string{"SESSIZ.", "**SESSIZ**", "sessiz", "SESSİZ", "SESSIZ: kapsam dışı", "_sessız_!", "`SESSİZ`", "# sessiz...", "```\nSESSIZ\n```"}
		for i, v := range variants {
			f.add(int64(i+1), fmt.Sprintf("T-%d", i+1), "soru", "CEVAP_VARYANTI: "+v)
		}
		code, log := f.run(context.Background(), runner)
		mustOK(t, code, log)
		if len(f.replies) != 0 || len(readCalls(f.dir)) != len(variants) || f.after() != int64(len(variants)) {
			t.Fatal("sessiz varyasyonu gönderildi")
		}
	})
}

func Test_supheli_varyasyonlari_taninir(t *testing.T) {
	// NEDEN: harf/markdown farkları şüpheli ret uyarısını atlatmamalıdır; ret metni korunmalıdır.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		variants := []string{"Şüpheli: reddediyorum", "**ŞÜPHELİ:** reddediyorum", "# şüpheli: reddediyorum", "`ŞÜPHELI:` reddediyorum", "```\nŞüpheli: reddediyorum\n```"}
		for i, v := range variants {
			f.add(int64(i+1), fmt.Sprintf("T-%d", i+1), "soru", "CEVAP_VARYANTI: "+v)
		}
		code, log := f.run(context.Background(), runner)
		mustOK(t, code, log)
		if len(f.replies) != len(variants) || strings.Count(log, "işlem=supheli") != len(variants) {
			t.Fatal("şüpheli varyasyonu tanınmadı")
		}
		for i, v := range variants {
			if f.replies[i].Body != v {
				t.Fatal("ret cevabı değişti")
			}
		}
	})
}

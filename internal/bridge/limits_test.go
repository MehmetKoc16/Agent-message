package bridge

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

func Test_kayan_pencere_saat_sinirinda_yenilenir(t *testing.T) {
	// NEDEN: sınır sabit saat başına değil son bir saate uygulanır; eski uyarılar unutulur.
	now := time.Date(2026, 10, 14, 12, 30, 0, 0, time.UTC)
	e := engine{o: Options{MaxRuns: 2}, runs: []time.Time{now.Add(-time.Hour), now.Add(-30 * time.Minute)}, notified: map[string]time.Time{"T-1": now.Add(-time.Hour), "T-2": now.Add(-time.Minute)}}
	if !e.allowed(now) || len(e.runs) != 1 {
		t.Fatal("eski çalıştırma pencereden düşmedi")
	}
	if _, ok := e.notified["T-1"]; ok {
		t.Fatal("eski uyarı düşmedi")
	}
	if _, ok := e.notified["T-2"]; !ok {
		t.Fatal("yeni uyarı unutuldu")
	}
	e.runs = append(e.runs, now)
	if e.allowed(now.Add(29 * time.Minute)) {
		t.Fatal("kayan pencere erken açıldı")
	}
	if !e.allowed(now.Add(30 * time.Minute)) {
		t.Fatal("pencere yenilenmedi")
	}
}

func Test_calistirici_kesfi_yalniz_sahte_evde_en_yeniyi_secer(t *testing.T) {
	// NEDEN: kurulum keşfi ortam değişkeni ve gerçek kullanıcı klasörü kullanmadan sınanır.
	home := t.TempDir()
	old := filepath.Join(home, "AppData", "Local", "OpenAI", "Codex", "bin", "eski", "codex.exe")
	newer := filepath.Join(home, "AppData", "Local", "OpenAI", "Codex", "bin", "yeni", "codex.exe")
	agy := filepath.Join(home, "AppData", "Local", "agy", "bin", "agy.exe")
	if discover(home, "codex") != "" || discover(home, "agy") != "" {
		t.Fatal("olmayan çalıştırıcı bulundu")
	}
	for _, path := range []string{old, newer, agy} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("sahte; çalıştırma"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(old, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if discover(home, "codex") != newer || discover(home, "agy") != agy {
		t.Fatal("keşif sonucu yanlış")
	}
}

func Test_agy_utf16_siniri_ve_oturum_kimligi(t *testing.T) {
	// NEDEN: Windows'ta emoji iki UTF-16 birimi tutar; oturum kimliği bayrak olamaz.
	o := Options{Runner: "agy", Model: "model", Timeout: 900 * time.Second}
	if _, err := arguments(o, "", "", strings.Repeat("a", 28000)); err != nil {
		t.Fatal(err)
	}
	if _, err := arguments(o, "", "", strings.Repeat("a", 40000)); err == nil {
		t.Fatal("uzun prompt kabul edildi")
	}
	if _, err := arguments(o, "", "", strings.Repeat("😀", 16001)); err == nil {
		t.Fatal("UTF-16 boyutu göz ardı edildi")
	}
	if runtime.GOOS == "windows" {
		if _, err := arguments(o, "", "", strings.Repeat(`"`, 16000)); err == nil {
			t.Fatal("Windows tırnak kaçışlarının boyutu göz ardı edildi")
		}
		o.Model = strings.Repeat("m", 4000)
		if _, err := arguments(o, "", "", strings.Repeat("a", 28000)); err == nil {
			t.Fatal("model dahil tüm argümanların boyutu sayılmadı")
		}
	} else if _, err := arguments(o, "", "", strings.Repeat("a", 28001)); err == nil {
		t.Fatal("Windows dışındaki 28000 sınırı değişti")
	}
	for _, runner := range []string{"codex", "agy"} {
		o.Runner = runner
		if _, err := arguments(o, "--gecersiz", "", "soru"); err == nil {
			t.Fatal("oturum bayrak olabiliyor")
		}
	}
}

func Test_tampon_yeniden_teslimde_mesaji_cogaltmaz(t *testing.T) {
	// NEDEN: kayıt başarılı, imleç yazımı başarısız olursa aynı bilgi tamponda çoğalmamalıdır.
	c := &conversation{}
	m := proto.Message{ID: 1, Body: "bilgi"}
	c.add(m)
	c.add(m)
	if len(c.Pending) != 1 {
		t.Fatal("yeniden teslim tamponu çoğalttı")
	}
}

func Test_cikti_bellek_siniri_pipe_tikanmadan_korunur(t *testing.T) {
	// NEDEN: kontrolsüz JSONL çıktısı belleği büyütmemeli ve kısa yazım hatası oluşturmamalıdır.
	var out cappedOutput
	p := bytes.Repeat([]byte("x"), maxRunnerOutput+100)
	n, err := out.Write(p)
	if n != len(p) || err != nil || out.Len() != maxRunnerOutput || !out.overflow {
		t.Fatal("çıktı sınırı yanlış")
	}
	if n, err = out.Write([]byte("son")); n != 3 || err != nil || out.Len() != maxRunnerOutput {
		t.Fatal("sınır sonrası pipe tıkanabilir")
	}
}

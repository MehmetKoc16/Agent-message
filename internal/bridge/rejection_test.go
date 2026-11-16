package bridge_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (f *fixture) hasDelivery(runner string) bool {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, "bridge@oda@kopru@"+runner+".json"))
	if err != nil {
		f.t.Fatal(err)
	}
	return bytes.Contains(b, []byte(`"delivery"`))
}

func Test_kalici_ret_teslimi_dusurur_kopru_devam_eder(t *testing.T) {
	// NEDEN: yeniden denemeyle düzelmeyecek ret, teslim kaydını diskte bırakıp köprüyü her
	// açılışta aynı hataya kilitlememeli; kayıt düşmeli, sonraki mesajlar işlenmelidir.
	for _, v := range []struct {
		status int
		code   string
	}{{400, "gecersiz_alici"}, {403, "yasak"}, {404, "konusma_yok"}, {413, "govde_cok_buyuk"}, {409, "istek_cakismasi"}} {
		t.Run(v.code, func(t *testing.T) {
			f := newFixture(t)
			f.code, f.postStatuses = v.code, []int{v.status}
			f.add(1, "T-1", "soru", "GOVDE_LOGA_GIRMEZ")
			f.add(2, "T-2", "soru", "sonraki")
			code, log := f.run(context.Background(), "codex")
			mustOK(t, code, log)
			if f.after() != 2 || len(f.replies) != 2 || f.replies[1].Thread != "T-2" || len(readCalls(f.dir)) != 2 {
				t.Fatalf("kalıcı retten sonra devam edilmedi: after=%d cevap=%d", f.after(), len(f.replies))
			}
			if f.hasDelivery("codex") {
				t.Fatal("zehirli teslim kaydı diskte kaldı")
			}
			if !strings.Contains(log, "işlem=dusuruldu") || !strings.Contains(log, `kod="`+v.code+`"`) ||
				!strings.Contains(log, `oda="oda"`) || !strings.Contains(log, "mesaj=1 ") {
				t.Fatalf("düşürme açıkça loglanmadı: %s", log)
			}
			if strings.Contains(log, "GOVDE_LOGA_GIRMEZ") || strings.Contains(log, "SAHTE_CEVAP_GIZLI") {
				t.Fatal("log gövde içeriyor")
			}
		})
	}
}

func Test_kayitli_zehirli_teslim_acilista_dusurulur(t *testing.T) {
	// NEDEN: önceki koşudan kalan kayıt kalıcı ret alıyorsa açılışta düşmeli; ajan yeniden çalışmamalı.
	f := newFixture(t)
	f.status = 503
	f.add(1, "T-1", "soru", "ilk")
	if code, _ := f.run(context.Background(), "codex"); code != 3 || !f.hasDelivery("codex") {
		t.Fatal("geçici hatada kayıt saklanmadı")
	}
	f.status, f.code, f.postStatuses = 0, "gecersiz_alici", []int{400}
	f.add(2, "T-2", "soru", "yeni")
	code, log := f.run(context.Background(), "codex")
	mustOK(t, code, log)
	if f.after() != 2 || len(readCalls(f.dir)) != 2 || f.hasDelivery("codex") {
		t.Fatal("kayıtlı zehirli teslim köprüyü kilitledi")
	}
}

func Test_401_teslim_kaydini_saklar_ve_durur(t *testing.T) {
	// NEDEN: iptal edilmiş/geçersiz token düzeltilebilir; hazır cevap atılmamalıdır.
	f := newFixture(t)
	f.status, f.code = 401, "yetkisiz"
	f.add(1, "T-1", "soru", "ilk")
	code, log := f.run(context.Background(), "codex")
	if code == 0 || f.after() != 0 || !f.hasDelivery("codex") || strings.Contains(log, "dusuruldu") {
		t.Fatalf("401 kaydı düşürdü: çıkış=%d", code)
	}
}

func Test_403_oda_yetkisi_yoksa_teslim_dusurulmez(t *testing.T) {
	// NEDEN: sunucu oda üyeliği kaybını da 403 "yasak" ile bildirir; köprü tümüyle
	// yetkisizken mesajları sessizce düşürmemeli, durup kaydı saklamalıdır.
	f := newFixture(t)
	f.status, f.code = 403, "yasak"
	f.getStatuses = []int{0, 403} // ilk okuma başarılı, üyelik yoklaması 403
	f.add(1, "T-1", "soru", "ilk")
	f.add(2, "T-2", "soru", "ikinci")
	code, log := f.run(context.Background(), "codex")
	if code == 0 || f.after() != 0 || !f.hasDelivery("codex") || len(f.replies) != 1 || len(readCalls(f.dir)) != 1 {
		t.Fatalf("yetkisiz köprü mesaj düşürdü: çıkış=%d after=%d log=%s", code, f.after(), log)
	}
}

func Test_uc_ardisik_sistemik_ret_kopruyu_durdurur(t *testing.T) {
	// NEDEN: her cevabı reddeden bir uyumsuzlukta (mesaja özgü olmayan kod) köprü
	// sonsuza dek ajan çalıştırıp cevap düşürmemelidir.
	f := newFixture(t)
	f.status, f.code = 400, "gecersiz_tip"
	for i := int64(1); i <= 5; i++ {
		f.add(i, "T-1", "soru", "soru")
	}
	code, log := f.run(context.Background(), "codex")
	if code == 0 || f.after() != 2 || len(readCalls(f.dir)) != 3 || !f.hasDelivery("codex") {
		t.Fatalf("sistemik ret sınırı uygulanmadı: çıkış=%d after=%d log=%s", code, f.after(), log)
	}
	// Mesaja özgü retler sayaca girmez: beşi de düşer, köprü durmaz.
	g := newFixture(t)
	g.status, g.code = 400, "gecersiz_alici"
	for i := int64(1); i <= 5; i++ {
		g.add(i, "T-1", "soru", "soru")
	}
	code, log = g.run(context.Background(), "codex")
	mustOK(t, code, log)
	if g.after() != 5 {
		t.Fatal("mesaja özgü retler köprüyü durdurdu")
	}
}

func Test_surekli_mod_gecici_gonderim_hatasini_yeniden_dener(t *testing.T) {
	// NEDEN: sürekli modda tek bir 502-504/ağ hatası köprüyü kapatmamalı; hazır cevap
	// ajan yeniden çalıştırılmadan aynı gövdeyle yeniden gönderilmelidir.
	f := newFixture(t)
	f.postStatuses = []int{503, 502}
	f.add(1, "T-1", "soru", "geçerli")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan int, 1)
	go func() { code, _ := f.execute(ctx, "codex", false); ch <- code }()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join(f.dir, "state@oda@kopru.json"))
		if bytes.Contains(b, []byte(`"after":1`)) {
			break
		}
		select {
		case code := <-ch:
			t.Fatalf("köprü geçici gönderim hatasında kapandı: çıkış=%d", code)
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	select {
	case code := <-ch:
		if code != 130 {
			t.Fatalf("çıkış=%d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("köprü iptal edilemedi")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.after() != 1 || len(f.replies) != 3 || f.replies[0] != f.replies[2] || len(readCalls(f.dir)) != 1 {
		t.Fatalf("hazır cevap yeniden denenmedi: cevap=%d", len(f.replies))
	}
}

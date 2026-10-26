package bridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/bridge"
	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

// addFrom, f.add ile aynı mesajı gönderen/alıcı değiştirerek ekler.
func (f *fixture) addFrom(id int64, from, to, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, proto.Message{ID: id, Room: "oda", From: from, To: to, Type: "soru", Thread: "T-1", Body: body, Turn: 1, MaxTurns: 20})
}

func Test_yonlu_olmayan_mesaj_varsayilan_calistirmaz_imlec_ilerler(t *testing.T) {
	// NEDEN: Codex read-only sandbox'ı okumayı kısıtlamaz; herkese açık soruyla her oda
	// üyesi köprüye dosya okutabilirdi. Elenen mesaj tekrar işlenmesin diye imleç ilerler,
	// log gönderen ve nedeni içerir ama gövdeyi içermez.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.addFrom(1, "diger", "", "ACIK_SORU_GIZLI")
		code, log := f.run(context.Background(), runner)
		mustOK(t, code, log)
		if len(readCalls(f.dir)) != 0 || len(f.replies) != 0 || f.after() != 1 {
			t.Fatal("yönlü olmayan mesaj işlendi ya da imleç ilerlemedi")
		}
		if !strings.Contains(log, `gönderen="diger"`) || !strings.Contains(log, "neden=izin_yok") || strings.Contains(log, "ACIK_SORU_GIZLI") {
			t.Fatalf("eleme logu yanlış: %s", log)
		}
	})
}

func Test_include_public_herkese_acik_mesaji_isler(t *testing.T) {
	// NEDEN: eski davranışı bilerek isteyen kurulum --include-public ile açık soruları yanıtlayabilmeli.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.addFrom(1, "diger", "", "acik soru")
		code, log := f.run(context.Background(), runner, "--include-public")
		mustOK(t, code, log)
		if len(readCalls(f.dir)) != 1 || len(f.replies) != 1 || f.replies[0].To != "diger" || f.after() != 1 {
			t.Fatal("--include-public açık soruyu işlemedi")
		}
	})
}

func Test_allow_from_disindaki_gonderen_tetiklemez(t *testing.T) {
	// NEDEN: yönlü mesaj bile izin listesinde olmayan bir oda üyesinden geliyorsa ajan çalışmamalı.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.addFrom(1, "yabanci", "kopru", "YABANCI_GOVDE")
		code, log := f.run(context.Background(), runner, "--allow-from", "diger,ali")
		mustOK(t, code, log)
		if len(readCalls(f.dir)) != 0 || len(f.replies) != 0 || f.after() != 1 {
			t.Fatal("izin listesi dışındaki gönderen ajanı tetikledi")
		}
		if !strings.Contains(log, `gönderen="yabanci"`) || !strings.Contains(log, "neden=izin_yok") || strings.Contains(log, "YABANCI_GOVDE") {
			t.Fatalf("eleme logu yanlış: %s", log)
		}
	})
}

func Test_allow_from_listesindeki_gonderen_tetikler(t *testing.T) {
	// NEDEN: izin listesi yetkili hesabı engellememeli; listedeki ikinci ad da tanınmalı.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.addFrom(1, "diger", "kopru", "izinli soru")
		code, log := f.run(context.Background(), runner, "--allow-from", "ali, diger")
		mustOK(t, code, log)
		if len(readCalls(f.dir)) != 1 || len(f.replies) != 1 || f.replies[0].To != "diger" {
			t.Fatal("izinli gönderen ajanı tetiklemedi")
		}
	})
}

func Test_gecersiz_allow_from_baslangicta_hata(t *testing.T) {
	// NEDEN: yazım hatalı izin listesi sessizce "herkes" ya da "hiç kimse" anlamına gelmemeli;
	// ağa ve çalıştırıcıya gidilmeden reddedilmeli.
	for _, bad := range []string{"Ali", "a,,b", "-ali", "ali veli", "ali/veli", ","} {
		f := newFixture(t)
		f.add(1, "T-1", "soru", "ilk")
		code, log := f.run(context.Background(), "codex", "--allow-from", bad)
		if code != 1 || f.gets != 0 || len(readCalls(f.dir)) != 0 || !strings.Contains(log, "--allow-from") {
			t.Fatalf("geçersiz --allow-from kabul edildi: %q kod=%d log=%s", bad, code, log)
		}
	}
}

func Test_ayni_dizindeki_baska_config_tokeni_kopru_cevabinda_engellenir(t *testing.T) {
	// NEDEN: aynı makinedeki başka relay ayarının token'ı bilinen önek taşımaz; model onu
	// okuyup yazarsa desen taraması kaçırır. Bozuk/kısa JSON dosyaları taramayı bozmamalı.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		const other = "other-fake-credential-abcdef0123"
		files := map[string]string{
			"a-bozuk.json": "{bozuk",
			"diger.json":   `{"url":"https://relay.example.com","room":"oda","agent":"insan","token":"` + other + `"}`,
			"kisa.json":    `{"token":"kisa"}`,
			"sayi.json":    `{"token":12345678901234567890}`,
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		f.add(1, "T-1", "soru", "CEVAP_VARYANTI: "+other)
		f.add(2, "T-2", "soru", "CEVAP_VARYANTI: kisa")
		code, log := f.run(context.Background(), runner)
		mustOK(t, code, log)
		if len(f.replies) != 2 || f.replies[0].Body != "köprü: cevap sır taraması nedeniyle gönderilmedi" || strings.Contains(f.replies[0].Body, other) {
			t.Fatal("başka ayarın token'ı engellenmedi")
		}
		if f.replies[1].Body != "kisa" {
			t.Fatal("16 karakterden kısa değer engellenmemeli")
		}
		if !strings.Contains(log, `sır_deseni="relay kimlik bilgisi"`) || strings.Contains(log, other) {
			t.Fatal("sır logu yanlış ya da değer sızdı")
		}
	})
}

func Test_eski_adli_durum_dosyalari_tasinir_imlec_korunur(t *testing.T) {
	// NEDEN: '@' ayırıcısına geçiş mevcut kurulumun imlecini ve runner oturumunu
	// sıfırlamamalı; aksi halde eski mesajlar yeniden çalıştırılır.
	f := newFixture(t)
	const session = "01234567-89ab-cdef-0123-456789abcdef"
	legacy := map[string]string{
		"state-oda-kopru.json":        `{"after":5}`,
		"bridge-oda-kopru-codex.json": `{"threads":{"T-1":{"session":"` + session + `"}}}`,
	}
	for name, body := range legacy {
		if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.add(5, "T-1", "soru", "ESKI_ISLENMIS")
	f.add(6, "T-1", "soru", "yeni")
	code, log := f.run(context.Background(), "codex")
	mustOK(t, code, log)
	calls := readCalls(f.dir)
	if len(calls) != 1 || strings.Contains(calls[0].Stdin, "ESKI_ISLENMIS") || f.after() != 6 {
		t.Fatal("imleç taşınmadı: eski mesaj yeniden işlendi")
	}
	if len(calls[0].Args) < 3 || calls[0].Args[1] != "resume" || calls[0].Args[2] != session {
		t.Fatal("köprü oturumu taşınmadı")
	}
	for name := range legacy {
		if _, err := os.Stat(filepath.Join(f.dir, name)); !os.IsNotExist(err) {
			t.Fatalf("eski dosya taşınmadı: %s", name)
		}
	}
}

func Test_baska_odaya_ait_bekleyen_teslim_gonderilmez(t *testing.T) {
	// NEDEN: eski çakışan dosya adları başka oda/ajanın bekleyen cevabını getirebilir;
	// yeniden başlatma onu bu odaya göndermemeli, kaydı düşürüp loglamalı. Alanları
	// olmayan eski kayıtta oda gelen mesajın Room alanından denetlenir.
	cases := map[string]map[string]any{
		"kayitli_oda":  {"room": "baska", "agent": "kopru"},
		"kayitli_ajan": {"room": "oda", "agent": "baska-ajan"},
		"eski_kayit":   {},
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			o, d, cur := f.direct("codex")
			logs := new(bytes.Buffer)
			d.Stderr = logs
			srcRoom := "oda"
			if name == "eski_kayit" {
				srcRoom = "baska"
			}
			delivery := map[string]any{
				"source": proto.Message{ID: 5, Room: srcRoom, From: "diger", To: "kopru", Type: "soru", Thread: "T-1", Turn: 1, MaxTurns: 20},
				"reply":  bridge.Outgoing{To: "diger", Type: "cevap", Thread: "T-1", ReplyTo: 5, Body: "BASKA_ODA_CEVABI", RequestID: "bridge-codex-5"},
			}
			for k, v := range fields {
				delivery[k] = v
			}
			b, _ := json.Marshal(map[string]any{"threads": map[string]any{"T-1": map[string]any{"delivery": delivery}}})
			if err := os.WriteFile(o.StatePath, b, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := bridge.Run(context.Background(), o, d); err != nil {
				t.Fatal(err)
			}
			if len(f.replies) != 0 || cur.after != 0 {
				t.Fatal("başka odaya ait teslim gönderildi")
			}
			if !strings.Contains(logs.String(), "işlem=dusuruldu") || strings.Contains(logs.String(), "BASKA_ODA_CEVABI") {
				t.Fatalf("düşürme logu yanlış: %s", logs.String())
			}
			state, _ := os.ReadFile(o.StatePath)
			if bytes.Contains(state, []byte("BASKA_ODA_CEVABI")) {
				t.Fatal("düşürülen teslim durum dosyasında kaldı")
			}
		})
	}
}

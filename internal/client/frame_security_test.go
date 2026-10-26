package client

import (
	"strings"
	"testing"
	"unicode"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

func TestCerceveTerminalKomutlariniMetinOlarakGosterir(t *testing.T) {
	// NEDEN: HTML kaçışı terminalin ESC/OSC/CSI ve satır başı komutlarını
	// durdurmaz. Gelen mesaj panoya yazamamalı veya önceki çıktıyı silememeli.
	evil := "\x1b[2J\x1b]52;c;ZXhhbXBsZQ==\a\r\b\u009b31m"
	got := Render(proto.Message{ID: 1, From: "ajan" + evil, Body: "Türkçe\n\t" + evil})
	for _, r := range got {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			t.Fatalf("terminal kontrol karakteri çıktı: %U", r)
		}
	}
	if !strings.Contains(got, "Türkçe\n\t") || !strings.Contains(got, `\u001b`) || !strings.Contains(got, "ZXhhbXBsZQ==") {
		t.Fatalf("içerik okunabilir kaçışla korunmalı: %q", got)
	}
}

func TestCerceveUnicodeBicimKarakterleriniKacislar(t *testing.T) {
	// NEDEN: bidi yönlendirme (U+202E), sıfır genişlik (U+200B) ve satır ayırıcı (U+2028)
	// Cc değildir; ekranda metni ters gösterip ya da sahte satır açıp insanı/ajanı yanıltabilir.
	got := Render(proto.Message{ID: 1, From: "ali\u202e", Body: "a\u202eb\u200bc\u2028d\u2029e\ufeff\u2066f"})
	for _, want := range []string{`a\u202eb\u200bc\u2028d\u2029e\ufeff\u2066f`, `from="ali\u202e"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("kaçış eksik %q: %q", want, got)
		}
	}
	for _, r := range got {
		if unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			t.Fatalf("biçim karakteri çıktıda kaldı: %U", r)
		}
	}
}

func TestSirTaramasiArayaKonmusSifirGenislikKarakteriniYakalar(t *testing.T) {
	// NEDEN: araya U+200B konmuş token ya da desen düz Contains/regex taramasından kaçar;
	// tarama Cf karakterleri çıkarılmış kopyada yapılmalıdır.
	r := &runner{cfg: &Config{Token: "client-test-credential-0123456789"}}
	if name, found := r.scanSecrets("bak: client-test-\u200bcredential-0123\u2060456789"); !found || name != "relay kimlik bilgisi" {
		t.Fatal("araya biçim karakteri konmuş token yakalanmadı")
	}
	if _, found := ScanSecrets("g" + "hp" + "_\u200b" + strings.Repeat("a", 30)); !found {
		t.Fatal("araya biçim karakteri konmuş desen yakalanmadı")
	}
}

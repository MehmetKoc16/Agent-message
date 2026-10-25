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

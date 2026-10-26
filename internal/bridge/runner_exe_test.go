package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test_agy_toplu_is_dosyasi_calistiriciyi_reddeder(t *testing.T) {
	// NEDEN: agy istemi -p argümanıyla alır; .cmd/.bat ise cmd.exe mesaj metnini komut sayabilir.
	dir := t.TempDir()
	options := func(runner, name string) Options {
		exe := filepath.Join(dir, name)
		if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return Options{Runner: runner, Exe: exe, Workdir: dir, MaxRuns: 1, Timeout: time.Second}
	}
	for _, name := range []string{"agy.cmd", "agy.BAT"} {
		o := options("agy", name)
		if err := o.prepare(); err == nil || !strings.Contains(err.Error(), ".cmd/.bat") {
			t.Fatalf("%s kabul edildi: %v", name, err)
		}
	}
	o := options("agy", "agy.exe")
	if err := o.prepare(); err != nil {
		t.Fatalf("agy.exe reddedildi: %v", err)
	}
	// Codex istemi stdin'den aldığı için npm'in codex.cmd kabuğu kullanılabilir kalır.
	o = options("codex", "codex.cmd")
	if err := o.prepare(); err != nil {
		t.Fatalf("codex.cmd reddedildi: %v", err)
	}
}

//go:build unix

package bridge_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_zaman_asimi_torun_sureci_de_oldurur(t *testing.T) {
	// NEDEN: ajanın başlattığı alt süreçler zaman aşımından sonra çalışmayı sürdürmemelidir.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.add(1, "T-1", "soru", "TORUN_ISTE YAVAS_ISTE")
		start := time.Now()
		code, log := f.run(context.Background(), runner, "--runner-timeout", "1")
		mustOK(t, code, log)
		// Torunun gerçekten başladığını doğrula; yoksa test boşuna geçer.
		if _, err := os.Stat(filepath.Join(f.dir, "grandchild-started")); err != nil {
			t.Fatal("torun süreç başlamadı")
		}
		// Torun yaşasaydı 4. saniyede dosyayı yazardı; o andan sonra bak.
		time.Sleep(time.Until(start.Add(6 * time.Second)))
		if _, err := os.Stat(filepath.Join(f.dir, "grandchild-finished")); !os.IsNotExist(err) {
			t.Fatal("torun süreç zaman aşımından sonra çalışmaya devam etti")
		}
	})
}

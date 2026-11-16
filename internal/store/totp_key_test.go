package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// NEDEN: anahtar dosyası önce boş oluşturulup sonra yazılırsa eşzamanlı açılış (ya da
// arada çökme) 0 baytlık dosya görür ve "anahtar geçersiz" ile biter. Dosya yalnız
// tam yazılmış hâliyle görünmeli; taze veritabanında eşzamanlı açılış da başarmalı.
func TestTOTPAnahtariAtomikOlusur(t *testing.T) {
	for i := 0; i < 40; i++ {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("r%d.db", i))
		var wg sync.WaitGroup
		errs := make([]error, 8)
		for j := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s, err := Open(path, 20)
				if err == nil {
					err = s.Close()
				}
				errs[j] = err
			}()
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatalf("tur %d: %v", i, err)
			}
		}
		key, err := os.ReadFile(path + ".totp.key")
		if err != nil || len(key) != 32 {
			t.Fatalf("anahtar: %d bayt %v", len(key), err)
		}
		left, _ := filepath.Glob(path + ".totp.key.*")
		if len(left) != 0 {
			t.Fatalf("geçici dosya kaldı: %v", left)
		}
	}
}

// NEDEN: var olan anahtar asla ezilmez; kısa/boş dosya sessizce yenilenmez, hata
// kurtarma adımını söyler. Şifreli sır varsa silme önerilmez.
func TestTOTPAnahtariEzilmez(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relay.db")
	keyPath := path + ".totp.key"
	mevcut := []byte(strings.Repeat("k", 32))
	if err := os.WriteFile(keyPath, mevcut, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := createTOTPKey(keyPath); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(keyPath); err != nil || string(got) != string(mevcut) {
		t.Fatalf("mevcut anahtar ezildi: %v", err)
	}
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddAgent(ctx, "y", "insan", HashToken("test-y"), nil); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, sirVar := range []bool{false, true} {
		if sirVar {
			if err := os.WriteFile(keyPath, mevcut, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(path, 20)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.ChangeAccount(ctx, "ssh", "yonetici-yap", "y", "", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", false); err != nil {
				t.Fatal(err)
			}
			s.Close()
		}
		for _, kisa := range [][]byte{nil, []byte("kisa")} {
			if err := os.WriteFile(keyPath, kisa, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(path, 20)
			if err == nil {
				s.Close()
				t.Fatal("kısa anahtar kabul edildi")
			}
			if strings.Contains(err.Error(), "silip") == sirVar {
				t.Fatalf("sır var=%v için yanlış yönlendirme: %v", sirVar, err)
			}
			if got, _ := os.ReadFile(keyPath); string(got) != string(kisa) {
				t.Fatal("kısa anahtar dosyası sessizce değiştirildi")
			}
		}
	}
}

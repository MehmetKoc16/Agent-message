package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Kilit bekleme süresi, bayat kilit eşiğinden uzundur: çöken bir sürecin artığı beklerken
// kilit alınabilsin diye.
const (
	lockWait  = 8 * time.Second
	staleLock = 5 * time.Second
)

// windows: yalnız Windows'ta başka süreçle dosya paylaşım çakışması geçici hata verir.
var windows = runtime.GOOS == "windows"

// cursor okuma imlecidir: son görülen mesaj id'si. Yalnız ileri gider, atomik yazılır,
// bozuksa sessizce sıfırlanmaz (sıfırlamak eski mesajları yeniden getirirdi).
type cursor struct{ path string }

type cursorFile struct {
	After *int64 `json:"after"`
}

// Read imleci okur. Dosya yoksa 0; bozuksa hata verir.
func (c cursor) Read() (int64, error) {
	b, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("imleç dosyası okunamadı: %s", c.path)
	}
	var f cursorFile
	if err := json.Unmarshal(b, &f); err != nil || f.After == nil || *f.After < 0 {
		return 0, fmt.Errorf("imleç dosyası bozuk: %s (düzeltin ya da silin; silerseniz eski mesajlar yeniden gelir)", c.path)
	}
	return *f.After, nil
}

// Advance imleci n'e ilerletir; n mevcut değerden büyük değilse hiçbir şey yapmaz.
// Diğer relay süreçleriyle yarışmamak için küçük bir kilit dosyası kullanır.
func (c cursor) Advance(n int64) error {
	unlock, err := c.lock()
	if err != nil {
		return err
	}
	defer unlock()
	cur, err := c.Read()
	if err != nil {
		return err
	}
	if n <= cur {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), filepath.Base(c.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("imleç dosyası yazılamadı: %s", c.path)
	}
	tmpName := tmp.Name()
	data, _ := json.Marshal(cursorFile{After: &n})
	_, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = renameRetry(tmpName, c.path) // atomik değiştirme
	}
	if werr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("imleç dosyası yazılamadı: %s", c.path)
	}
	return nil
}

func (c cursor) lock() (func(), error) {
	lockPath := c.path + ".lock"
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(lockPath) }, nil
		}
		// Windows'ta başka süreç kilidi silerken oluşturma "erişim engellendi" verebilir; geçicidir.
		if !errors.Is(err, os.ErrExist) && !(windows && errors.Is(err, os.ErrPermission)) {
			return nil, fmt.Errorf("imleç kilidi alınamadı: %s", lockPath)
		}
		if st, serr := os.Stat(lockPath); serr == nil && time.Since(st.ModTime()) > staleLock {
			os.Remove(lockPath) // çökmüş sürecin artığı
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("imleç kilidi alınamadı: %s", lockPath)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// renameRetry, Windows'ta hedef dosya başka süreç tarafından o anda okunuyorsa oluşan geçici
// hatalara karşı kısa süre yeniden dener.
func renameRetry(from, to string) error {
	var err error
	for i := 0; i < 50; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		if !windows {
			return err // Linux'ta kalıcı hata: boşuna beklenmez
		}
		time.Sleep(20 * time.Millisecond)
	}
	return err
}

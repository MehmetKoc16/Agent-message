package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const encryptedTOTP = "enc:v1:"

// The key is deliberately outside SQLite, so a database-only backup does not
// contain both authentication factors. Back it up separately with restricted ACLs.
func (s *Store) initTOTP(path string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT name,totp_secret FROM agents WHERE COALESCE(totp_secret,'') <> ''`)
	if err != nil {
		return err
	}
	type entry struct{ name, secret string }
	var entries []entry
	hasEncrypted := false
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.name, &e.secret); err != nil {
			rows.Close()
			return err
		}
		hasEncrypted = hasEncrypted || strings.HasPrefix(e.secret, encryptedTOTP)
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	keyPath := path + ".totp.key"
	f, err := os.Open(keyPath)
	if errors.Is(err, os.ErrNotExist) && !hasEncrypted {
		if err := createTOTPKey(keyPath); err != nil {
			return errors.New("TOTP anahtarı yazılamadı")
		}
		f, err = os.Open(keyPath)
	}
	if err != nil {
		return errors.New("TOTP anahtarı okunamadı; ayrı anahtar yedeğini geri yükleyin")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return errors.New("TOTP anahtarı yalnız sahibine açık normal bir dosya olmalı (0600)")
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 {
		if !hasEncrypted {
			// Bilerek yeniden üretilmez: var olan dosya asla ezilmez, karar yöneticinindir.
			return errors.New("TOTP anahtarı geçersiz (32 bayt olmalı); şifreli sır yok: anahtar dosyasını silip yeniden başlatın")
		}
		return errors.New("TOTP anahtarı geçersiz; ayrı anahtar yedeğini geri yükleyin")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	s.totpCipher, err = cipher.NewGCM(block)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.secret, encryptedTOTP) {
			if _, err := s.openTOTP(e.name, e.secret); err != nil {
				return err
			}
			continue
		}
		sealed, err := s.sealTOTP(e.name, e.secret)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE agents SET totp_secret=? WHERE name=?`, sealed, e.name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// createTOTPKey yeni anahtarı aynı klasörde 0600 geçici dosyaya yazıp fsync eder ve
// hard link ile yerleştirir: dosya ya hiç yoktur ya da tamdır (arada çökme boş anahtar
// bırakmaz). Link mevcut dosyayı ezmez; anahtar zaten varsa ona dokunulmaz.
func createTOTPKey(keyPath string) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	dir := filepath.Dir(keyPath)
	tmp, err := os.CreateTemp(dir, filepath.Base(keyPath)+".*") // 0600 ile oluşur
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(key)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Link(tmp.Name(), keyPath); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	// Klasör girdisini de diske yaz; desteklenmeyen sistemlerde (Windows) hata yok sayılır.
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

func (s *Store) sealTOTP(name, secret string) (string, error) {
	nonce := make([]byte, s.totpCipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := s.totpCipher.Seal(nonce, nonce, []byte(secret), []byte(name))
	return encryptedTOTP + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *Store) openTOTP(name, secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	if !strings.HasPrefix(secret, encryptedTOTP) {
		return "", errors.New("şifrelenmemiş TOTP sırrı reddedildi; güncel sunucuyla yeniden açın")
	}
	data, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(secret, encryptedTOTP))
	n := s.totpCipher.NonceSize()
	if err != nil || len(data) < n {
		return "", errors.New("TOTP sırrı bozuk")
	}
	plain, err := s.totpCipher.Open(nil, data[:n], data[n:], []byte(name))
	if err != nil {
		return "", errors.New("TOTP sırrı çözülemedi; doğru anahtar gerekli")
	}
	return string(plain), nil
}

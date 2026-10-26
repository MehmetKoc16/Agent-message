package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
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
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		f, err = os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, err = f.Write(key)
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return errors.New("TOTP anahtarı yazılamadı")
			}
		}
		if err == nil || errors.Is(err, os.ErrExist) {
			f, err = os.Open(keyPath)
		}
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
		return errors.New("TOTP anahtarı geçersiz")
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

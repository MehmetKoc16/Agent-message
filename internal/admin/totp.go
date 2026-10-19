package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"time"
)

// NewTOTPSecret yalnız SSH kaydında bir kez gösterilecek 160 bit sır üretir.
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// TOTPCode RFC 6238 SHA1, 30 saniye ve altı hane kullanır.
func TOTPCode(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) == 0 {
		return "", fmt.Errorf("geçersiz TOTP sırrı")
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(b[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 15
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000), nil
}

// MatchTOTP pencere içindeki en yeni adımı döndürür; tekrar kontrolü DB'dedir.
func MatchTOTP(secret, code string, now time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	current := now.Unix() / 30
	found := int64(-1)
	for d := int64(-1); d <= 1; d++ {
		step := current + d
		if step < 0 {
			continue
		}
		want, err := TOTPCode(secret, step)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			found = step
		}
	}
	return found, found >= 0
}

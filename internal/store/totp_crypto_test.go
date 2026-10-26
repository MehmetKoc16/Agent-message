package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTOTPEncryptionMigrationAndKeyRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "relay.db")
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if err := s.AddAgent(ctx, "admin", "insan", HashToken("test-token"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetYonetici(ctx, "admin", secret); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := s.db.QueryRow(`SELECT totp_secret FROM agents WHERE name='admin'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, encryptedTOTP) || strings.Contains(stored, secret) {
		t.Fatal("plaintext TOTP persisted")
	}
	// Simulate a legacy database; opening it encrypts without resetting replay state.
	if _, err := s.db.Exec(`UPDATE agents SET totp_secret=?,totp_last_step=123 WHERE name='admin'`, secret); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	got, step, err := s.TOTPState(ctx, "admin")
	if err != nil || got != secret || step != 123 {
		t.Fatalf("migration/reopen failed: step=%d err=%v", step, err)
	}
	if _, err := s.Authenticate(ctx, "test-token"); err != nil {
		t.Fatal("token lost", err)
	}
	s.Close()
	keyPath := path + ".totp.key"
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 || bytes.Contains(key, []byte(secret)) {
		t.Fatal("invalid external key")
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path, 20); err == nil {
		opened.Close()
		t.Fatal("missing key accepted")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("missing key was replaced")
	}
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{1}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path, 20); err == nil {
		opened.Close()
		t.Fatal("wrong key accepted")
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, _, err = s.TOTPState(ctx, "admin")
	if err != nil || got != secret {
		t.Fatal("key recovery failed", err)
	}
	if _, err := s.openTOTP("another-account", stored); err == nil {
		t.Fatal("ciphertext not bound to account")
	}
}

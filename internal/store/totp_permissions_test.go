//go:build !windows

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTOTPKeyRejectsGroupReadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.db")
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.Chmod(path+".totp.key", 0o640); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path, 20); err == nil {
		opened.Close()
		t.Fatal("group-readable key accepted")
	}
}

//go:build !windows

package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRejectsGroupOrOtherAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")
	const body = `{"url":"https://relay.example.com","room":"r1","agent":"ali","token":"test-token"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0o600, 0o400, 0o640, 0o604, 0o620, 0o601} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		_, err := LoadConfig(path)
		if (err == nil) != (mode&0o077 == 0) {
			t.Fatalf("mode %o: %v", mode, err)
		}
		if err != nil && strings.Contains(err.Error(), "test-token") {
			t.Fatal("token leaked")
		}
	}
}

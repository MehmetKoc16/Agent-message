package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKnownRelayCredentialCannotBeSentOrAnnounced(t *testing.T) {
	for _, cmd := range []string{"send", "announce"} {
		t.Run(cmd, func(t *testing.T) {
			e := newCEnv(t)
			args := []string{cmd}
			if cmd == "send" {
				args = append(args, "--type", "bilgi")
			} else {
				args = append(args, "--room", "r1")
			}
			args = append(args, "credential: "+e.toks["ali"])
			code, out, stderr := e.run("ali", "", args...)
			if code != ExitSecret {
				t.Fatalf("credential was not blocked: exit=%d", code)
			}
			if strings.Contains(out+stderr, e.toks["ali"]) {
				t.Fatal("credential leaked to console")
			}
			messages, err := e.st.RoomLog(context.Background(), "r1", 0, 100)
			if err != nil || len(messages) != 0 {
				t.Fatal("credential reached relay")
			}
		})
	}
}

func TestServerErrorCannotEmitTerminalControlsOrCredential(t *testing.T) {
	for _, attack := range []string{"\x1b]52;c;payload\x07\rforged", strings.Repeat("A", 2000), "client-test-credential-0123456789"} {
		t.Run("untrusted-error", func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": attack})
			}))
			defer ts.Close()
			code, _, stderr := runWith(context.Background(), fakeConfig(t, ts.URL), "send", "--type", "bilgi", "hello")
			if code != ExitError || strings.ContainsAny(stderr, "\x1b\x07\r") || strings.Contains(stderr, attack) {
				t.Fatal("untrusted server error reached console")
			}
		})
	}
}

package bridge_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/bridge"
)

func TestConfiguredCredentialNeverLeavesRunnerResponse(t *testing.T) {
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		const credential = "bridge-test-credential-0123456789"
		f.add(1, "T-1", "soru", "CEVAP_VARYANTI: "+credential)
		code, log := f.run(context.Background(), runner)
		mustOK(t, code, log)
		if len(f.replies) != 1 || strings.Contains(f.replies[0].Body, credential) || !strings.Contains(f.replies[0].Body, "sır taraması") {
			t.Fatal("configured credential was not blocked")
		}
		if strings.Contains(log, credential) {
			t.Fatal("credential leaked to log")
		}
		state, err := os.ReadFile(filepath.Join(f.dir, "bridge@oda@kopru@"+runner+".json"))
		if err != nil || strings.Contains(string(state), credential) {
			t.Fatal("credential saved in outgoing state")
		}
	})
}

func TestDurableReplyIsRescannedAfterRestart(t *testing.T) {
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.add(1, "T-1", "soru", "normal question")
		o, d, cur := f.direct(runner)
		broken := &failingCursor{memoryCursor: cur, fail: true}
		d.Cursor = broken
		if err := bridge.Run(context.Background(), o, d); err == nil {
			t.Fatal("expected interrupted delivery")
		}
		sent := len(f.replies)
		d.Scan = func(text string) (string, bool) {
			return "newly blocked value", strings.Contains(text, "SAHTE_CEVAP_GIZLI")
		}
		if err := bridge.Run(context.Background(), o, d); err == nil {
			t.Fatal("cached reply bypassed new scan")
		}
		if len(f.replies) != sent || cur.after != 0 || len(readCalls(f.dir)) != 1 {
			t.Fatal("unsafe replay made progress")
		}
	})
}

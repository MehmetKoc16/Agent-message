package bridge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/bridge"
)

type failingCursor struct {
	*memoryCursor
	fail bool
}

func (c *failingCursor) Advance(id int64) error {
	if c.fail {
		c.fail = false
		return errors.New("imleç yazımı kesildi")
	}
	return c.memoryCursor.Advance(id)
}

func TestGonderimVeImlecArasindaKesintiHazirCevabiKorur(t *testing.T) {
	// NEDEN: sunucu kabulünden sonra imleç yazılamazsa model tekrar çağrılmamalı;
	// yeniden teslim aynı request_id ve aynı gövdeyi taşımalıdır.
	both(t, func(t *testing.T, runner string) {
		f := newFixture(t)
		f.add(1, "T-1", "soru", "tek model çağrısı")
		o, d, cur := f.direct(runner)
		broken := &failingCursor{memoryCursor: cur, fail: true}
		d.Cursor = broken
		send := d.Send
		var requests []bridge.Outgoing
		d.Send = func(ctx context.Context, in bridge.Outgoing) error {
			requests = append(requests, in)
			return send(ctx, in)
		}
		if err := bridge.Run(context.Background(), o, d); err == nil {
			t.Fatal("imleç hatası görünmedi")
		}
		if cur.after != 0 || len(readCalls(f.dir)) != 1 {
			t.Fatal("ilk koşu tutarsız")
		}
		if err := bridge.Run(context.Background(), o, d); err != nil {
			t.Fatal(err)
		}
		if cur.after != 1 || len(readCalls(f.dir)) != 1 || len(requests) != 2 {
			t.Fatal("hazır cevap tekrar kullanılamadı")
		}
		if requests[0] != requests[1] || requests[0].RequestID == "" {
			t.Fatal("tekrar kimliği veya cevap değişti")
		}
	})
}

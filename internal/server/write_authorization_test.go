package server

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

type beforeRead struct {
	io.Reader
	once func()
}

func (b *beforeRead) Read(p []byte) (int, error) {
	if b.once != nil {
		fn := b.once
		b.once = nil
		fn()
	}
	return b.Reader.Read(p)
}

func TestGovdeBeklerkenIptalEdilenGonderimReddedilir(t *testing.T) {
	// NEDEN: başlık doğrulaması geçmiş olsa da gövde beklerken iptal edilen
	// hesap/oda yetkisi yeni mesaj veya eski isteğin cevabını alamamalı.
	for _, revoke := range []bool{false, true} {
		for _, replay := range []bool{false, true} {
			t.Run(fmt.Sprintf("revoke=%t/replay=%t", revoke, replay), func(t *testing.T) {
				e := izleEnv(t)
				ctx := context.Background()
				in := store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "bekleyen", RequestID: "held-request"}
				if replay {
					if _, err := e.st.Post(ctx, in); err != nil {
						t.Fatal(err)
					}
				}
				mutated := false
				body := &beforeRead{Reader: strings.NewReader(`{"type":"bilgi","body":"bekleyen","request_id":"held-request"}`), once: func() {
					mutated = true
					var err error
					if revoke {
						err = e.st.RevokeAgent(ctx, "ali")
					} else {
						err = e.st.RemoveRoom(ctx, "ali", "r1")
					}
					if err != nil {
						t.Fatal(err)
					}
				}}
				r := httptest.NewRequest("POST", "/v1/rooms/r1/messages", body)
				r.Header.Set("Authorization", "Bearer "+e.tokens["ali"])
				w := httptest.NewRecorder()
				e.srv.Handler().ServeHTTP(w, r)
				want := 403
				if revoke {
					want = 401
				}
				if !mutated || w.Code != want {
					t.Fatalf("iptal sonrası gönderim: %d %s, beklenen %d", w.Code, w.Body.String(), want)
				}
				msgs, err := e.st.RoomLog(ctx, "r1", 0, 100)
				count := 0
				if replay {
					count = 1
				}
				if err != nil || len(msgs) != count {
					t.Fatalf("iptal sonrası mesaj yazıldı: %v %v", msgs, err)
				}
			})
		}
	}
}

func TestIlkDenetimdenSonrakiIptalKonusmayiKapatamaz(t *testing.T) {
	// NEDEN: close da üyeliği/kimliği değiştiren transaction'dan sonra
	// eski kimlik bilgileriyle yazamamalı. Touch ilk oda denetiminden sonradır.
	for _, revoke := range []bool{false, true} {
		t.Run(fmt.Sprintf("revoke=%t", revoke), func(t *testing.T) {
			e := izleEnv(t)
			ctx := context.Background()
			m, err := e.st.Post(ctx, store.PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: "açık"})
			if err != nil {
				t.Fatal(err)
			}
			mutate := true
			e.st.Now = func() time.Time {
				if mutate {
					mutate = false
					var err error
					if revoke {
						err = e.st.RevokeAgent(ctx, "ali")
					} else {
						err = e.st.RemoveRoom(ctx, "ali", "r1")
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				return time.Now()
			}
			r := httptest.NewRequest("POST", "/v1/rooms/r1/threads/"+m.Thread+"/close", nil)
			r.Header.Set("Authorization", "Bearer "+e.tokens["ali"])
			w := httptest.NewRecorder()
			e.srv.Handler().ServeHTTP(w, r)
			want := 403
			if revoke {
				want = 401
			}
			if mutate || w.Code != want {
				t.Fatalf("iptal sonrası close: %d %s, beklenen %d", w.Code, w.Body.String(), want)
			}
			thread, err := e.st.GetThread(ctx, "r1", m.Thread)
			if err != nil || thread.Status != "acik" {
				t.Fatalf("iptal edilen kullanıcı kapattı: %+v %v", thread, err)
			}
		})
	}
}

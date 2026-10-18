package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func TestMeKimlik(t *testing.T) {
	// NEDEN: oda gerektirmeyen uç yalnız çağıranın adını, türünü ve sıralı odalarını açmalı.
	e := izleEnv(t)
	ctx := context.Background()
	if err := e.st.AddAgent(ctx, "odasiz", "insan", store.HashToken("test-me-odasiz"), nil); err != nil {
		t.Fatal(err)
	}
	if err := e.st.AddAgent(ctx, "sirali", "ajan", store.HashToken("test-me-sirali"), []string{"z", "a"}); err != nil {
		t.Fatal(err)
	}
	h := e.srv.Handler()
	for _, tc := range []struct{ token, want string }{
		{e.tokens["mehmet"], `{"name":"mehmet","kind":"insan","rooms":["r1"]}`},
		{e.tokens["ali"], `{"name":"ali","kind":"ajan","rooms":["r1","r2"]}`},
		{"test-me-odasiz", `{"name":"odasiz","kind":"insan","rooms":[]}`},
		{"test-me-sirali", `{"name":"sirali","kind":"ajan","rooms":["a","z"]}`},
	} {
		w := izleRequest(h, "GET", "/v1/me", tc.token)
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != tc.want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	for _, token := range []string{"", "test-me-yanlis"} {
		w := izleRequest(h, "GET", "/v1/me", token)
		if w.Code != 401 || strings.TrimSpace(w.Body.String()) != `{"error":"yetkisiz"}` {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	e.srv.unauth = newUnauthLimiter(1, 10)
	calls := 0
	e.srv.authenticate = func(context.Context, string) (store.Agent, error) {
		calls++
		return store.Agent{}, store.ErrUnauthorized
	}
	for _, want := range []int{401, 429} {
		w := izleRequest(h, "GET", "/v1/me", "test-me-yanlis")
		if w.Code != want {
			t.Fatalf("%d != %d", w.Code, want)
		}
	}
	if calls != 1 {
		t.Fatalf("doğrulama sayısı=%d", calls)
	}
}

func TestMeEszamanliSinir(t *testing.T) {
	// NEDEN: oda gerektirmeyen uç da bağlantı kotasını aşamamalı ve aldığı yeri bırakmalı.
	e := izleEnv(t)
	e.srv.conns = newConnLimiter(1)
	release, ok := e.srv.conns.Acquire("mehmet")
	if !ok {
		t.Fatal("yer ayrılamadı")
	}
	h := e.srv.Handler()
	w := izleRequest(h, "GET", "/v1/me", e.tokens["mehmet"])
	if w.Code != 429 || !strings.Contains(w.Body.String(), "cok_fazla_baglanti") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	release()
	for i := 0; i < 2; i++ {
		if w := izleRequest(h, "GET", "/v1/me", e.tokens["mehmet"]); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
}

func TestIzleInsanMesajGonderir(t *testing.T) {
	// NEDEN: insanın yeni konusu ajan Fetch'ine ulaşmalı; tarafı olmadığı konuşmaya yazma hakkı korunmalı.
	e := izleEnv(t)
	h := e.srv.Handler()
	post := func(body string) store.Message {
		t.Helper()
		r := httptest.NewRequest("POST", "/v1/rooms/r1/messages", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+e.tokens["mehmet"])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var m store.Message
		if w.Code != 201 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		if m.From != "mehmet" {
			t.Fatal("insan kimliği kayboldu")
		}
		return m
	}
	first := post(`{"type":"soru","body":"İnsan sorusu"}`)
	if first.Thread == "" {
		t.Fatal("konuşma açılmadı")
	}
	private, err := e.st.Post(context.Background(), store.PostInput{Room: "r1", From: "ali", To: "veli", Type: "soru", Body: "Özel konu"})
	if err != nil {
		t.Fatal(err)
	}
	second := post(`{"to":"ali","type":"bilgi","thread":"` + private.Thread + `","body":"İnsan katkısı"}`)
	if second.Thread != private.Thread {
		t.Fatal("yanlış konuşma")
	}
	w := izleRequest(h, "GET", "/v1/rooms/r1/messages", e.tokens["ali"])
	var out struct {
		Messages []store.Message `json:"messages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 {
		t.Fatalf("%d %v", w.Code, err)
	}
	found := map[int64]bool{}
	for _, m := range out.Messages {
		found[m.ID] = true
	}
	if !found[first.ID] || !found[second.ID] {
		t.Fatal("insan mesajları ajan Fetch'ine ulaşmadı")
	}
}

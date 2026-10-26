package server

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// NEDEN: üçüncü bir ajan başkalarının konuşmasına yazıp tur kotasını tüketemez ya da `kapat`
// ile close uç noktasındaki yetki denetimini atlatamaz. Taraflar ve insan yazabilir.
// (Herkese açık mesajla açılan konuşmayı odadaki herkes görür, dolayısıyla taraftır.)
func TestUcuncuAjanBaskasininKonusmasinaYazamaz(t *testing.T) {
	e := newEnv(t, nil)
	_, m := e.send("r1", "ali", map[string]any{"to": "veli"})
	th := m["thread"].(string)
	if st, r := e.send("r1", "can", map[string]any{"thread": th}); st != 403 || r["error"] != "yasak" {
		t.Fatalf("üçüncü ajan normal mesaj: %d %v", st, r)
	}
	if st, _ := e.send("r1", "can", map[string]any{"thread": th, "type": "kapat"}); st != 403 {
		t.Fatalf("üçüncü ajan kapat: %d", st)
	}
	_, info := e.call("GET", "/v1/rooms/r1/threads/"+th, "ali", nil)
	if info["status"] != "acik" || num(info["turns"]) != 1 {
		t.Fatalf("konuşma etkilenmemeli: %v", info)
	}
	if st, _ := e.send("r1", "veli", map[string]any{"thread": th}); st != 201 {
		t.Fatalf("hedef ajan yazabilmeli: %d", st)
	}
	if st, _ := e.send("r1", "mehmet", map[string]any{"thread": th}); st != 201 {
		t.Fatalf("insan yazabilmeli: %d", st)
	}
	_, b := e.send("r1", "ali", map[string]any{})
	if st, _ := e.send("r1", "can", map[string]any{"thread": b["thread"]}); st != 201 {
		t.Fatalf("herkese açık konuşmaya odadaki ajan yazabilmeli: %d", st)
	}
}

// NEDEN: başka konuşmadaki mesaja bağlanmak konuşma sınırını delerdi.
func TestReplyToBaskaKonusmadan400(t *testing.T) {
	e := newEnv(t, nil)
	_, a := e.send("r1", "ali", map[string]any{})
	_, b := e.send("r1", "ali", map[string]any{})
	if st, _ := e.send("r1", "veli", map[string]any{"thread": b["thread"], "reply_to": a["id"]}); st != 400 {
		t.Fatalf("başka konuşmadan reply_to: %d", st)
	}
	if st, _ := e.send("r1", "veli", map[string]any{"thread": b["thread"], "reply_to": b["id"]}); st != 201 {
		t.Fatalf("aynı konuşmadan reply_to: %d", st)
	}
}

// NEDEN: göndericinin göremediği (başkasına özel) bir mesaja reply_to vermek, mesajın
// varlığını sızdırırdı.
func TestReplyToGorunmeyenMesaj400(t *testing.T) {
	e := newEnv(t, nil)
	_, m1 := e.send("r1", "ali", map[string]any{"to": "veli", "body": "veli'ye özel"})
	th := m1["thread"]
	_, m2 := e.send("r1", "ali", map[string]any{"to": "can", "thread": th})
	if st, _ := e.send("r1", "can", map[string]any{"thread": th, "reply_to": m1["id"]}); st != 400 {
		t.Fatalf("can özel mesaja yanıt veremez: %d", st)
	}
	if st, _ := e.send("r1", "can", map[string]any{"thread": th, "reply_to": m2["id"]}); st != 201 {
		t.Fatalf("can kendine gelene yanıt verebilir: %d", st)
	}
}

// NEDEN: yeni açılan konuşmada yanıtlanacak bir mesaj olamaz.
func TestYeniKonusmadaReplyTo400(t *testing.T) {
	e := newEnv(t, nil)
	_, m := e.send("r1", "ali", map[string]any{})
	if st, _ := e.send("r1", "veli", map[string]any{"reply_to": m["id"]}); st != 400 {
		t.Fatalf("yeni konuşmada reply_to: %d", st)
	}
}

// NEDEN: taraf bilgisi mesajlardan değil thread_parties'ten gelir; saklama süresi mesajları
// silince alıcı okuma/yazma hakkını korur; kapatma açan ajan veya insana aittir.
func TestSaklamadanSonraAliciHalaTaraf(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()
	e.st.Now = func() time.Time { return time.Now().Add(-20 * 24 * time.Hour) }
	m, err := e.st.Post(ctx, store.PostInput{Room: "r1", From: "ali", FromKind: "ajan", To: "veli", Type: "soru", Body: "eski"})
	if err != nil {
		t.Fatal(err)
	}
	e.st.Now = time.Now
	e.srv.PurgeOld(ctx)
	if got := e.fetch("r1", "veli", 0, 0); len(got) != 0 {
		t.Fatalf("mesaj silinmiş olmalı: %v", got)
	}
	path := "/v1/rooms/r1/threads/" + m.Thread + "/close"
	if st, _ := e.call("POST", path, "can", nil); st != 403 {
		t.Fatalf("taraf olmayan: %d", st)
	}
	if st, _ := e.call("POST", path, "veli", nil); st != 403 {
		t.Fatalf("alıcı kapatamamalı: %d", st)
	}
	if st, _ := e.send("r1", "veli", map[string]any{"thread": m.Thread}); st != 201 {
		t.Fatalf("alıcı hâlâ yazabilmeli: %d", st)
	}
	if st, _ := e.call("POST", path, "ali", nil); st != 200 {
		t.Fatalf("açan ajan kapatabilmeli: %d", st)
	}
}

// NEDEN: konuşma geçmişi (özel mesajlar dahil) taraf olmayana açılmamalı; close ile tutarlı.
func TestKonusmaDetayiTarafOlmayana403(t *testing.T) {
	e := newEnv(t, nil)
	_, m := e.send("r1", "ali", map[string]any{"to": "veli", "body": "özel"})
	path := "/v1/rooms/r1/threads/" + m["thread"].(string)
	if st, _ := e.call("GET", path, "can", nil); st != 403 {
		t.Fatalf("taraf olmayan: %d", st)
	}
	for _, who := range []string{"veli", "ali", "mehmet"} {
		if st, _ := e.call("GET", path, who, nil); st != 200 {
			t.Fatalf("%s görebilmeli: %d", who, st)
		}
	}
}

// NEDEN: URL'den gelen thread değeri de gövdedeki gibi biçim denetiminden geçmeli.
func TestGecersizThreadYolu400(t *testing.T) {
	e := newEnv(t, nil)
	for _, th := range []string{"xyz", "T-", "t-1", "T-1x"} {
		if st, _ := e.call("GET", "/v1/rooms/r1/threads/"+th, "ali", nil); st != 400 {
			t.Fatalf("GET %q: %d", th, st)
		}
		if st, _ := e.call("POST", "/v1/rooms/r1/threads/"+th+"/close", "ali", nil); st != 400 {
			t.Fatalf("close %q: %d", th, st)
		}
	}
}

// NEDEN: log yolu 200 bayta kesilirken çok baytlı bir karakter ortadan bölünürse log bozuk
// UTF-8 içerir. Kesme karakter sınırında olmalı.
func TestLogYoluUTF8Bozmaz(t *testing.T) {
	e := newEnv(t, nil)
	e.raw("GET", "/ab"+strings.Repeat("ğ", 150), "", nil)
	out := e.logs.String()
	if !utf8.ValidString(out) || strings.Contains(out, `\x`) || !strings.Contains(out, "path=/abğğ") {
		t.Fatalf("log bozuk: %q", out)
	}
	if got := truncUTF8("ab"+strings.Repeat("ğ", 5), 3); got != "ab" {
		t.Fatalf("truncUTF8: %q", got)
	}
}

package bridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

func testStore(t *testing.T) stateStore {
	t.Helper()
	return stateStore{path: filepath.Join(t.TempDir(), "bridge.json"),
		lock: func(string) (func(), error) { return func() {}, nil }, replace: os.Rename}
}

const testSession = "00000000-0000-0000-0000-000000000001"

func Test_durum_dosyasi_toplam_boyut_ve_konusma_sayisiyla_sinirlanir(t *testing.T) {
	// NEDEN: konuşma başına sınır yetmez; her yeni konuşmaya bilgi yazan üye dosyayı
	// sınırsız büyütebilirdi. Toplam bayt ve konuşma sayısı da sınırlı kalmalıdır.
	s := testStore(t)
	for i := 1; i <= 120; i++ {
		c := &conversation{}
		c.add(proto.Message{ID: int64(i), Thread: fmt.Sprintf("T-%d", i), Type: "bilgi", Body: strings.Repeat("a", 16000)})
		if err := s.update(fmt.Sprintf("T-%d", i), c); err != nil {
			t.Fatal(err)
		}
	}
	st, err := os.Stat(s.path)
	if err != nil || st.Size() > maxStateBytes {
		t.Fatalf("durum dosyası sınırı aştı: %d", st.Size())
	}
	f, err := s.load()
	if err != nil || f.Threads["T-120"] == nil || f.Threads["T-1"] != nil {
		t.Fatal("en eski konuşma yerine yanlış konuşma budandı")
	}
	s = testStore(t)
	for i := 1; i <= maxConversations+50; i++ {
		c := &conversation{}
		c.add(proto.Message{ID: int64(i), Type: "bilgi", Body: "k"})
		if err := s.update(fmt.Sprintf("T-%d", i), c); err != nil {
			t.Fatal(err)
		}
	}
	if f, err = s.load(); err != nil || len(f.Threads) != maxConversations || f.Threads[fmt.Sprintf("T-%d", maxConversations+50)] == nil {
		t.Fatalf("konuşma sayısı sınırlanmadı: %d", len(f.Threads))
	}
}

func Test_budama_once_oturumsuz_tamponu_dusurur_bekleyen_teslimi_korur(t *testing.T) {
	// NEDEN: resume kimliği kaybı konuşma bağlamını siler, bekleyen teslim kaydı kaybı
	// cevabı kaybettirir; salt tampon konuşmalar daha yeni olsalar da önce düşmelidir.
	s := testStore(t)
	reply := Outgoing{To: "diger", Type: "cevap", Thread: "T-teslim", ReplyTo: 1, Body: "c", RequestID: "bridge-codex-1"}
	if err := s.update("T-teslim", &conversation{Delivery: &pendingDelivery{Room: "oda", Agent: "kopru",
		Source: proto.Message{ID: 1, Thread: "T-teslim", From: "diger"}, Reply: reply}}); err != nil {
		t.Fatal(err)
	}
	if err := s.update("T-oturum", &conversation{Session: testSession}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= maxConversations+10; i++ {
		c := &conversation{}
		c.add(proto.Message{ID: int64(i), Type: "bilgi", Body: "k"})
		if err := s.update(fmt.Sprintf("T-%d", i), c); err != nil {
			t.Fatal(err)
		}
	}
	f, err := s.load()
	if err != nil || len(f.Threads) != maxConversations {
		t.Fatalf("sınır uygulanmadı: %d %v", len(f.Threads), err)
	}
	if c := f.Threads["T-teslim"]; c == nil || c.Delivery == nil {
		t.Fatal("bekleyen teslim kaydı budandı")
	}
	if c := f.Threads["T-oturum"]; c == nil || c.Session != testSession {
		t.Fatal("oturumlu konuşma salt tampondan önce budandı")
	}
	// Yalnız oturumlu konuşmalar kalınca en uzun süredir kullanılmayan düşer.
	s = testStore(t)
	for i := 1; i <= maxConversations+1; i++ {
		if err := s.update(fmt.Sprintf("T-%d", i), &conversation{Session: testSession}); err != nil {
			t.Fatal(err)
		}
	}
	// T-1 yeniden kullanılır; en eski artık T-2'dir.
	if err := s.update("T-1", &conversation{Session: testSession}); err != nil {
		t.Fatal(err)
	}
	if f, err = s.load(); err != nil || f.Threads["T-1"] == nil || f.Threads["T-2"] != nil || len(f.Threads) != maxConversations {
		t.Fatal("en uzun süredir kullanılmayan oturum yerine başkası budandı")
	}
}

func Test_eski_durum_dosyasi_yeni_alan_olmadan_okunur(t *testing.T) {
	// NEDEN: son kullanım alanı sonradan eklendi; eski dosya reddedilmemeli, veri korunmalıdır.
	s := testStore(t)
	old := `{"threads":{"T-1":{"session":"` + testSession + `","pending":[{"id":3,"thread":"T-1","type":"bilgi","body":"eski"}]}}}`
	if err := os.WriteFile(s.path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := s.load()
	if err != nil || f.Threads["T-1"].Session != testSession || len(f.Threads["T-1"].Pending) != 1 {
		t.Fatalf("eski dosya okunamadı: %v", err)
	}
	if err := s.update("T-2", &conversation{Session: testSession}); err != nil {
		t.Fatal(err)
	}
	if f, err = s.load(); err != nil || f.Threads["T-1"] == nil || f.Threads["T-2"] == nil {
		t.Fatal("eski konuşma yazımda kayboldu")
	}
	var raw map[string]any
	b, _ := os.ReadFile(s.path)
	if json.Unmarshal(b, &raw) != nil || raw["threads"] == nil {
		t.Fatal("dosya biçimi değişti")
	}
}

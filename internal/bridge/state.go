package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

const (
	maxPendingMessages = 20
	maxPendingBytes    = 32 << 10
	// Dosyanın tümü için sınırlar; aşılınca en uzun süredir kullanılmayan konuşma düşer.
	maxConversations = 256
	maxStateBytes    = 512 << 10
)

type conversation struct {
	Session  string           `json:"session,omitempty"`
	Pending  []proto.Message  `json:"pending,omitempty"`
	Delivery *pendingDelivery `json:"delivery,omitempty"`
	// Used son yazımın sıra numarasıdır (saat değil); eski dosyada yoktur ve 0 = en eski sayılır.
	Used int64 `json:"used,omitempty"`
}

type pendingDelivery struct {
	// Room/Agent teslimin hangi köprüye ait olduğunu kaydeder; yeniden göndermeden önce denetlenir.
	Room   string        `json:"room,omitempty"`
	Agent  string        `json:"agent,omitempty"`
	Source proto.Message `json:"source"`
	Reply  Outgoing      `json:"reply"`
}

type stateFile struct {
	Threads map[string]*conversation `json:"threads"`
}

type stateStore struct {
	path    string
	lock    func(string) (func(), error)
	replace func(string, string) error
}

func (s stateStore) load() (stateFile, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return stateFile{Threads: make(map[string]*conversation)}, nil
	}
	if err != nil {
		return stateFile{}, fmt.Errorf("köprü oturum dosyası okunamadı: %s", s.path)
	}
	var f stateFile
	if json.Unmarshal(b, &f) != nil || f.Threads == nil {
		return f, fmt.Errorf("köprü oturum dosyası bozuk: %s", s.path)
	}
	for thread, c := range f.Threads {
		if c == nil || (c.Session != "" && !sessionRE.MatchString(c.Session)) {
			return f, fmt.Errorf("köprü oturum dosyası bozuk: %s", s.path)
		}
		if p := c.Delivery; p != nil {
			if p.Source.ID < 1 || p.Source.Thread != thread || p.Reply.Thread != thread ||
				p.Reply.ReplyTo != p.Source.ID || p.Reply.To != p.Source.From ||
				p.Reply.RequestID == "" || len(p.Reply.RequestID) > 64 || len(p.Reply.Body) > maxBody ||
				(p.Reply.Type != "cevap" && p.Reply.Type != "bilgi") {
				return f, fmt.Errorf("köprü gönderim kaydı bozuk: %s", s.path)
			}
		}
		c.trim()
	}
	return f, nil
}

// update kilidi yalnız kısa oku/değiştir/yaz işlemi boyunca tutar; ajan çalışırken
// tutulmaz. Diğer konuşmaların eşlemeleri eşzamanlı yazımda korunur.
func (s stateStore) update(thread string, c *conversation) error {
	unlock, err := s.lock(s.path)
	if err != nil {
		return err
	}
	defer unlock()
	f, err := s.load()
	if err != nil {
		return err
	}
	if c == nil {
		delete(f.Threads, thread)
	} else {
		for _, old := range f.Threads {
			c.Used = max(c.Used, old.Used)
		}
		c.Used++
		f.Threads[thread] = c
	}
	f.prune(thread)
	b, err := json.Marshal(f)
	if err != nil {
		return errors.New("köprü oturumları kodlanamadı")
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return errors.New("köprü oturum dosyası yazılamadı")
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = s.replace(tmp.Name(), s.path)
	}
	if err != nil {
		return errors.New("köprü oturum dosyası yazılamadı")
	}
	return nil
}

// prune dosyayı konuşma sayısı ve toplam bayt sınırına indirir. Önce oturumu olmayan
// salt tampon konuşmalar, sonra oturumlular en eski kullanımdan başlayarak düşer.
// Bekleyen teslim kaydı olan ve o an yazılan konuşma budanmaz.
func (f *stateFile) prune(keep string) {
	type entry struct {
		thread string
		c      *conversation
		size   int
	}
	total := len(`{"threads":{}}`)
	var list []entry
	for thread, c := range f.Threads {
		k, _ := json.Marshal(thread)
		v, _ := json.Marshal(c)
		size := len(k) + len(v) + 2 // iki nokta ve virgül
		total += size
		if thread != keep && c.Delivery == nil {
			list = append(list, entry{thread, c, size})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if (a.c.Session == "") != (b.c.Session == "") {
			return a.c.Session == ""
		}
		if a.c.Used != b.c.Used {
			return a.c.Used < b.c.Used
		}
		return a.thread < b.thread
	})
	for _, e := range list {
		if len(f.Threads) <= maxConversations && total <= maxStateBytes {
			return
		}
		delete(f.Threads, e.thread)
		total -= e.size
	}
}

func (c *conversation) add(m proto.Message) {
	for _, old := range c.Pending {
		if old.ID == m.ID { // kayıt ile imleç arasında çökülürse aynı bağlamı çoğaltma
			return
		}
	}
	c.Pending = append(c.Pending, m)
	c.trim()
}

func (c *conversation) trim() {
	// 32 KB sınırı gövdeyle birlikte metadata'nın JSON boyutunu da kapsar.
	for len(c.Pending) > 0 {
		b, _ := json.Marshal(c.Pending)
		if len(c.Pending) <= maxPendingMessages && len(b) <= maxPendingBytes {
			break
		}
		c.Pending = c.Pending[1:]
	}
}

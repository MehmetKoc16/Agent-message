package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

const (
	maxPendingMessages = 20
	maxPendingBytes    = 32 << 10
)

type conversation struct {
	Session  string           `json:"session,omitempty"`
	Pending  []proto.Message  `json:"pending,omitempty"`
	Delivery *pendingDelivery `json:"delivery,omitempty"`
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
		f.Threads[thread] = c
	}
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

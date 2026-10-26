package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

const (
	maxBody      = 16384
	secretReply  = "köprü: cevap sır taraması nedeniyle gönderilmedi"
	failureReply = "köprü: ajan çalıştırılamadı"
	budgetReply  = "köprü: saatlik çalıştırma sınırına ulaşıldı; mesajınız kaydedildi ama yanıtlanmadı, lütfen daha sonra yeniden sorun"
	tooLongReply = "köprü: mesaj ajan için çok uzun"
)

type Outgoing struct {
	To, Type, Thread, Body string
	ReplyTo                int64
	RequestID              string
}

// Dependencies mevcut istemcinin HTTP, imleç, çerçeve ve sır taramasını paylaşır.
// Köprü böylece aynı çıkış/hata politikasını kullanır ve client paketine bağımlı olmaz.
type Dependencies struct {
	Cursor interface {
		Read() (int64, error)
		Advance(int64) error
	}
	Fetch               func(context.Context, int64, int) ([]proto.Message, error)
	Send                func(context.Context, Outgoing) error
	Frame               func(proto.Message) string
	Scan                func(string) (string, bool)
	Transient, Conflict func(error) bool
	RateLimited         func(error) bool
	Lock                func(string) (func(), error)
	Replace             func(string, string) error
	Stderr              io.Writer
	PollSeconds         int
	RetryDelay          time.Duration
	SendRetryDelay      time.Duration
}

type engine struct {
	o        Options
	d        Dependencies
	store    stateStore
	runs     []time.Time
	notified map[string]time.Time
	failures int
}

// Run seri çalışır. Sinyal iptali context üzerinden hem long-poll'a hem sürece ulaşır.
func Run(ctx context.Context, o Options, d Dependencies) error {
	if err := o.prepare(); err != nil {
		return err
	}
	if d.SendRetryDelay <= 0 {
		d.SendRetryDelay = 15 * time.Second
	}
	if o.Runner == "codex" && o.Sandbox == "workspace-write" {
		fmt.Fprintln(d.Stderr, "UYARI: workspace-write seçildi; Codex çalışma alanına yazabilir. Köprü talimatı yalnız okuyarak cevap vermesini ister.")
	}
	e := engine{o: o, d: d, store: stateStore{o.StatePath, d.Lock, d.Replace}, notified: make(map[string]time.Time)}
	if _, err := e.store.load(); err != nil {
		return err
	}
	if _, err := d.Cursor.Read(); err != nil {
		return err
	}
	if err := e.resumeDeliveries(ctx); err != nil {
		return err
	}
	for ctx.Err() == nil {
		after, err := d.Cursor.Read()
		if err != nil {
			return err
		}
		wait := d.PollSeconds
		if o.Once {
			wait = 0
		}
		msgs, err := d.Fetch(ctx, after, wait)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !d.Transient(err) || o.Once {
				return err
			}
			fmt.Fprintf(d.Stderr, "%s işlem=hata sunucu_geçici=true süre=0s\n", time.Now().Format(time.RFC3339))
			timer := time.NewTimer(d.RetryDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
			continue
		}
		sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].ID < msgs[j].ID })
		for _, m := range msgs {
			if m.ID <= after {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := e.process(ctx, m); err != nil {
				return err // başarısız gönderim/kayıt: imleç yerinde kalır
			}
			if err := d.Cursor.Advance(m.ID); err != nil {
				return err
			}
			if err := e.clearDelivery(m.Thread); err != nil {
				return err
			}
			after = m.ID
			if e.failures >= 3 {
				return errors.New("köprü üç ardışık çalıştırıcı hatası nedeniyle durdu")
			}
		}
		if o.Once {
			return nil
		}
	}
	return ctx.Err()
}

func (e *engine) log(m proto.Message, action string, duration time.Duration, detail string) {
	fmt.Fprintf(e.d.Stderr, "%s konuşma=%q mesaj=%d tip=%q işlem=%s süre=%.3fs%s\n",
		time.Now().Format(time.RFC3339), m.Thread, m.ID, m.Type, action, duration.Seconds(), detail)
}

func (e *engine) allowed(now time.Time) bool {
	cutoff := now.Add(-time.Hour)
	i := 0
	for i < len(e.runs) && !e.runs[i].After(cutoff) {
		i++
	}
	e.runs = e.runs[i:]
	for thread, sent := range e.notified {
		if !sent.After(cutoff) {
			delete(e.notified, thread)
		}
	}
	return len(e.runs) < e.o.MaxRuns
}

func (e *engine) send(ctx context.Context, m proto.Message, typ, body string, duration time.Duration) error {
	f, err := e.store.load()
	if err != nil {
		return err
	}
	c := f.Threads[m.Thread]
	if c == nil {
		c = &conversation{}
	}
	return e.sendReply(ctx, m, typ, body, duration, c)
}

func (e *engine) sendReply(ctx context.Context, m proto.Message, typ, body string, duration time.Duration, c *conversation) error {
	// Cevap ve runner oturumu tek atomik yazımla, imleç onaylanana kadar saklanır.
	pending := Outgoing{To: m.From, Type: typ, Thread: m.Thread, ReplyTo: m.ID, Body: body,
		RequestID: fmt.Sprintf("bridge-%s-%d", e.o.Runner, m.ID)}
	source := m
	source.Body = "" // Yeniden gönderim için gelen mesajın gövdesi gerekmez.
	c.Delivery = &pendingDelivery{Source: source, Reply: pending}
	if err := e.store.update(m.Thread, c); err != nil {
		return err
	}
	return e.deliver(ctx, m, pending, duration)
}

func (e *engine) clearDelivery(thread string) error {
	f, err := e.store.load()
	if err != nil {
		return err
	}
	if c := f.Threads[thread]; c != nil && c.Delivery != nil {
		c.Delivery = nil
		return e.store.update(thread, c)
	}
	return nil
}

func (e *engine) resumeDeliveries(ctx context.Context) error {
	f, err := e.store.load()
	if err != nil {
		return err
	}
	var pending []*pendingDelivery
	for _, c := range f.Threads {
		if c.Delivery != nil {
			pending = append(pending, c.Delivery)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Source.ID < pending[j].Source.ID })
	for _, p := range pending {
		if err := e.deliver(ctx, p.Source, p.Reply, 0); err != nil {
			return err
		}
		if err := e.d.Cursor.Advance(p.Source.ID); err != nil {
			return err
		}
		if err := e.clearDelivery(p.Source.Thread); err != nil {
			return err
		}
	}
	return nil
}

func (e *engine) deliver(ctx context.Context, m proto.Message, pending Outgoing, duration time.Duration) error {
	// An earlier binary may have saved a reply under weaker scanning rules.
	// Recheck durable deliveries before every resumed network send.
	if name, found := e.d.Scan(pending.Body); found {
		e.log(m, "hata", duration, fmt.Sprintf(" sır_deseni=%q gönderim=engellendi", name))
		return errors.New("köprü: kayıtlı cevap sır denetimini geçemedi; gönderim durduruldu")
	}
	for attempt := 1; attempt <= 5; attempt++ {
		err := e.d.Send(ctx, pending)
		if err == nil {
			e.log(m, "gonderildi", duration, "")
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e.d.Conflict(err) {
			e.log(m, "hata", duration, " HTTP=409 oturum=temizlendi")
			return e.store.update(m.Thread, nil)
		}
		if e.d.RateLimited != nil && e.d.RateLimited(err) {
			e.log(m, "hata", duration, fmt.Sprintf(" HTTP=429 deneme=%d", attempt))
			if attempt == 5 {
				return errors.New("köprü gönderimi beş denemede tamamlayamadı: relay hız sınırı")
			}
			timer := time.NewTimer(e.d.SendRetryDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
			continue
		}
		e.log(m, "hata", duration, " gönderim=başarısız")
		return err
	}
	return nil
}

// fitPrompt yalnız eski bağlamı budar; yeni mesaj ve ilk tur önsözü değişmez.
func (e *engine) fitPrompt(m proto.Message, c *conversation) (string, error) {
	dropped := 0
	for {
		prompt := Prompt(e.o.Agent, e.o.Room, c.Session == "", c.Pending, m, e.d.Frame)
		_, err := arguments(e.o, c.Session, "", prompt)
		if errors.Is(err, errPromptTooLong) && len(c.Pending) > 0 {
			c.Pending = c.Pending[1:]
			dropped++
			continue
		}
		if dropped > 0 {
			if saveErr := e.store.update(m.Thread, c); saveErr != nil {
				return "", saveErr
			}
			e.log(m, "tampon_budandi", 0, fmt.Sprintf(" adet=%d", dropped))
		}
		return prompt, err
	}
}

func (e *engine) process(ctx context.Context, m proto.Message) error {
	f, err := e.store.load()
	if err != nil {
		return err
	}
	c := f.Threads[m.Thread]
	if c == nil {
		c = &conversation{}
	}
	switch m.Type {
	case "kapat":
		if err := e.store.update(m.Thread, nil); err != nil {
			return err
		}
		e.log(m, "sessiz", 0, " oturum=temizlendi")
		return nil
	case "cevap", "sonuc", "bilgi":
		c.add(m)
		if err := e.store.update(m.Thread, c); err != nil {
			return err
		}
		e.log(m, "tampon", 0, "")
		return nil
	case "soru", "gorev":
	default:
		e.log(m, "bilinmeyen_tip", 0, "")
		return nil
	}
	prompt, err := e.fitPrompt(m, c)
	if errors.Is(err, errPromptTooLong) {
		e.log(m, "mesaj_uzun", 0, "")
		return e.send(ctx, m, "cevap", tooLongReply, 0)
	}
	if err != nil {
		return err
	}
	now := time.Now()
	if !e.allowed(now) {
		c.add(m)
		if err := e.store.update(m.Thread, c); err != nil {
			return err
		}
		e.log(m, "butce", 0, "")
		if _, done := e.notified[m.Thread]; !done {
			if err := e.send(ctx, m, "bilgi", budgetReply, 0); err != nil {
				return err
			}
			e.notified[m.Thread] = now
		}
		return nil
	}
	e.runs = append(e.runs, now) // hatalar da kotayı tüketebilir; denemeyi say
	result, err := invoke(ctx, e.o, c.Session, prompt)
	if err != nil {
		e.debug(m, result, prompt, c.Pending)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		e.failures++
		// Yalnız köprünün ürettiği sabit hata; çalıştırıcı stderr'i asla aktarılmaz.
		e.log(m, "hata", result.Duration, fmt.Sprintf(" çıkış=%d neden=%q", result.ExitCode, err))
		return e.send(ctx, m, "cevap", failureReply, result.Duration)
	}
	e.failures = 0
	e.log(m, "calistirildi", result.Duration, fmt.Sprintf(" çıkış=%d", result.ExitCode))
	c.Session, c.Pending = result.Session, nil
	body := strings.TrimSpace(strings.ToValidUTF8(result.Response, "�"))
	silent, suspicious := responseKind(body)
	if silent {
		e.log(m, "sessiz", result.Duration, "")
		return e.store.update(m.Thread, c)
	}
	if suspicious {
		e.log(m, "supheli", result.Duration, " uyarı=şüpheli_cevap")
	}
	// Taramayı kısaltmadan önce yap: uzun bir cevabın sonundaki sır da engellenir.
	if name, found := e.d.Scan(body); found {
		e.log(m, "hata", result.Duration, fmt.Sprintf(" sır_deseni=%q", name))
		body = secretReply
	}
	return e.sendReply(ctx, m, "cevap", shorten(body), result.Duration, c)
}

func shorten(body string) string {
	if len(body) <= maxBody {
		return body
	}
	const suffix = "… (köprü kısalttı)"
	n := maxBody - len(suffix)
	for !utf8.RuneStart(body[n]) {
		n--
	}
	return body[:n] + suffix
}

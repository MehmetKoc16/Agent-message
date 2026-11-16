package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

// ErrUnreachable sunucuya ulaşılamadığını (ağ hatası, zaman aşımı) belirtir.
var ErrUnreachable = errors.New("sunucuya ulaşılamadı")

// ErrUnavailable ters vekil (Caddy) 502/503/504 döndürdü: sunucu geçici olarak yanıt vermiyor
// (ör. yeniden başlıyor). wait bunu ulaşılamama gibi geçici sayıp yeniden dener.
var ErrUnavailable = errors.New("sunucu geçici olarak yanıt vermiyor")

// ErrTooLarge sunucu yanıtı maxResponse sınırını aştı. Kesilmiş gövde ayrıştırılmaz;
// "geçersiz yanıt"tan ayrı bildirilir.
var ErrTooLarge = errors.New("sunucu yanıtı çok büyük (4 MiB sınırı)")

// maxResponse tek yanıttan okunan en çok bayttır.
const maxResponse = 4 << 20

// ErrCanceled kullanıcı iptali (Ctrl+C / context iptali).
var ErrCanceled = errors.New("iptal edildi")

func isTransient(err error) bool {
	return errors.Is(err, ErrUnreachable) || errors.Is(err, ErrUnavailable)
}

// APIError sunucunun döndürdüğü HTTP hatasıdır.
type APIError struct {
	Status int
	Code   string
}

func (e *APIError) Error() string { return fmt.Sprintf("HTTP %d (%s)", e.Status, e.Code) }

type api struct {
	base, token, room string
	hc                *http.Client
}

func newAPI(c *Config) *api {
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0, // long-poll: istek başına context süresi geçerli
		DisableKeepAlives:     false,
		MaxIdleConns:          2,
		IdleConnTimeout:       30 * time.Second,
	}
	return &api{base: c.URL, token: c.Token, room: c.Room, hc: &http.Client{
		Transport:     tr,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (a *api) do(parent context.Context, method, path string, payload, out any, timeout time.Duration) error {
	data, err := a.call(parent, method, path, payload, timeout)
	if err != nil || out == nil {
		return err
	}
	if len(data) > maxResponse {
		return ErrTooLarge
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &APIError{Status: http.StatusOK, Code: "gecersiz_yanit"}
	}
	return nil
}

// call isteği yapar ve başarılı yanıtın gövdesini döndürür. Gövde maxResponse'tan uzunsa
// sınır+1 bayt döner: çağıran kesilmeyi uzunluktan anlar.
func (a *api) call(parent context.Context, method, path string, payload any, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		if parent.Err() != nil {
			return nil, ErrCanceled
		}
		return nil, ErrUnreachable
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		if parent.Err() != nil {
			return nil, ErrCanceled
		}
		return nil, ErrUnreachable
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return nil, ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		return nil, &APIError{Status: resp.StatusCode, Code: e.Error}
	}
	return data, nil
}

type sendInput struct {
	To        string `json:"to,omitempty"`
	Type      string `json:"type"`
	Thread    string `json:"thread,omitempty"`
	ReplyTo   *int64 `json:"reply_to,omitempty"`
	Body      string `json:"body"`
	RequestID string `json:"request_id,omitempty"`
}

func (a *api) send(ctx context.Context, in sendInput) (proto.Message, error) {
	var m proto.Message
	err := a.do(ctx, http.MethodPost, "/v1/rooms/"+url.PathEscape(a.room)+"/messages", in, &m, 30*time.Second)
	return m, err
}

func (a *api) fetch(ctx context.Context, after int64, wait int) ([]proto.Message, error) {
	p := fmt.Sprintf("/v1/rooms/%s/messages?after=%d&wait=%d", url.PathEscape(a.room), after, wait)
	data, err := a.call(ctx, http.MethodGet, p, nil, time.Duration(wait+10)*time.Second)
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponse {
		// Sayfayı baytla sınırlamayan sunucu: hata verip imleci aynı yanıta kilitlemek yerine
		// sığan tam mesajlar teslim edilir; imleç onlarla ilerler, kalanı sonraki çağrı alır.
		msgs := partialMessages(data[:maxResponse])
		if len(msgs) == 0 {
			return nil, ErrTooLarge
		}
		return msgs, nil
	}
	var r struct {
		Messages []proto.Message `json:"messages"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, &APIError{Status: http.StatusOK, Code: "gecersiz_yanit"}
	}
	return r.Messages, nil
}

// partialMessages kesilmiş bir {"messages":[...]} gövdesinden yalnız tam ayrıştırılan baştaki
// mesajları döndürür; yarım kalan son öğe alınmaz. id artmıyorsa orada durur: imleç en büyük
// id'ye ilerlediği için sırasız bir kalan, teslim edilmeden atlanırdı.
func partialMessages(data []byte) []proto.Message {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil
		}
		if key != "messages" {
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				return nil
			}
			continue
		}
		if t, err := dec.Token(); err != nil || t != json.Delim('[') {
			return nil
		}
		var msgs []proto.Message
		for dec.More() {
			var m proto.Message
			if dec.Decode(&m) != nil || (len(msgs) > 0 && m.ID <= msgs[len(msgs)-1].ID) {
				break
			}
			msgs = append(msgs, m)
		}
		return msgs
	}
	return nil
}

type threadView struct {
	proto.Thread
	Messages []proto.Message `json:"messages"`
	HasMore  bool            `json:"has_more"` // daha eski mesaj var (eski sunucuda hep false)
}

// thread konuşmanın en yeni sayfasını, before > 0 ise before öncesindeki sayfayı okur.
func (a *api) thread(ctx context.Context, id string, before int64) (threadView, error) {
	var t threadView
	p := "/v1/rooms/" + url.PathEscape(a.room) + "/threads/" + url.PathEscape(id)
	if before > 0 {
		p += fmt.Sprintf("?before=%d", before)
	}
	err := a.do(ctx, http.MethodGet, p, nil, &t, 30*time.Second)
	return t, err
}

func (a *api) closeThread(ctx context.Context, id string) error {
	return a.do(ctx, http.MethodPost, "/v1/rooms/"+url.PathEscape(a.room)+"/threads/"+url.PathEscape(id)+"/close", nil, nil, 30*time.Second)
}

type agentView struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	LastSeen string `json:"last_seen"`
	Waiting  bool   `json:"waiting"`
}

func (a *api) agents(ctx context.Context) ([]agentView, error) {
	var r struct {
		Agents []agentView `json:"agents"`
	}
	err := a.do(ctx, http.MethodGet, "/v1/rooms/"+url.PathEscape(a.room)+"/agents", nil, &r, 30*time.Second)
	return r.Agents, err
}

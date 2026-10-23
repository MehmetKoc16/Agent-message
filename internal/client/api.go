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
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		if parent.Err() != nil {
			return ErrCanceled
		}
		return ErrUnreachable
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		if parent.Err() != nil {
			return ErrCanceled
		}
		return ErrUnreachable
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		return &APIError{Status: resp.StatusCode, Code: e.Error}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return &APIError{Status: resp.StatusCode, Code: "gecersiz_yanit"}
		}
	}
	return nil
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
	var r struct {
		Messages []proto.Message `json:"messages"`
	}
	p := fmt.Sprintf("/v1/rooms/%s/messages?after=%d&wait=%d", url.PathEscape(a.room), after, wait)
	err := a.do(ctx, http.MethodGet, p, nil, &r, time.Duration(wait+10)*time.Second)
	return r.Messages, err
}

type threadView struct {
	proto.Thread
	Messages []proto.Message `json:"messages"`
}

func (a *api) thread(ctx context.Context, id string) (threadView, error) {
	var t threadView
	err := a.do(ctx, http.MethodGet, "/v1/rooms/"+url.PathEscape(a.room)+"/threads/"+url.PathEscape(id), nil, &t, 30*time.Second)
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

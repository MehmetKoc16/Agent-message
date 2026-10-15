// Package server, relay'in HTTP API'sini (docs/protokol.md §4) sunar.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

var (
	threadRe  = regexp.MustCompile(`^T-[0-9]{1,9}$`)
	msgTypes  = map[string]bool{"soru": true, "gorev": true, "cevap": true, "sonuc": true, "bilgi": true, "kapat": true}
	touchEach = 30 * time.Second
)

const maxFetch = 100

// Server HTTP işleyicilerini ve sınırları bir arada tutar.
type Server struct {
	cfg          config.Server
	st           *store.Store
	log          *slog.Logger
	hub          *Hub
	rate         *rateLimiter
	conns        *connLimiter
	unauth       *unauthLimiter
	authenticate func(context.Context, string) (store.Agent, error)

	touchMu sync.Mutex
	touched map[string]time.Time
}

// New sunucuyu kurar. Log hiçbir zaman gövde, token ya da Authorization içermez.
func New(cfg config.Server, st *store.Store, log *slog.Logger) *Server {
	return &Server{
		cfg: cfg, st: st, log: log, hub: NewHub(),
		rate:         newRateLimiter(cfg.RatePerMin, cfg.RatePerDay),
		conns:        newConnLimiter(cfg.MaxConnsPerAgent),
		unauth:       newUnauthLimiter(cfg.UnauthPerMin, cfg.UnauthGlobalPerMin),
		authenticate: st.Authenticate,
		touched:      map[string]time.Time{},
	}
}

// Close bekleyen tüm long-poll'ları uyandırır.
func (s *Server) Close() { s.hub.Close() }

// Handler tüm yolları (log ve çökme koruması dahil) döndürür.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/rooms/{room}/messages", s.room(s.handlePost))
	mux.HandleFunc("GET /v1/rooms/{room}/messages", s.room(s.handleFetch))
	mux.HandleFunc("GET /v1/rooms/{room}/threads/{thread}", s.room(s.handleThread))
	mux.HandleFunc("POST /v1/rooms/{room}/threads/{thread}/close", s.room(s.handleClose))
	mux.HandleFunc("GET /v1/rooms/{room}/agents", s.room(s.handleAgents))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "bulunamadi")
	})
	return s.logging(mux)
}

// ---- ortak yardımcılar ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func bearer(r *http.Request) string {
	v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	v = strings.TrimSpace(v)
	if !ok || v == "" || len(v) > 256 || len(r.Header.Values("Authorization")) != 1 || strings.ContainsAny(v, " \t\r\n") {
		return ""
	}
	return v
}

type roomHandler func(w http.ResponseWriter, r *http.Request, a store.Agent, room string)

// room kimlik doğrulama, oda üyeliği, eşzamanlı istek ve hız sınırını uygular.
func (s *Server) room(h roomHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := infoFrom(r.Context())
		token := bearer(r)
		// Başarısız sayılar ve eşzamanlı rezervasyonlar DB'den önce denetlenir.
		finish, ok := s.unauth.begin(s.clientIP(r), token)
		if !ok {
			writeErr(w, http.StatusTooManyRequests, "kimliksiz_hiz_siniri")
			return
		}
		failed := false
		defer func() { finish(failed) }()
		if token == "" {
			failed = true
			writeErr(w, http.StatusUnauthorized, "yetkisiz")
			return
		}
		a, err := s.authenticate(r.Context(), token)
		if errors.Is(err, store.ErrUnauthorized) {
			failed = true
			writeErr(w, http.StatusUnauthorized, "yetkisiz")
			return
		} else if err != nil {
			s.internal(w, "kimlik doğrulama", err)
			return
		}
		s.unauth.recognize(token)
		// Kimlik doğrulandı: rezervasyonu uzun isteğin sonuna kadar tutma.
		finish(false)
		info.agent = a.Name
		// Yer ayırma, DB'ye başka sorgu atmadan önce yapılır (tek bağlantıya gereksiz yük binmesin).
		release, ok := s.conns.Acquire(a.Name)
		if !ok {
			writeErr(w, http.StatusTooManyRequests, "cok_fazla_baglanti")
			return
		}
		defer release()
		room := r.PathValue("room")
		member, err := s.st.HasRoom(r.Context(), a.Name, room)
		if err != nil {
			s.internal(w, "oda denetimi", err)
			return
		}
		if !member {
			writeErr(w, http.StatusForbidden, "yasak")
			return
		}
		if r.Method == http.MethodPost && !s.rate.Allow(a.Name) {
			writeErr(w, http.StatusTooManyRequests, "hiz_siniri")
			return
		}
		s.touch(a.Name)
		h(w, r, a, room)
	}
}

func (s *Server) internal(w http.ResponseWriter, what string, err error) {
	// Ayrıntı yalnız sunucu logunda (gövde/token içermez); istemciye genel kod gider.
	s.log.Error("iç hata", "islem", what, "err", err.Error())
	writeErr(w, http.StatusInternalServerError, "sunucu_hatasi")
}

// touch last_seen alanını en fazla 30 sn'de bir yazar.
func (s *Server) touch(name string) {
	s.touchMu.Lock()
	last, ok := s.touched[name]
	now := time.Now()
	due := !ok || now.Sub(last) >= touchEach
	if due {
		s.touched[name] = now
	}
	s.touchMu.Unlock()
	if due {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.st.Touch(ctx, name); err != nil {
			s.log.Warn("last_seen yazılamadı", "err", err.Error())
		}
	}
}

// ---- işleyiciler ----

type postRequest struct {
	To      string `json:"to"`
	Type    string `json:"type"`
	Thread  string `json:"thread"`
	ReplyTo *int64 `json:"reply_to"`
	Body    string `json:"body"`
	// "from" alanı bilerek yok: gövdede gelse de yok sayılır (kimlik token'dandır).
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	// JSON kaçışları gövdeyi büyütebilir; okuma sınırı bu yüzden max_body'nin birkaç katıdır.
	// Asıl gövde sınırı çözümlendikten sonra aşağıda uygulanır.
	r.Body = http.MaxBytesReader(w, r.Body, int64(4*s.cfg.MaxBody+1024))
	var in postRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "govde_cok_buyuk")
			return
		}
		writeErr(w, http.StatusBadRequest, "gecersiz_json")
		return
	}
	if len(in.Body) > s.cfg.MaxBody {
		writeErr(w, http.StatusRequestEntityTooLarge, "govde_cok_buyuk")
		return
	}
	switch {
	case strings.TrimSpace(in.Body) == "" || strings.ContainsRune(in.Body, 0) || !utf8.ValidString(in.Body):
		writeErr(w, http.StatusBadRequest, "gecersiz_govde")
	case !msgTypes[in.Type]:
		writeErr(w, http.StatusBadRequest, "gecersiz_tip")
	case in.To != "" && !config.NameRe.MatchString(in.To):
		writeErr(w, http.StatusBadRequest, "gecersiz_alici")
	case in.Thread != "" && !threadRe.MatchString(in.Thread):
		writeErr(w, http.StatusBadRequest, "gecersiz_thread")
	case in.ReplyTo != nil && *in.ReplyTo < 1:
		writeErr(w, http.StatusBadRequest, "gecersiz_reply_to")
	default:
		s.post(w, r, a, room, in)
	}
}

func (s *Server) post(w http.ResponseWriter, r *http.Request, a store.Agent, room string, in postRequest) {
	m, err := s.st.Post(r.Context(), store.PostInput{
		Room: room, From: a.Name, FromKind: a.Kind, To: in.To, Type: in.Type, Thread: in.Thread,
		ReplyTo: in.ReplyTo, Body: in.Body,
	})
	switch {
	case errors.Is(err, store.ErrBadRecipient):
		writeErr(w, http.StatusBadRequest, "gecersiz_alici")
	case errors.Is(err, store.ErrBadReplyTo):
		writeErr(w, http.StatusBadRequest, "gecersiz_reply_to")
	case errors.Is(err, store.ErrThreadNotFound):
		writeErr(w, http.StatusNotFound, "konusma_yok")
	case errors.Is(err, store.ErrNotParty):
		writeErr(w, http.StatusForbidden, "yasak")
	case errors.Is(err, store.ErrThreadClosed):
		writeErr(w, http.StatusConflict, "konusma_kilitli")
	case err != nil:
		s.internal(w, "mesaj ekleme", err)
	default:
		s.hub.Notify(room) // commit'ten SONRA
		writeJSON(w, http.StatusCreated, m)
	}
}

func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	q := r.URL.Query()
	var after int64
	if v := q.Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, "gecersiz_after")
			return
		}
		after = n
	}
	wait := 0
	if v := q.Get("wait"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, "gecersiz_wait")
			return
		}
		wait = min(n, s.cfg.MaxWaitSec)
	}

	var timeout <-chan time.Time
	if wait > 0 {
		defer s.hub.BeginWait(room, a.Name)()
		t := time.NewTimer(time.Duration(wait) * time.Second)
		defer t.Stop()
		timeout = t.C
	}
	for {
		ch := s.hub.Chan(room) // sorgudan ÖNCE alınır (kaçırılan bildirim olmasın)
		msgs, err := s.st.Fetch(r.Context(), room, a.Name, after, maxFetch)
		if err != nil {
			if r.Context().Err() != nil {
				w.WriteHeader(499)
				return
			}
			s.internal(w, "mesaj okuma", err)
			return
		}
		if len(msgs) > 0 || wait == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
			return
		}
		select {
		case <-ch:
		case <-timeout:
			s.finalFetch(w, r, room, a.Name, after)
			return
		case <-s.hub.Done():
			s.finalFetch(w, r, room, a.Name, after)
			return
		case <-r.Context().Done():
			w.WriteHeader(499) // istemci koptu; bekleme bırakıldı
			return
		}
	}
}

// finalFetch bekleme bitince (süre ya da kapanış) son bir kez bakar: süre dolarken gelen
// mesaj select'te kaybolmasın.
func (s *Server) finalFetch(w http.ResponseWriter, r *http.Request, room, agent string, after int64) {
	msgs, err := s.st.Fetch(r.Context(), room, agent, after, maxFetch)
	if err != nil {
		if r.Context().Err() != nil {
			w.WriteHeader(499)
			return
		}
		s.internal(w, "mesaj okuma", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *Server) handleThread(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	id := r.PathValue("thread")
	if !threadRe.MatchString(id) {
		writeErr(w, http.StatusBadRequest, "gecersiz_thread")
		return
	}
	t, err := s.st.GetThread(r.Context(), room, id)
	if errors.Is(err, store.ErrThreadNotFound) {
		writeErr(w, http.StatusNotFound, "konusma_yok")
		return
	} else if err != nil {
		s.internal(w, "konuşma okuma", err)
		return
	}
	if a.Kind != "insan" {
		party, err := s.st.IsThreadParty(r.Context(), room, id, a.Name)
		if err != nil {
			s.internal(w, "taraf denetimi", err)
			return
		}
		if !party {
			writeErr(w, http.StatusForbidden, "yasak")
			return
		}
	}
	msgs, err := s.st.ThreadMessages(r.Context(), room, id, a.Name)
	if err != nil {
		s.internal(w, "konuşma mesajları", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		store.Thread
		Messages []store.Message `json:"messages"`
	}{t, msgs})
}

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	id := r.PathValue("thread")
	if !threadRe.MatchString(id) {
		writeErr(w, http.StatusBadRequest, "gecersiz_thread")
		return
	}
	if _, err := s.st.GetThread(r.Context(), room, id); errors.Is(err, store.ErrThreadNotFound) {
		writeErr(w, http.StatusNotFound, "konusma_yok")
		return
	} else if err != nil {
		s.internal(w, "konuşma okuma", err)
		return
	}
	if a.Kind != "insan" {
		party, err := s.st.IsThreadParty(r.Context(), room, id, a.Name)
		if err != nil {
			s.internal(w, "taraf denetimi", err)
			return
		}
		if !party {
			writeErr(w, http.StatusForbidden, "yasak")
			return
		}
	}
	if err := s.st.CloseThread(r.Context(), room, id); err != nil {
		s.internal(w, "konuşma kapatma", err)
		return
	}
	t, err := s.st.GetThread(r.Context(), room, id)
	if err != nil {
		s.internal(w, "konuşma okuma", err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	list, err := s.st.RoomAgents(r.Context(), room)
	if err != nil {
		s.internal(w, "ajan listesi", err)
		return
	}
	waiting := s.hub.Waiting(room)
	type item struct {
		store.RoomAgent
		Waiting bool `json:"waiting"`
	}
	out := make([]item, 0, len(list))
	for _, ag := range list {
		out = append(out, item{ag, waiting[ag.Name]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": out})
}

// ---- çalıştırma ----

// Run yapılandırılan TCP/Unix/systemd dinleyicisini kullanır.
func (s *Server) Run(ctx context.Context) error {
	ln, err := Listen(s.cfg)
	if err != nil {
		return err
	}
	defer ln.Close()
	return s.Serve(ctx, ln)
}

// Serve ln üzerinde hizmet verir; ctx iptalinde düzgün kapanır.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      time.Duration(s.cfg.MaxWaitSec)*time.Second + 15*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}
	purgeDone := make(chan struct{})
	pctx, stopPurge := context.WithCancel(context.Background())
	go func() {
		defer close(purgeDone)
		s.purgeLoop(pctx)
	}()

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	s.log.Info("dinleniyor", "adres", ln.Addr().String())

	select {
	case err := <-errc:
		stopPurge()
		<-purgeDone
		s.Close()
		return err
	case <-ctx.Done():
	}
	s.log.Info("kapanıyor")
	s.Close() // bekleyen long-poll'lar uyansın
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := srv.Shutdown(sctx)
	stopPurge()
	<-purgeDone
	if err != nil {
		_ = srv.Close()
	}
	return err
}

func (s *Server) purgeLoop(ctx context.Context) {
	s.PurgeOld(ctx)
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.PurgeOld(ctx)
		}
	}
}

// PurgeOld retention_days'ten eski mesajları siler.
func (s *Server) PurgeOld(ctx context.Context) {
	cutoff := time.Now().Add(-time.Duration(s.cfg.RetentionDays) * 24 * time.Hour)
	n, err := s.st.Purge(ctx, cutoff)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("saklama temizliği başarısız", "err", err.Error())
		}
		return
	}
	if n > 0 {
		s.log.Info("eski mesajlar silindi", "adet", n)
	}
}

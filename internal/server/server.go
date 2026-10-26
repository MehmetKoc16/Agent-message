// Package server, relay'in HTTP API'sini (docs/protokol.md §4) sunar.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	requestRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)
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

	touchMu         sync.Mutex
	touched         map[string]time.Time
	yonetimMu       sync.Mutex
	yonetimSessions map[string]yonetimSession
}

// New sunucuyu kurar. Log hiçbir zaman gövde, token ya da Authorization içermez.
func New(cfg config.Server, st *store.Store, log *slog.Logger) *Server {
	return &Server{
		cfg: cfg, st: st, log: log, hub: NewHub(),
		rate:            newRateLimiter(cfg.RatePerMin, cfg.RatePerDay),
		conns:           newConnLimiter(cfg.MaxConnsPerAgent),
		unauth:          newUnauthLimiter(cfg.UnauthPerMin, cfg.UnauthGlobalPerMin),
		authenticate:    st.Authenticate,
		touched:         map[string]time.Time{},
		yonetimSessions: map[string]yonetimSession{},
	}
}

// Close bekleyen tüm long-poll'ları uyandırır.
func (s *Server) Close() { s.hub.Close() }

// Handler tüm yolları (log ve çökme koruması dahil) döndürür.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.yonetimRoutes(mux)
	s.unreadRoutes(mux)
	mux.HandleFunc("GET /v1/me", s.authed(s.handleMe))
	mux.HandleFunc("GET /v1/rooms", s.authed(s.handleRooms))
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /v1/rooms/{room}/messages", s.room(s.handlePost))
	mux.HandleFunc("POST /v1/rooms/{room}/announcements", s.room(s.handlePost))
	mux.HandleFunc("GET /v1/rooms/{room}/messages", s.room(s.handleFetch))
	mux.HandleFunc("GET /v1/rooms/{room}/log", s.room(s.handleLog))
	mux.HandleFunc("GET /v1/rooms/{room}/threads/{thread}", s.room(s.handleThread))
	mux.HandleFunc("POST /v1/rooms/{room}/threads/{thread}/close", s.room(s.handleClose))
	mux.HandleFunc("GET /v1/rooms/{room}/agents", s.room(s.handleAgents))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "bulunamadi")
	})
	return s.logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ServeMux temizlemeden önce izleme yollarını tam eşleştir.
		if r.URL.Path == "/v1/izle" || strings.HasPrefix(r.URL.Path, "/v1/izle/") {
			handleIzle(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
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
	return s.authed(func(w http.ResponseWriter, r *http.Request, a store.Agent) {
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
	})
}

// authed oda gerektirmeyen kimlik ve eşzamanlı istek denetimidir.
// touch çağrısı işleyicide kalır: odalarda üyelik ve POST sınırından sonra çalışır.
func (s *Server) authed(h func(http.ResponseWriter, *http.Request, store.Agent)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := infoFrom(r.Context())
		token := bearer(r)
		// Başarısız sayılar ve eşzamanlı rezervasyonlar DB'den önce denetlenir.
		finish, ok := s.unauth.begin(s.clientIP(r), token)
		var a store.Agent
		var err error
		preauthenticated := false
		// Dolu başarısızlık kovası henüz tanınmayan geçerli token'ları
		// dışlayamaz. Yalnız biçimi geçerli token için ucuz özet araması yap.
		if !ok && authTokenRe.MatchString(token) {
			a, err = s.authenticate(r.Context(), token)
			if err != nil && !errors.Is(err, store.ErrUnauthorized) {
				s.internal(w, "kimlik doğrulama", err)
				return
			}
			if err == nil {
				ok, preauthenticated = true, true
				finish = func(bool) {}
			}
		}
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
		if !preauthenticated {
			a, err = s.authenticate(r.Context(), token)
		}
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
		h(w, r, a)
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, a store.Agent) {
	yonetici, err := s.st.IsYonetici(r.Context(), a.Name)
	if err != nil {
		s.internal(w, "yönetici denetimi", err)
		return
	}
	s.touch(a.Name)
	rooms, err := s.st.AgentRooms(r.Context(), a.Name)
	if err != nil {
		s.internal(w, "hesabın odaları", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Name     string   `json:"name"`
		Kind     string   `json:"kind"`
		Rooms    []string `json:"rooms"`
		Yonetici bool     `json:"yonetici"`
		MaxBody  int      `json:"max_body"`
	}{a.Name, a.Kind, rooms, yonetici && a.Kind == "insan", s.cfg.MaxBody})
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
	To           string `json:"to"`
	Type         string `json:"type"`
	Thread       string `json:"thread"`
	ReplyTo      *int64 `json:"reply_to"`
	Body         string `json:"body"`
	RequestID    string `json:"request_id,omitempty"`
	Announcement bool   `json:"-"`
	// "from" alanı bilerek yok: gövdede gelse de yok sayılır (kimlik token'dandır).
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	// JSON kaçışları gövdeyi büyütebilir; okuma sınırı bu yüzden max_body'nin birkaç katıdır.
	// Asıl gövde sınırı çözümlendikten sonra aşağıda uygulanır.
	r.Body = http.MaxBytesReader(w, r.Body, int64(4*s.cfg.MaxBody+1024))
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "govde_cok_buyuk")
			return
		}
		writeErr(w, http.StatusBadRequest, "gecersiz_json")
		return
	}
	if !utf8.Valid(raw) {
		writeErr(w, http.StatusBadRequest, "gecersiz_govde")
		return
	}
	var in postRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "gecersiz_json")
		return
	}
	in.Announcement = r.Pattern == "POST /v1/rooms/{room}/announcements"
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
	case in.RequestID != "" && !requestRe.MatchString(in.RequestID):
		writeErr(w, http.StatusBadRequest, "gecersiz_request_id")
	default:
		s.post(w, r, a, room, in)
	}
}

func (s *Server) post(w http.ResponseWriter, r *http.Request, a store.Agent, room string, in postRequest) {
	m, err := s.st.Post(r.Context(), store.PostInput{
		Room: room, From: a.Name, FromKind: a.Kind, To: in.To, Type: in.Type, Thread: in.Thread,
		ReplyTo: in.ReplyTo, Body: in.Body, RequestID: in.RequestID, Announcement: in.Announcement,
	})
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		writeErr(w, http.StatusUnauthorized, "yetkisiz")
	case errors.Is(err, store.ErrRoomForbidden):
		writeErr(w, http.StatusForbidden, "yasak")
	case errors.Is(err, store.ErrBadAnnouncement):
		writeErr(w, http.StatusBadRequest, "gecersiz_duyuru")
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
	case errors.Is(err, store.ErrRequestConflict):
		writeErr(w, http.StatusConflict, "istek_cakismasi")
	case err != nil:
		s.internal(w, "mesaj ekleme", err)
	default:
		s.hub.Notify(room) // commit'ten SONRA
		writeJSON(w, http.StatusCreated, m)
	}
}

func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	s.fetchMessages(w, r, a, room, false)
}

// roomReadAllowed bekleyen isteklerin SSH/web üzerinden yapılan iptalleri de
// görmesini sağlar. Kimlik önbelleğine veya isteğin ilk yetki kontrolüne güvenmez.
func (s *Server) roomReadAllowed(w http.ResponseWriter, r *http.Request, agent, room string) bool {
	a, err := s.authenticate(r.Context(), bearer(r))
	if errors.Is(err, store.ErrUnauthorized) || (err == nil && a.Name != agent) {
		writeErr(w, http.StatusUnauthorized, "yetkisiz")
		return false
	}
	if err != nil {
		s.internal(w, "bekleyen isteğin kimliği", err)
		return false
	}
	member, err := s.st.HasRoom(r.Context(), agent, room)
	if err != nil {
		s.internal(w, "bekleyen isteğin odası", err)
		return false
	}
	if !member {
		writeErr(w, http.StatusForbidden, "yasak")
		return false
	}
	return true
}

func (s *Server) fetchMessages(w http.ResponseWriter, r *http.Request, a store.Agent, room string, roomLog bool) {
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
	if roomLog && (q.Has("before") || q.Has("tail")) {
		var before int64
		if q.Has("before") {
			var err error
			before, err = strconv.ParseInt(q.Get("before"), 10, 64)
			if err != nil || before < 1 {
				writeErr(w, http.StatusBadRequest, "gecersiz_before")
				return
			}
		}
		if (q.Has("tail") && q.Get("tail") != "1") || (q.Has("before") && q.Has("tail")) || q.Has("after") || wait != 0 {
			writeErr(w, http.StatusBadRequest, "gecersiz_sayfa")
			return
		}
		msgs, err := s.st.RoomLogPage(r.Context(), room, before, maxFetch)
		if err != nil {
			s.internal(w, "geçmiş sayfası", err)
			return
		}
		if s.roomReadAllowed(w, r, a.Name, room) {
			writeJSON(w, http.StatusOK, map[string]any{"messages": msgs, "has_more": len(msgs) == maxFetch})
		}
		return
	}

	var timeout <-chan time.Time
	var accessCheck <-chan time.Time
	if wait > 0 {
		check := time.NewTicker(time.Second)
		defer check.Stop()
		accessCheck = check.C
		if !roomLog {
			defer s.hub.BeginWait(room, a.Name)()
		}
		t := time.NewTimer(time.Duration(wait) * time.Second)
		defer t.Stop()
		timeout = t.C
	}
	for {
		ch := s.hub.Chan(room) // sorgudan ÖNCE alınır (kaçırılan bildirim olmasın)
		if !s.roomReadAllowed(w, r, a.Name, room) {
			return
		}
		msgs, err := s.readMessages(r.Context(), room, a.Name, after, roomLog)
		if err != nil {
			if r.Context().Err() != nil {
				w.WriteHeader(499)
				return
			}
			s.internal(w, "mesaj okuma", err)
			return
		}
		if len(msgs) > 0 || wait == 0 {
			if s.roomReadAllowed(w, r, a.Name, room) {
				writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
			}
			return
		}
	awaitChange:
		for {
			select {
			case <-ch:
				break awaitChange
			case <-accessCheck:
				if !s.roomReadAllowed(w, r, a.Name, room) {
					return
				}
				// Yalnız yetkiyi denetle; mesaj gelene kadar sorguyu tekrar çalıştırma.
			case <-timeout:
				s.finalMessages(w, r, room, a.Name, after, roomLog)
				return
			case <-s.hub.Done():
				s.finalMessages(w, r, room, a.Name, after, roomLog)
				return
			case <-r.Context().Done():
				w.WriteHeader(499) // istemci koptu; bekleme bırakıldı
				return
			}
		}
	}
}

// finalMessages bekleme bitince (süre ya da kapanış) son bir kez bakar: süre dolarken gelen
// mesaj select'te kaybolmasın.
func (s *Server) finalMessages(w http.ResponseWriter, r *http.Request, room, agent string, after int64, roomLog bool) {
	msgs, err := s.readMessages(r.Context(), room, agent, after, roomLog)
	if err != nil {
		if r.Context().Err() != nil {
			w.WriteHeader(499)
			return
		}
		s.internal(w, "mesaj okuma", err)
		return
	}
	if s.roomReadAllowed(w, r, agent, room) {
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	}
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
	err := s.st.CloseThreadFor(r.Context(), room, id, a.Name)
	if errors.Is(err, store.ErrUnauthorized) {
		writeErr(w, http.StatusUnauthorized, "yetkisiz")
		return
	} else if errors.Is(err, store.ErrRoomForbidden) || errors.Is(err, store.ErrNotParty) {
		writeErr(w, http.StatusForbidden, "yasak")
		return
	} else if errors.Is(err, store.ErrBadAnnouncement) {
		writeErr(w, http.StatusBadRequest, "gecersiz_duyuru")
		return
	} else if errors.Is(err, store.ErrThreadNotFound) {
		writeErr(w, http.StatusNotFound, "konusma_yok")
		return
	} else if err != nil {
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

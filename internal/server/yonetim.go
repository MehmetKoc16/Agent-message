package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/MehmetKoc16/Agent-message/internal/admin"
	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

type yonetimSession struct {
	hash, secretHash [32]byte
	expires          time.Time
}

// Her sekme kendi oturumunu alır; yeni giriş eski sekmeyi düşürmez. En yeni
// maxYonetimSessions oturum geçerli kalır.
const maxYonetimSessions = 5

// liveYonetimSessions süresi dolanları atar; çağıran yonetimMu'yu tutar.
func (s *Server) liveYonetimSessions(name string) []yonetimSession {
	live := []yonetimSession{}
	for _, session := range s.yonetimSessions[name] {
		if s.st.Now().Before(session.expires) {
			live = append(live, session)
		}
	}
	if len(live) == 0 {
		delete(s.yonetimSessions, name)
	} else {
		s.yonetimSessions[name] = live
	}
	return live
}

func (s *Server) yonetimRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/yonetim/oturum", s.yonetim(s.handleYonetimLogin, true))
	m.HandleFunc("GET /v1/yonetim/hesaplar", s.yonetim(s.handleYonetimList, false))
	m.HandleFunc("POST /v1/yonetim/hesaplar", s.yonetim(s.handleYonetimCreate, false))
	m.HandleFunc("POST /v1/yonetim/odalar", s.yonetim(s.handleCreateRoom, false))
	m.HandleFunc("GET /v1/yonetim/odalar", s.yonetim(s.handleManagementRooms, false))
	m.HandleFunc("POST /v1/yonetim/hesaplar/{ad}/odalar", s.yonetim(s.handleYonetimChange, false))
	m.HandleFunc("DELETE /v1/yonetim/hesaplar/{ad}/odalar/{oda}", s.yonetim(s.handleYonetimChange, false))
	m.HandleFunc("POST /v1/yonetim/hesaplar/{ad}/iptal", s.yonetim(s.handleYonetimChange, false))
	m.HandleFunc("GET /v1/yonetim/kayit", s.yonetim(s.handleYonetimKayit, false))
}
func (s *Server) yonetim(h func(http.ResponseWriter, *http.Request, store.Agent), login bool) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, a store.Agent) {
		yes, err := s.st.IsYonetici(r.Context(), a.Name)
		if err != nil {
			s.internal(w, "yönetici denetimi", err)
			return
		}
		if !yes || a.Kind != "insan" {
			writeErr(w, 403, "yalniz_yonetici")
			return
		}
		if !login {
			secret, _, err := s.st.TOTPState(r.Context(), a.Name)
			if err != nil {
				s.internal(w, "oturum denetimi", err)
				return
			}
			got := sha256.Sum256([]byte(r.Header.Get("X-Yonetim-Oturum")))
			sh := sha256.Sum256([]byte(secret))
			ok := false
			s.yonetimMu.Lock()
			for _, session := range s.liveYonetimSessions(a.Name) {
				if subtle.ConstantTimeCompare(got[:], session.hash[:]) == 1 && sh == session.secretHash {
					ok = true
				}
			}
			s.yonetimMu.Unlock()
			if !ok || len(r.Header.Values("X-Yonetim-Oturum")) != 1 {
				writeErr(w, 401, "yonetim_oturumu_gerekli")
				return
			}
		}
		if r.Method != "GET" && !s.rate.Allow(a.Name) {
			writeErr(w, 429, "hiz_siniri")
			return
		}
		s.touch(a.Name)
		h(w, r, a)
	})
}
func yonetimBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			writeErr(w, 413, "govde_cok_buyuk")
		} else {
			writeErr(w, 400, "gecersiz_json")
		}
		return false
	}
	if !utf8.Valid(raw) {
		writeErr(w, 400, "gecersiz_govde")
		return false
	}
	if json.Unmarshal(raw, v) != nil {
		writeErr(w, 400, "gecersiz_json")
		return false
	}
	return true
}
func (s *Server) handleYonetimLogin(w http.ResponseWriter, r *http.Request, a store.Agent) {
	var in struct {
		Kod string `json:"kod"`
	}
	if !yonetimBody(w, r, &in) {
		return
	}
	s.yonetimMu.Lock()
	defer s.yonetimMu.Unlock()
	now := s.st.Now()
	secret, ok, locked, err := s.st.AttemptTOTP(r.Context(), a.Name, in.Kod, admin.MatchTOTP)
	if err != nil {
		s.internal(w, "TOTP denetimi", err)
		return
	}
	if !ok {
		status, code := 401, "kod_gecersiz"
		if locked {
			status, code = 429, "cok_fazla_deneme"
		}
		if s.audit(w, r, a, "oturum_reddedildi", a.Name, "") {
			writeErr(w, status, code)
		}
		return
	}
	token, err := admin.NewToken()
	if err != nil {
		s.internal(w, "oturum üretimi", err)
		return
	}
	expires := now.Add(10 * time.Minute)
	if !s.audit(w, r, a, "oturum_acildi", a.Name, "") {
		return
	}
	sessions := append(s.liveYonetimSessions(a.Name), yonetimSession{sha256.Sum256([]byte(token)), sha256.Sum256([]byte(secret)), expires})
	s.yonetimSessions[a.Name] = sessions[max(0, len(sessions)-maxYonetimSessions):]
	writeJSON(w, 200, map[string]string{"oturum": token, "bitis": expires.UTC().Format(time.RFC3339)})
}
func (s *Server) audit(w http.ResponseWriter, r *http.Request, a store.Agent, islem, hedef, ayrinti string) bool {
	if err := s.st.AddKayit(r.Context(), a.Name, islem, hedef, ayrinti); err != nil {
		s.internal(w, "denetim kaydı", err)
		return false
	}
	return true
}
func (s *Server) handleYonetimList(w http.ResponseWriter, r *http.Request, a store.Agent) {
	rows, err := s.st.ListAgents(r.Context())
	if err != nil {
		s.internal(w, "hesap listesi", err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		rooms := v.Rooms
		if rooms == nil {
			rooms = []string{}
		}
		out = append(out, map[string]any{"name": v.Name, "kind": v.Kind, "rooms": rooms, "last_seen": v.LastSeen, "revoked": v.RevokedAt != "", "yonetici": v.Yonetici})
	}
	if s.audit(w, r, a, "hesaplar_listelendi", "", "") {
		writeJSON(w, 200, out)
	}
}
func (s *Server) handleYonetimCreate(w http.ResponseWriter, r *http.Request, a store.Agent) {
	var in struct {
		Ad     string   `json:"ad"`
		Tur    string   `json:"tur"`
		Odalar []string `json:"odalar"`
	}
	if !yonetimBody(w, r, &in) {
		return
	}
	if !config.NameRe.MatchString(in.Ad) {
		writeErr(w, 400, "gecersiz_ad")
		return
	}
	if in.Tur != "insan" && in.Tur != "ajan" {
		writeErr(w, 400, "gecersiz_tur")
		return
	}
	if len(in.Odalar) == 0 {
		writeErr(w, 400, "gecersiz_oda")
		return
	}
	for _, oda := range in.Odalar {
		if !config.NameRe.MatchString(oda) {
			writeErr(w, 400, "gecersiz_oda")
			return
		}
	}
	token, err := admin.NewToken()
	if err != nil {
		s.internal(w, "hesap üretimi", err)
		return
	}
	err = s.st.AddAgentWithKayit(r.Context(), in.Ad, in.Tur, store.HashToken(token), in.Odalar, a.Name, true)
	if errors.Is(err, store.ErrUnauthorized) {
		writeErr(w, 403, "yalniz_yonetici")
		return
	}
	if errors.Is(err, store.ErrExists) {
		writeErr(w, 409, "zaten_var")
		return
	}
	if errors.Is(err, store.ErrUnknownRoom) {
		writeErr(w, 404, "oda_yok")
		return
	}
	if err != nil {
		s.internal(w, "hesap açma", err)
		return
	}
	writeJSON(w, 201, map[string]string{"ad": in.Ad, "token": token})
}
func (s *Server) handleYonetimChange(w http.ResponseWriter, r *http.Request, a store.Agent) {
	ad := r.PathValue("ad")
	if !config.NameRe.MatchString(ad) {
		writeErr(w, 400, "gecersiz_ad")
		return
	}
	yes, err := s.st.IsYonetici(r.Context(), ad)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "hesap_yok")
		return
	}
	if err != nil {
		s.internal(w, "hedef denetimi", err)
		return
	}
	if yes && r.Pattern == "POST /v1/yonetim/hesaplar/{ad}/iptal" {
		writeErr(w, 403, "yoneticiye_dokunulamaz")
		return
	}
	oda := r.PathValue("oda")
	islem := "hesap_iptal"
	if r.Pattern == "POST /v1/yonetim/hesaplar/{ad}/odalar" {
		var in struct {
			Oda string `json:"oda"`
		}
		if !yonetimBody(w, r, &in) {
			return
		}
		oda = in.Oda
		islem = "odaya_eklendi"
	} else if r.Method == "DELETE" {
		islem = "odadan_cikarildi"
	}
	if islem != "hesap_iptal" && !config.NameRe.MatchString(oda) {
		writeErr(w, 400, "gecersiz_oda")
		return
	}
	err = s.st.ChangeAccount(r.Context(), a.Name, islem, ad, oda, "", true)
	if errors.Is(err, store.ErrUnauthorized) {
		writeErr(w, 403, "yalniz_yonetici")
		return
	}
	if errors.Is(err, store.ErrYoneticiProtected) {
		writeErr(w, 403, "yoneticiye_dokunulamaz")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "hesap_yok")
		return
	}
	if err != nil {
		s.internal(w, "hesap değişikliği", err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) handleYonetimKayit(w http.ResponseWriter, r *http.Request, a store.Agent) {
	limit := 100
	if v := r.URL.Query().Get("son"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			writeErr(w, 400, "gecersiz_son")
			return
		}
		limit = n
	}
	if !s.audit(w, r, a, "kayit_listelendi", "", "") {
		return
	}
	rows, err := s.st.ListKayit(r.Context(), limit)
	if err != nil {
		s.internal(w, "kayıt listesi", err)
		return
	}
	writeJSON(w, 200, rows)
}

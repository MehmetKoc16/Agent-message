package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

// Yönetici silme uçları yonetimRoutes'ta kayıtlıdır: TOTP oturumu ister, silme
// kalıcıdır ve mesaj gövdesi denetim kaydına yazılmaz.
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request, a store.Agent) {
	room := r.PathValue("oda")
	if !config.NameRe.MatchString(room) {
		writeErr(w, 400, "gecersiz_oda")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, 400, "gecersiz_mesaj")
		return
	}
	thread, err := s.st.DeleteMessage(r.Context(), a.Name, room, id)
	if s.deletionFailed(w, err, "mesaj silme") {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "thread": thread})
}

func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request, a store.Agent) {
	room, thread := r.PathValue("oda"), r.PathValue("konusma")
	if !config.NameRe.MatchString(room) {
		writeErr(w, 400, "gecersiz_oda")
		return
	}
	if !threadRe.MatchString(thread) {
		writeErr(w, 400, "gecersiz_thread")
		return
	}
	n, err := s.st.DeleteThread(r.Context(), a.Name, room, thread)
	if s.deletionFailed(w, err, "konuşma silme") {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "silinen": n})
}

func (s *Server) deletionFailed(w http.ResponseWriter, err error, what string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrUnauthorized):
		writeErr(w, 403, "yalniz_yonetici")
	case errors.Is(err, store.ErrUnknownRoom):
		writeErr(w, 404, "oda_yok")
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "mesaj_yok")
	default:
		s.internal(w, what, err)
	}
	return true
}

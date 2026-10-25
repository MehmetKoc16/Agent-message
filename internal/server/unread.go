package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func (s *Server) unreadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/unread", s.authed(s.handleUnread))
	mux.HandleFunc("POST /v1/rooms/{room}/threads/{thread}/read", s.room(s.handleReadThread))
	mux.HandleFunc("GET /v1/rooms/{room}/threads/{thread}/log", s.room(s.handleThreadLog))
}

func (s *Server) handleUnread(w http.ResponseWriter, r *http.Request, a store.Agent) {
	if a.Kind != "insan" {
		writeErr(w, 403, "yalniz_insan")
		return
	}
	rows, err := s.st.ThreadSummaries(r.Context(), a.Name)
	if err != nil {
		s.internal(w, "okuma özeti", err)
		return
	}
	writeJSON(w, 200, map[string]any{"threads": rows})
}

func (s *Server) handleReadThread(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	if a.Kind != "insan" {
		writeErr(w, 403, "yalniz_insan")
		return
	}
	thread := r.PathValue("thread")
	if !threadRe.MatchString(thread) {
		writeErr(w, 400, "gecersiz_thread")
		return
	}
	var in struct {
		LastID int64 `json:"last_id"`
	}
	if !yonetimBody(w, r, &in) {
		return
	}
	err := s.st.MarkThreadRead(r.Context(), a.Name, room, thread, in.LastID)
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		writeErr(w, 401, "yetkisiz")
	case errors.Is(err, store.ErrRoomForbidden):
		writeErr(w, 403, "yasak")
	case errors.Is(err, store.ErrBadRead):
		writeErr(w, 400, "gecersiz_okuma")
	case err != nil:
		s.internal(w, "okuma kaydı", err)
	default:
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func (s *Server) handleThreadLog(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	if a.Kind != "insan" {
		writeErr(w, 403, "yalniz_insan")
		return
	}
	thread := r.PathValue("thread")
	if !threadRe.MatchString(thread) {
		writeErr(w, 400, "gecersiz_thread")
		return
	}
	before := int64(0)
	if r.URL.Query().Has("before") {
		n, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		if err != nil || n < 1 {
			writeErr(w, 400, "gecersiz_before")
			return
		}
		before = n
	}
	rows, err := s.st.ThreadLogPage(r.Context(), room, thread, before, maxFetch)
	if err != nil {
		s.internal(w, "konuşma sayfası", err)
		return
	}
	if s.roomReadAllowed(w, r, a.Name, room) {
		writeJSON(w, 200, map[string]any{"messages": rows, "has_more": len(rows) == maxFetch})
	}
}

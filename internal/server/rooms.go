package server

import (
	"errors"
	"net/http"

	"github.com/MehmetKoc16/Agent-message/internal/config"
	"github.com/MehmetKoc16/Agent-message/internal/store"
)

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request, a store.Agent) {
	rooms, err := s.st.RoomsForAgent(r.Context(), a.Name)
	if err != nil {
		s.internal(w, "odalar", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

func (s *Server) handleManagementRooms(w http.ResponseWriter, r *http.Request, a store.Agent) {
	rooms, err := s.st.AllRooms(r.Context())
	if err != nil {
		s.internal(w, "yönetim odaları", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request, a store.Agent) {
	var in struct {
		Title   string   `json:"baslik"`
		Members []string `json:"uyeler"`
	}
	if !yonetimBody(w, r, &in) {
		return
	}
	if len(in.Members) > 100 {
		writeErr(w, 400, "gecersiz_uye")
		return
	}
	for _, name := range in.Members {
		if !config.NameRe.MatchString(name) {
			writeErr(w, 400, "gecersiz_uye")
			return
		}
	}
	room, err := s.st.CreateRoom(r.Context(), a.Name, in.Title, in.Members)
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		writeErr(w, 403, "yalniz_yonetici")
	case errors.Is(err, store.ErrBadRoomTitle):
		writeErr(w, 400, "gecersiz_baslik")
	case errors.Is(err, store.ErrBadMember):
		writeErr(w, 400, "gecersiz_uye")
	case err != nil:
		s.internal(w, "oda oluşturma", err)
	default:
		writeJSON(w, http.StatusCreated, room)
	}
}

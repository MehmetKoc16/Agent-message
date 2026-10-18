package server

import (
	"context"
	"embed"
	"net/http"

	"github.com/MehmetKoc16/Agent-message/internal/store"
)

//go:embed izle/index.html izle/izle.js izle/izle.css
var izleFiles embed.FS

const izleCSP = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func handleIzle(w http.ResponseWriter, r *http.Request) {
	files := map[string][2]string{
		"/v1/izle":          {"izle/index.html", "text/html; charset=utf-8"},
		"/v1/izle/izle.js":  {"izle/izle.js", "text/javascript; charset=utf-8"},
		"/v1/izle/izle.css": {"izle/izle.css", "text/css; charset=utf-8"},
	}
	f, ok := files[r.URL.Path]
	if !ok || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		writeErr(w, http.StatusNotFound, "bulunamadi")
		return
	}
	b, err := izleFiles.ReadFile(f[0])
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "sunucu_hatasi")
		return
	}
	h := w.Header()
	h.Set("Content-Type", f[1])
	h.Set("Content-Security-Policy", izleCSP)
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(b)
	}
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request, a store.Agent, room string) {
	if a.Kind != "insan" {
		writeErr(w, http.StatusForbidden, "yalniz_insan")
		return
	}
	s.fetchMessages(w, r, a, room, true)
}

func (s *Server) readMessages(ctx context.Context, room, agent string, after int64, roomLog bool) ([]store.Message, error) {
	if roomLog {
		return s.st.RoomLog(ctx, room, after, maxFetch)
	}
	return s.st.Fetch(ctx, room, agent, after, maxFetch)
}

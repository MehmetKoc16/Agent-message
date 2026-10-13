package server

import (
	"context"
	"net/http"
	"time"
	"unicode/utf8"
)

type ctxKey struct{}

// reqInfo log satırına yazılacak, işleyicilerin doldurduğu bilgidir.
type reqInfo struct{ agent string }

func infoFrom(ctx context.Context) *reqInfo {
	if v, ok := ctx.Value(ctxKey{}).(*reqInfo); ok {
		return v
	}
	return &reqInfo{}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status, r.wrote = http.StatusOK, true
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logging her isteği tek satır loglar: yöntem, yol (query YOK), ajan, durum, süre.
// Gövde, token ve Authorization başlığı ASLA loglanmaz. Çökmeleri de yakalar.
func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &reqInfo{}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("işleyici çöktü")
				if !rec.wrote {
					writeErr(rec, http.StatusInternalServerError, "sunucu_hatasi")
				}
			}
			path := r.URL.Path
			path = truncUTF8(path, 200)
			s.log.Info("istek", "method", r.Method, "path", path, "agent", info.agent,
				"status", rec.status, "ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), ctxKey{}, info)))
	})
}

// truncUTF8 s'yi en çok n bayta keser; çok baytlı bir karakteri ortasından bölmez.
func truncUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

package server

import (
	"crypto/sha256"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxUnauthIPs = 10000

type unauthEntry struct {
	hist    []time.Time
	pending int
}
type unauthLimiter struct {
	mu                        sync.Mutex
	ipMax, globalMax, pending int
	ips                       map[string]*unauthEntry
	global                    []time.Time
	known                     map[[32]byte]time.Time
	now                       func() time.Time
	cleaned                   time.Time
}

func newUnauthLimiter(ip, global int) *unauthLimiter {
	return &unauthLimiter{ipMax: ip, globalMax: global, ips: make(map[string]*unauthEntry), known: make(map[[32]byte]time.Time), now: time.Now}
}

func recent(h []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(h) && !h[i].After(cutoff) {
		i++
	}
	return h[i:]
}

// begin yalnız başarısızlıkları sayar; geçici rezervasyon paralel DB yükünü de sınırlar.
// Bilinen token özeti yalnız ön sınırı atlar; her istekte DB iptal kontrolü sürer.
func (l *unauthLimiter) begin(ip, token string) (func(bool), bool) {
	l.mu.Lock()
	now := l.now()
	cutoff := now.Add(-time.Minute)
	l.global = recent(l.global, cutoff)
	if now.Sub(l.cleaned) >= time.Minute {
		for k, e := range l.ips {
			e.hist = recent(e.hist, cutoff)
			if len(e.hist) == 0 && e.pending == 0 {
				delete(l.ips, k)
			}
		}
		for k, expiry := range l.known {
			if !expiry.After(now) {
				delete(l.known, k)
			}
		}
		l.cleaned = now
	}
	hash := sha256.Sum256([]byte(token))
	known := token != "" && l.known[hash].After(now)
	e := l.ips[ip]
	if e != nil {
		e.hist = recent(e.hist, cutoff)
	}
	if !known && (len(l.global)+l.pending >= l.globalMax || (e != nil && len(e.hist)+e.pending >= l.ipMax) || (e == nil && len(l.ips) >= maxUnauthIPs)) {
		l.mu.Unlock()
		return nil, false
	}
	// Bilinen ajanın yeni IP'si dolu haritayı büyütmez.
	if e == nil && len(l.ips) < maxUnauthIPs {
		e = &unauthEntry{}
		l.ips[ip] = e
	}
	if !known {
		e.pending++
		l.pending++
	}
	l.mu.Unlock()
	var once sync.Once
	return func(failed bool) {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if !known {
				e.pending--
				l.pending--
			}
			if failed {
				delete(l.known, hash)
				now := l.now()
				l.global = recent(l.global, now.Add(-time.Minute))
				if len(l.global) < l.globalMax {
					l.global = append(l.global, now)
				}
				if e != nil {
					e.hist = recent(e.hist, now.Add(-time.Minute))
					if len(e.hist) < l.ipMax {
						e.hist = append(e.hist, now)
					}
				}
			}
		})
	}, true
}

func (l *unauthLimiter) recognize(token string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	h := sha256.Sum256([]byte(token))
	if _, ok := l.known[h]; ok || len(l.known) < maxUnauthIPs {
		l.known[h] = l.now().Add(time.Hour)
	}
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustForwardedFor && (s.cfg.Listen == "systemd" || strings.HasPrefix(s.cfg.Listen, "unix:")) {
		values := r.Header.Values("X-Forwarded-For")
		if len(values) > 0 {
			parts := strings.Split(values[len(values)-1], ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return "unknown" // Unix bağlantısında başlık yoksa tek ortak kova; keyfi metin haritayı büyütmez.
}

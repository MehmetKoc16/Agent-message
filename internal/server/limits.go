package server

import (
	"crypto/sha256"
	"net/netip"
	"sync"
	"time"
)

// unauthKey kimliksiz sınırlayıcının sayaç anahtarıdır. IPv6 adresleri /64
// önekine indirilir: tek abone /64'ün içinde adres değiştirerek kovayı
// aşamaz. IPv4 ve IPv4-mapped IPv6 tam IPv4 adresidir; çözülemeyen değer
// ("unknown") aynen kalır. clientIP'nin döndürdüğü değer (log vb.) değişmez.
func unauthKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}
	p, err := addr.WithZone("").Prefix(64)
	if err != nil {
		return ip
	}
	return p.String()
}

// isKnown token son bir saatte doğrulanmışsa true verir (recognize).
func (l *unauthLimiter) isKnown(token string) bool {
	if token == "" {
		return false
	}
	h := sha256.Sum256([]byte(token))
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.known[h].After(l.now())
}

// rateLimiter ajan başına POST sayısını kayan pencerede (dakika ve gün) sınırlar.
// Yalnız kabul edilen istekler sayılır; reddedilenler pencereyi uzatmaz.
type rateLimiter struct {
	mu     sync.Mutex
	perMin int
	perDay int
	now    func() time.Time
	hist   map[string][]time.Time
}

func newRateLimiter(perMin, perDay int) *rateLimiter {
	return &rateLimiter{perMin: perMin, perDay: perDay, now: time.Now, hist: map[string][]time.Time{}}
}

// Allow isteği sayar; sınır aşılıyorsa false verir.
func (l *rateLimiter) Allow(agent string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	h := l.hist[agent]
	dayAgo := now.Add(-24 * time.Hour)
	i := 0
	for i < len(h) && !h[i].After(dayAgo) {
		i++
	}
	h = h[i:]
	if len(h) >= l.perDay {
		l.hist[agent] = h
		return false
	}
	minAgo := now.Add(-time.Minute)
	n := 0
	for j := len(h) - 1; j >= 0 && h[j].After(minAgo); j-- {
		n++
	}
	if n >= l.perMin {
		l.hist[agent] = h
		return false
	}
	l.hist[agent] = append(h, now)
	return true
}

// connLimiter ajan başına eşzamanlı istek sayısını sınırlar.
type connLimiter struct {
	mu  sync.Mutex
	max int
	n   map[string]int
}

func newConnLimiter(max int) *connLimiter { return &connLimiter{max: max, n: map[string]int{}} }

// Acquire yer varsa bırakma işlevini döndürür.
func (c *connLimiter) Acquire(agent string) (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n[agent] >= c.max {
		return nil, false
	}
	c.n[agent]++
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.n[agent]--; c.n[agent] <= 0 {
				delete(c.n, agent)
			}
		})
	}, true
}

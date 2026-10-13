package server

import (
	"sync"
	"time"
)

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

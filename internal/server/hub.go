package server

import "sync"

// Hub, long-poll bekleyenleri uyandıran oda başına yayın düzenidir.
//
// Kaçırılan bildirim olmaması için sıra şudur: bekleyen ÖNCE Chan(room) ile
// kanalı alır, SONRA veritabanını sorgular, boşsa kanalda bekler. Yazan ise
// mesajı commit ettikten SONRA Notify(room) çağırır (kanalı kapatır, yenisini
// koyar). Sorgudan sonra commit olan mesaj, kanal alındıktan sonra olduğu için
// Notify onu mutlaka kapatır; sorgudan önce commit olan mesajı da sorgu görür.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]*roomHub
	done  chan struct{}
	once  sync.Once
}

type roomHub struct {
	ch      chan struct{}
	waiting map[string]int
}

// NewHub boş bir Hub döndürür.
func NewHub() *Hub {
	return &Hub{rooms: map[string]*roomHub{}, done: make(chan struct{})}
}

func (h *Hub) room(name string) *roomHub {
	r := h.rooms[name]
	if r == nil {
		r = &roomHub{ch: make(chan struct{}), waiting: map[string]int{}}
		h.rooms[name] = r
	}
	return r
}

// Chan odanın bir sonraki yayınında kapanacak kanalı verir.
func (h *Hub) Chan(room string) <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.room(room).ch
}

// Notify odadaki tüm bekleyenleri uyandırır.
func (h *Hub) Notify(room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.room(room)
	close(r.ch)
	r.ch = make(chan struct{})
}

// BeginWait ajanı "bekliyor" olarak işaretler; dönen işlev işareti kaldırır.
func (h *Hub) BeginWait(room, agent string) func() {
	h.mu.Lock()
	h.room(room).waiting[agent]++
	h.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			w := h.room(room).waiting
			if w[agent]--; w[agent] <= 0 {
				delete(w, agent)
			}
		})
	}
}

// Waiting odada aktif long-poll'u olan ajanları verir.
func (h *Hub) Waiting(room string) map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]bool{}
	for a := range h.room(room).waiting {
		out[a] = true
	}
	return out
}

// Done kapanışta kapanan kanaldır; bekleyenler bununla uyanır.
func (h *Hub) Done() <-chan struct{} { return h.done }

// Close tüm bekleyenleri uyandırır (kapanış).
func (h *Hub) Close() { h.once.Do(func() { close(h.done) }) }

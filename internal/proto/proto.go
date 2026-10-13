// Package proto, sunucu ve istemcinin paylaştığı JSON tiplerini tanımlar.
package proto

// Message API'de görünen mesajdır.
type Message struct {
	ID        int64  `json:"id"`
	Room      string `json:"room"`
	From      string `json:"from"`
	To        string `json:"to"`
	Type      string `json:"type"`
	Thread    string `json:"thread"`
	ReplyTo   *int64 `json:"reply_to"`
	Turn      int    `json:"turn"`
	MaxTurns  int    `json:"max_turns"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// Thread konuşma durumudur.
type Thread struct {
	Room      string `json:"room"`
	ID        string `json:"id"`
	Status    string `json:"status"` // acik | kilitli | kapali
	Turns     int    `json:"turns"`
	TurnLimit int    `json:"max_turns"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

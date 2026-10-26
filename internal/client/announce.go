package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/flagx"
	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

func (r *runner) cmdAnnounce(ctx context.Context, fs *flag.FlagSet, args []string) int {
	room := fs.String("room", "", "hedef oda kimliği")
	title := fs.String("room-title", "Update Messages", "hedef oda başlığı")
	newTopic := fs.Bool("new-topic", false, "kullanımı değiştiren özellik için yeni konu")
	requestID := fs.String("request-id", "", "güvenli yeniden gönderim için sabit istek kimliği")
	pos, err := flagx.Parse(fs, args)
	if err != nil {
		return ExitError
	}
	if len(pos) != 1 {
		return r.fail(ExitError, "kullanım: relay announce [--room-title <başlık>|--room <kimlik>] [--new-topic] [--request-id <id>] <metin|->")
	}
	body := pos[0]
	if body == "-" {
		data, err := io.ReadAll(io.LimitReader(r.env.Stdin, maxStdin+1))
		if err != nil || int64(len(data)) > maxStdin {
			return r.fail(ExitError, "stdin okunamadı veya mesaj çok büyük")
		}
		body = strings.ToValidUTF8(string(data), "�")
	}
	if strings.TrimSpace(body) == "" {
		return r.fail(ExitError, "duyuru metni boş olamaz")
	}
	if name, found := r.scanSecrets(body); found {
		return r.fail(ExitSecret, "GÖNDERİLMEDİ: mesajda sır deseni bulundu (%s).", name)
	}
	var rooms struct {
		Rooms []struct {
			Name  string `json:"name"`
			Title string `json:"title"`
		} `json:"rooms"`
	}
	if err := r.api.do(ctx, http.MethodGet, "/v1/rooms", nil, &rooms, 30*time.Second); err != nil {
		return r.apiFail(err)
	}
	var matches []string
	for _, candidate := range rooms.Rooms {
		if (*room != "" && candidate.Name == *room) || (*room == "" && strings.EqualFold(candidate.Title, *title)) {
			matches = append(matches, candidate.Name)
		}
	}
	if len(matches) != 1 {
		return r.fail(ExitError, "hedef oda bulunamadı veya başlık birden fazla odaya ait; üyeliğini denetle ve gerekirse --room ile kimlik seç")
	}
	if *requestID == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return r.fail(ExitError, "istek kimliği üretilemedi")
		}
		*requestID = hex.EncodeToString(id[:])
	}
	endpoint := "announcements"
	if *newTopic {
		endpoint = "messages"
	}
	in := sendInput{Type: "bilgi", Body: body, RequestID: *requestID}
	var m proto.Message
	path := "/v1/rooms/" + url.PathEscape(matches[0]) + "/" + endpoint
	if err := r.api.do(ctx, http.MethodPost, path, in, &m, 30*time.Second); err != nil {
		return r.apiFail(err)
	}
	fmt.Fprintf(r.env.Stdout, "duyuruldu id=%d thread=%s request_id=%s\n", m.ID, m.Thread, *requestID)
	return ExitOK
}

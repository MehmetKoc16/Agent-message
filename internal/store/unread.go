package store

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"unicode/utf8"
)

var ErrBadRead = errors.New("okuma imleci bu konuşmadaki bir mesaj olmalı")

type ThreadSummary struct {
	Room           string `json:"room"`
	Thread         string `json:"thread"`
	Title          string `json:"title"`
	Total          int    `json:"total"`
	LastID         int64  `json:"last_id"`
	LastIncomingID int64  `json:"last_incoming_id"`
	ReadID         int64  `json:"read_id"`
	// Unread okuma imlecinden sonra gelen, başkalarının yazdığı mesaj sayısıdır.
	Unread int `json:"unread"`
}

// MaxSummaries tek özet yanıtındaki konuşma sayısı tavanıdır.
const MaxSummaries = 200

// maxTitleBytes özet başlığının bayt tavanıdır (180 karakter 720 bayta varabilir).
const maxTitleBytes = 256

// ThreadSummaries insanın oda günlüğüyle aynı görünürlüğü kullanır. Üye olunmayan
// odalar ve iptal edilmiş hesaplar sonuç üretmez; CLI inbox imlecine dokunulmaz.
// En çok MaxSummaries konuşma döner: önce okunmamışlar, sonra en yeniler; böylece
// sınır okunmamış işaretini ancak MaxSummaries'ten fazla okunmamış konuşmada düşürür.
// truncated, sınır yüzünden dışarıda kalan konuşma olduğunu bildirir. Sonuç en yeni
// mesaja göre azalan sıradadır.
func (s *Store) ThreadSummaries(ctx context.Context, agent string) (out []ThreadSummary, truncated bool, err error) {
	// Başlık alt sorgusu yalnız seçilen satırlar için çalışsın diye dışarıdadır.
	rows, err := s.db.QueryContext(ctx, `SELECT g.room,g.thread,
 (SELECT substr(first.body,1,180) FROM messages first WHERE first.room=g.room AND first.thread=g.thread ORDER BY first.id LIMIT 1),
 g.total,g.last_id,g.last_incoming,g.read_id,g.unread
 FROM (SELECT m.room AS room,m.thread AS thread,COUNT(*) AS total,MAX(m.id) AS last_id,
  MAX(CASE WHEN m.from_agent<>? THEN m.id ELSE 0 END) AS last_incoming,COALESCE(r.last_id,0) AS read_id,
  SUM(CASE WHEN m.from_agent<>? AND m.id>COALESCE(r.last_id,0) THEN 1 ELSE 0 END) AS unread
  FROM messages m JOIN agent_rooms ar ON ar.room=m.room AND ar.agent=?
  JOIN agents a ON a.name=ar.agent AND a.kind='insan' AND a.revoked_at IS NULL
  LEFT JOIN thread_reads r ON r.agent=ar.agent AND r.room=m.room AND r.thread=m.thread
  GROUP BY m.room,m.thread ORDER BY unread>0 DESC,last_id DESC LIMIT ?) g
 ORDER BY g.unread>0 DESC,g.last_id DESC`, agent, agent, agent, MaxSummaries+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out = []ThreadSummary{}
	for rows.Next() {
		if len(out) == MaxSummaries { // fazladan istenen satır: yalnız kırpma işareti
			truncated = true
			break
		}
		var row ThreadSummary
		if err := rows.Scan(&row.Room, &row.Thread, &row.Title, &row.Total, &row.LastID, &row.LastIncomingID, &row.ReadID, &row.Unread); err != nil {
			return nil, false, err
		}
		row.Title = cutUTF8(row.Title, maxTitleBytes)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	slices.SortFunc(out, func(a, b ThreadSummary) int { return cmp.Compare(b.LastID, a.LastID) })
	return out, truncated, nil
}

// cutUTF8 s'yi en çok n bayta, karakter ortasından bölmeden kısaltır.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// MarkThreadRead yazarın yetkisini ve imlecin konuşmaya ait olduğunu aynı
// transaction'da doğrular. Başka sekmedeki eski imleç okuma durumunu geri alamaz.
func (s *Store) MarkThreadRead(ctx context.Context, agent, room, thread string, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	kind, err := requireRoomMemberTx(ctx, tx, agent, room)
	if err != nil {
		return err
	}
	if kind != "insan" {
		return ErrUnauthorized
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE room=? AND thread=? AND id=?`, room, thread, id).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrBadRead
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO thread_reads(agent,room,thread,last_id) VALUES (?,?,?,?) ON CONFLICT(agent,room,thread) DO UPDATE SET last_id=MAX(last_id,excluded.last_id)`, agent, room, thread, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ThreadLogPage RoomLogPage'in tek konuşmaya daraltılmış halidir.
func (s *Store) ThreadLogPage(ctx context.Context, room, thread string, before int64, limit int) ([]Message, bool, error) {
	where := `WHERE m.room=? AND m.thread=? `
	args := []any{room, thread}
	if before > 0 {
		where += `AND m.id<? `
		args = append(args, before)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, msgSelect+where+`ORDER BY m.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	out, cut, err := scanPage(rows, MaxPageBytes)
	reverse(out)
	return out, cut || len(out) == limit, err
}

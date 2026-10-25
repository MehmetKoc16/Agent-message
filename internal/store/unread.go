package store

import (
	"context"
	"errors"
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
}

// ThreadSummaries insanın oda günlüğüyle aynı görünürlüğü kullanır. Üye olunmayan
// odalar ve iptal edilmiş hesaplar sonuç üretmez; CLI inbox imlecine dokunulmaz.
func (s *Store) ThreadSummaries(ctx context.Context, agent string) ([]ThreadSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT m.room,m.thread,
 (SELECT substr(first.body,1,180) FROM messages first WHERE first.room=m.room AND first.thread=m.thread ORDER BY first.id LIMIT 1),
 COUNT(*),MAX(m.id),MAX(CASE WHEN m.from_agent<>? THEN m.id ELSE 0 END),COALESCE(r.last_id,0)
 FROM messages m JOIN agent_rooms ar ON ar.room=m.room AND ar.agent=?
 JOIN agents a ON a.name=ar.agent AND a.kind='insan' AND a.revoked_at IS NULL
 LEFT JOIN thread_reads r ON r.agent=ar.agent AND r.room=m.room AND r.thread=m.thread
 GROUP BY m.room,m.thread ORDER BY MAX(m.id) DESC`, agent, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ThreadSummary{}
	for rows.Next() {
		var row ThreadSummary
		if err := rows.Scan(&row.Room, &row.Thread, &row.Title, &row.Total, &row.LastID, &row.LastIncomingID, &row.ReadID); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
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

func (s *Store) ThreadLogPage(ctx context.Context, room, thread string, before int64, limit int) ([]Message, error) {
	where := `WHERE m.room=? AND m.thread=? `
	args := []any{room, thread}
	if before > 0 {
		where += `AND m.id<? `
		args = append(args, before)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, msgSelect+where+`ORDER BY m.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	out, err := scanMessages(rows)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, err
}

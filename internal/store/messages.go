package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/proto"
)

// Message ve Thread API'nin ortak tipleridir (istemci sqlite çekmesin diye proto'da).
type (
	Message = proto.Message
	Thread  = proto.Thread
)

// PostInput Post'a verilen alanlardır. From ve FromKind token'dan gelir.
type PostInput struct {
	Room, From, FromKind, To, Type, Thread, Body string
	ReplyTo                                      *int64
}

// anyAgent, herkese açık (to boş) mesaj gönderilmiş konuşmalarda thread_parties'e
// yazılan işaretçidir: odadaki herkes o mesajı görebildiği için konuşmanın tarafıdır.
const anyAgent = "*"

// Post mesajı ekler. Alıcı/reply_to/thread/taraf denetimi, tur sayacı ve ekleme
// AYNI transaction'dadır (BEGIN IMMEDIATE ile yazarlar sıralanır), bu yüzden
// eşzamanlı gönderimlerde tur numaraları benzersiz ve sınır kesindir.
func (s *Store) Post(ctx context.Context, in PostInput) (Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()

	if in.To != "" {
		if in.To == in.From {
			return Message{}, ErrBadRecipient
		}
		var one int
		err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM agent_rooms r JOIN agents a ON a.name = r.agent
			 WHERE r.room = ? AND r.agent = ? AND a.revoked_at IS NULL`, in.Room, in.To).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, ErrBadRecipient
		} else if err != nil {
			return Message{}, err
		}
	}

	now := s.stamp()
	threadID, turn, limit := in.Thread, 1, s.maxTurns
	if threadID == "" {
		if in.ReplyTo != nil { // yeni konuşmada yanıtlanacak mesaj olamaz
			return Message{}, ErrBadReplyTo
		}
		var n int64
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(CAST(substr(id, 3) AS INTEGER)), 0) + 1 FROM threads WHERE room = ?`,
			in.Room).Scan(&n); err != nil {
			return Message{}, err
		}
		threadID = fmt.Sprintf("T-%d", n)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO threads (room, id, status, turns, turn_limit, created_by, created_at)
			 VALUES (?, ?, 'acik', 1, ?, ?, ?)`, in.Room, threadID, limit, in.From, now); err != nil {
			return Message{}, err
		}
	} else {
		var status, createdBy string
		var turns int
		err := tx.QueryRowContext(ctx,
			`SELECT status, turns, turn_limit, created_by FROM threads WHERE room = ? AND id = ?`,
			in.Room, threadID).Scan(&status, &turns, &limit, &createdBy)
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, ErrThreadNotFound
		} else if err != nil {
			return Message{}, err
		}
		if in.FromKind != "insan" && createdBy != in.From {
			var one int
			err := tx.QueryRowContext(ctx,
				`SELECT 1 FROM thread_parties WHERE room = ? AND thread = ? AND agent IN (?, ?)`,
				in.Room, threadID, in.From, anyAgent).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return Message{}, ErrNotParty
			} else if err != nil {
				return Message{}, err
			}
		}
		if status != "acik" || turns >= limit {
			return Message{}, ErrThreadClosed
		}
		if in.ReplyTo != nil {
			// Yanıtlanan mesaj aynı konuşmada ve gönderenin görebildiği bir mesaj olmalı.
			var one int
			err := tx.QueryRowContext(ctx,
				`SELECT 1 FROM messages WHERE id = ? AND room = ? AND thread = ?
				 AND (to_agent = '' OR to_agent = ? OR from_agent = ?)`,
				*in.ReplyTo, in.Room, threadID, in.From, in.From).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return Message{}, ErrBadReplyTo
			} else if err != nil {
				return Message{}, err
			}
		}
		turn = turns + 1
		if _, err := tx.ExecContext(ctx,
			`UPDATE threads SET turns = ? WHERE room = ? AND id = ?`, turn, in.Room, threadID); err != nil {
			return Message{}, err
		}
	}

	// Taraflar mesajlardan bağımsız tutulur: saklama süresi mesajları silse de taraf kalır.
	to := in.To
	if to == "" {
		to = anyAgent
	}
	for _, ag := range []string{in.From, to} {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO thread_parties (room, thread, agent) VALUES (?,?,?)`,
			in.Room, threadID, ag); err != nil {
			return Message{}, err
		}
	}

	newStatus := ""
	if in.Type == "kapat" {
		newStatus = "kapali"
	} else if turn >= limit {
		newStatus = "kilitli"
	}
	if newStatus != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE threads SET status = ? WHERE room = ? AND id = ?`, newStatus, in.Room, threadID); err != nil {
			return Message{}, err
		}
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO messages (room, from_agent, to_agent, type, thread, reply_to, turn, body, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		in.Room, in.From, in.To, in.Type, threadID, in.ReplyTo, turn, in.Body, now)
	if err != nil {
		return Message{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Message{}, err
	}
	return Message{ID: id, Room: in.Room, From: in.From, To: in.To, Type: in.Type, Thread: threadID,
		ReplyTo: in.ReplyTo, Turn: turn, MaxTurns: limit, Body: in.Body, CreatedAt: now}, nil
}

const msgSelect = `SELECT m.id, m.room, m.from_agent, m.to_agent, m.type, m.thread, m.reply_to,
	m.turn, COALESCE(t.turn_limit, 0), m.body, m.created_at
	FROM messages m LEFT JOIN threads t ON t.room = m.room AND t.id = m.thread `

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var rt sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Room, &m.From, &m.To, &m.Type, &m.Thread, &rt,
			&m.Turn, &m.MaxTurns, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		if rt.Valid {
			v := rt.Int64
			m.ReplyTo = &v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Fetch ajanın görebileceği (to boş ya da ajan; from ajan değil) yeni mesajları verir.
func (s *Store) Fetch(ctx context.Context, room, agent string, after int64, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, msgSelect+
		`WHERE m.room = ? AND m.id > ? AND m.from_agent <> ? AND (m.to_agent = '' OR m.to_agent = ?)
		 ORDER BY m.id LIMIT ?`, room, after, agent, agent, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// GetThread konuşma durumunu verir.
func (s *Store) GetThread(ctx context.Context, room, id string) (Thread, error) {
	t := Thread{Room: room, ID: id}
	err := s.db.QueryRowContext(ctx,
		`SELECT status, turns, turn_limit, created_by, created_at FROM threads WHERE room = ? AND id = ?`,
		room, id).Scan(&t.Status, &t.Turns, &t.TurnLimit, &t.CreatedBy, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Thread{}, ErrThreadNotFound
	}
	return t, err
}

// ThreadMessages konuşmada ajanın görebildiği mesajlardır (kendi gönderdikleri dahil).
func (s *Store) ThreadMessages(ctx context.Context, room, id, agent string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, msgSelect+
		`WHERE m.room = ? AND m.thread = ? AND (m.to_agent = '' OR m.to_agent = ? OR m.from_agent = ?)
		 ORDER BY m.id`, room, id, agent, agent)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// IsThreadParty ajan konuşmayı açtıysa, mesaj gönderdiyse ya da mesaj hedefi olduysa
// (herkese açık mesaj olan konuşmada odadaki herkes) true. thread_parties'e bakar; bu yüzden
// mesajlar saklama süresiyle silinse de sonuç değişmez.
func (s *Store) IsThreadParty(ctx context.Context, room, id, agent string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM threads WHERE room = ? AND id = ? AND created_by = ?
		 UNION ALL SELECT 1 FROM thread_parties WHERE room = ? AND thread = ? AND agent IN (?, ?)
		 LIMIT 1`, room, id, agent, room, id, agent, anyAgent).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CloseThread konuşmayı kapatır (zaten kapalıysa değişmez).
func (s *Store) CloseThread(ctx context.Context, room, id string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE threads SET status = 'kapali' WHERE room = ? AND id = ?`, room, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrThreadNotFound
	}
	return nil
}

// ReopenThread konuşmayı açar ve tur sınırını add kadar artırır.
func (s *Store) ReopenThread(ctx context.Context, room, id string, add int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE threads SET status = 'acik', turn_limit = turn_limit + ? WHERE room = ? AND id = ?`,
		add, room, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrThreadNotFound
	}
	return nil
}

// Purge cutoff'tan eski mesajları siler ve silinen sayısını döndürür.
// (Konuşma satırları küçük olduğu için silinmez; T-<n> numaraları yeniden kullanılmaz.)
func (s *Store) Purge(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE created_at < ?`, FormatTime(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

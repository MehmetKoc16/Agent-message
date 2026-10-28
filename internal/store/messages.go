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
	RequestID                                    string
	Announcement                                 bool
}

// anyAgent, herkese açık (to boş) mesaj gönderilmiş konuşmalarda thread_parties'e
// yazılan işaretçidir: odadaki herkes o mesajı görebildiği için konuşmanın tarafıdır.
const anyAgent = "*"

var ErrBadAnnouncement = errors.New("duyuru yalnız herkese açık bilgi ya da bildirim mesajı olabilir; bildirim yalnız T-0'da")

// IsAnnouncementType T-0 duyuru akışında kabul edilen mesaj türleridir:
// "bilgi" (Güncellemeler) ve "bildirim" (Bildirimler; dinleyen ajanı uyandırır).
func IsAnnouncementType(typ string) bool { return typ == "bilgi" || typ == "bildirim" }

// Post mesajı ekler. Alıcı/reply_to/thread/taraf denetimi, tur sayacı ve ekleme
// AYNI transaction'dadır (BEGIN IMMEDIATE ile yazarlar sıralanır), bu yüzden
// eşzamanlı gönderimlerde tur numaraları benzersiz ve sınır kesindir.
func (s *Store) Post(ctx context.Context, in PostInput) (Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, err
	}
	defer tx.Rollback()
	kind, err := requireRoomMemberTx(ctx, tx, in.From, in.Room)
	if err != nil {
		return Message{}, err
	}
	in.FromKind = kind
	if in.Announcement {
		if !IsAnnouncementType(in.Type) || in.To != "" || in.ReplyTo != nil || (in.Thread != "" && in.Thread != "T-0") {
			return Message{}, ErrBadAnnouncement
		}
		in.Thread = "T-0"
	}
	// "bildirim" dinleyen ajanları uyandırır; normal konuşmada (yeni konu ya da
	// T-n) kabul edilirse herhangi bir mesaj bildirim gibi gösterilebilirdi.
	if in.Type == "bildirim" && in.Thread != "T-0" {
		return Message{}, ErrBadAnnouncement
	}
	if in.RequestID != "" {
		rows, err := tx.QueryContext(ctx, msgSelect+`WHERE m.room=? AND m.from_agent=? AND m.request_id=? AND m.request_id <> ''`, in.Room, in.From, in.RequestID)
		if err != nil {
			return Message{}, err
		}
		prior, err := scanMessages(rows)
		if err != nil {
			return Message{}, err
		}
		if len(prior) > 0 {
			m := prior[0]
			sameReply := (m.ReplyTo == nil && in.ReplyTo == nil) || (m.ReplyTo != nil && in.ReplyTo != nil && *m.ReplyTo == *in.ReplyTo)
			if m.To != in.To || m.Type != in.Type || m.Body != in.Body || !sameReply || (in.Thread != "" && m.Thread != in.Thread) {
				return Message{}, ErrRequestConflict
			}
			return m, tx.Commit()
		}
	}

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
	if in.Announcement {
		// T-0 is reserved for a room's shared broadcast feed. It does not consume topic IDs.
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO threads(room,id,status,turns,turn_limit,created_by,created_at) VALUES (?, 'T-0', 'acik', 0, 0, ?, ?)`, in.Room, in.From, now); err != nil {
			return Message{}, err
		}
	}
	threadID, turn, limit := in.Thread, 1, s.maxTurns
	publicOnly := false
	if threadID == "" {
		if in.ReplyTo != nil { // yeni konuşmada yanıtlanacak mesaj olamaz
			return Message{}, ErrBadReplyTo
		}
		var n int64
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO room_counters(room,last_thread) VALUES (?,1)
			 ON CONFLICT(room) DO UPDATE SET last_thread=last_thread+1 RETURNING last_thread`,
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
		var turns, publicTurns int
		err := tx.QueryRowContext(ctx,
			`SELECT status, turns, turn_limit, created_by, public_turns FROM threads WHERE room = ? AND id = ?`,
			in.Room, threadID).Scan(&status, &turns, &limit, &createdBy, &publicTurns)
		if errors.Is(err, sql.ErrNoRows) {
			return Message{}, ErrThreadNotFound
		} else if err != nil {
			return Message{}, err
		}
		if in.FromKind != "insan" && createdBy != in.From {
			if in.Type == "kapat" {
				return Message{}, ErrNotParty
			}
			var one int
			err := tx.QueryRowContext(ctx,
				`SELECT 1 FROM thread_parties WHERE room = ? AND thread = ? AND agent IN (?, ?)`,
				in.Room, threadID, in.From, anyAgent).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return Message{}, ErrNotParty
			} else if err != nil {
				return Message{}, err
			}
			var explicit int
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM thread_parties WHERE room=? AND thread=? AND agent=?`, in.Room, threadID, in.From).Scan(&explicit)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return Message{}, err
			}
			publicOnly = errors.Is(err, sql.ErrNoRows)
		}
		if limit == 0 && (!IsAnnouncementType(in.Type) || in.To != "" || in.ReplyTo != nil) {
			return Message{}, ErrBadAnnouncement
		}
		if status != "acik" || (limit > 0 && turns >= limit) {
			return Message{}, ErrThreadClosed
		}
		// Wildcard viewers share at most half the budget and cannot consume
		// the final turn. The counter survives retention and process restarts.
		if publicOnly && limit > 0 && (publicTurns >= max(1, limit/2) || turns+1 >= limit) {
			return Message{}, ErrNotParty
		}
		if in.ReplyTo != nil {
			// Yanıtlanan mesaj aynı konuşmada ve gönderenin görebildiği bir mesaj olmalı.
			// İnsanlar odanın özel mesajlarını da gördüğü için onlara da yanıt verebilir.
			query := `SELECT 1 FROM messages WHERE id = ? AND room = ? AND thread = ?`
			args := []any{*in.ReplyTo, in.Room, threadID}
			if in.FromKind != "insan" {
				query += ` AND (to_agent = '' OR to_agent = ? OR from_agent = ?)`
				args = append(args, in.From, in.From)
			}
			var one int
			err := tx.QueryRowContext(ctx, query, args...).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return Message{}, ErrBadReplyTo
			} else if err != nil {
				return Message{}, err
			}
		}
		turn = turns + 1
		if _, err := tx.ExecContext(ctx,
			`UPDATE threads SET turns = ?, public_turns = public_turns + ? WHERE room = ? AND id = ?`, turn, boolInt(publicOnly), in.Room, threadID); err != nil {
			return Message{}, err
		}
	}

	// Taraflar mesajlardan bağımsız tutulur: saklama süresi mesajları silse de taraf kalır.
	to := in.To
	if to == "" {
		to = anyAgent
	}
	for _, ag := range []string{in.From, to} {
		// A public reply does not invite its author or another account into
		// the trusted participant set, otherwise the next reply bypasses quota.
		if publicOnly && ag != anyAgent {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO thread_parties (room, thread, agent) VALUES (?,?,?)`,
			in.Room, threadID, ag); err != nil {
			return Message{}, err
		}
	}

	newStatus := ""
	if in.Type == "kapat" {
		newStatus = "kapali"
	} else if limit > 0 && turn >= limit {
		newStatus = "kilitli"
	}
	if newStatus != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE threads SET status = ? WHERE room = ? AND id = ?`, newStatus, in.Room, threadID); err != nil {
			return Message{}, err
		}
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO messages (room, from_agent, to_agent, type, thread, reply_to, turn, body, created_at, request_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		in.Room, in.From, in.To, in.Type, threadID, in.ReplyTo, turn, in.Body, now, in.RequestID)
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
	return Message{ID: id, Room: in.Room, From: in.From, FromKind: in.FromKind, To: in.To, Type: in.Type, Thread: threadID,
		ReplyTo: in.ReplyTo, Turn: turn, MaxTurns: limit, Body: in.Body, CreatedAt: now}, nil
}

// msgSelect gönderenin türünü agents tablosundan alır (LEFT JOIN: kayıt yoksa
// mesaj yine döner, from_kind boş olur). Tür gövdeden ya da istemciden gelmez.
const msgSelect = `SELECT m.id, m.room, m.from_agent, COALESCE(a.kind, ''), m.to_agent, m.type, m.thread, m.reply_to,
	m.turn, COALESCE(t.turn_limit, 0), m.body, m.created_at
	FROM messages m LEFT JOIN threads t ON t.room = m.room AND t.id = m.thread
	LEFT JOIN agents a ON a.name = m.from_agent `

func scanMessages(rows *sql.Rows) ([]Message, error) {
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var rt sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Room, &m.From, &m.FromKind, &m.To, &m.Type, &m.Thread, &rt,
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

// RoomLog odadaki tüm mesajları artan id sırasıyla verir.
func (s *Store) RoomLog(ctx context.Context, room string, after int64, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, msgSelect+
		`WHERE m.room = ? AND m.id > ? ORDER BY m.id LIMIT ?`, room, after, limit)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// RoomLogPage en yeni veya before öncesindeki sayfayı, artan ID sırasıyla verir.
func (s *Store) RoomLogPage(ctx context.Context, room string, before int64, limit int) ([]Message, error) {
	where := `WHERE m.room = ? `
	args := []any{room}
	if before > 0 {
		where += `AND m.id < ? `
		args = append(args, before)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, msgSelect+where+`ORDER BY m.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, err
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

// CloseThreadFor kimlik, üyelik, taraf ve kapatmayı aynı transaction'da yapar.
func (s *Store) CloseThreadFor(ctx context.Context, room, id, agent string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	kind, err := requireRoomMemberTx(ctx, tx, agent, room)
	if err != nil {
		return err
	}
	var creator string
	var limit int
	err = tx.QueryRowContext(ctx, `SELECT created_by,turn_limit FROM threads WHERE room=? AND id=?`, room, id).Scan(&creator, &limit)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrThreadNotFound
	}
	if err != nil {
		return err
	}
	if limit == 0 {
		return ErrBadAnnouncement
	}
	if kind != "insan" && creator != agent {
		return ErrNotParty
	}
	if _, err := tx.ExecContext(ctx, `UPDATE threads SET status='kapali' WHERE room=? AND id=?`, room, id); err != nil {
		return err
	}
	return tx.Commit()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// CloseThread güvenilir yerel işlemler içindir; HTTP CloseThreadFor kullanır.
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

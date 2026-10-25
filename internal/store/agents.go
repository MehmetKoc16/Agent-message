package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
)

// Agent kimliği doğrulanmış ajandır.
type Agent struct {
	Name string
	Kind string // "ajan" | "insan"
}

// AgentRow admin listesi içindir. Token ya da özeti içermez.
type AgentRow struct {
	Yonetici                                   bool
	Name, Kind, CreatedAt, LastSeen, RevokedAt string
	Rooms                                      []string
}

// RoomAgent bir odadaki ajanın herkese açık bilgisidir.
type RoomAgent struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	LastSeen string `json:"last_seen"`
}

// HashToken token'ın saklanan SHA-256 özetini (hex) döndürür.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// AddAgent ajanı ve odalarını tek transaction'da ekler. Yalnız özet saklanır.
func (s *Store) AddAgent(ctx context.Context, name, kind, tokenHash string, rooms []string) error {
	return s.addAgent(ctx, name, kind, tokenHash, rooms, "", false)
}

// AddAgentWithKayit web/SSH hesabını ve denetim kaydını birlikte commit eder.
func (s *Store) AddAgentWithKayit(ctx context.Context, name, kind, tokenHash string, rooms []string, yapan string, web bool) error {
	return s.addAgent(ctx, name, kind, tokenHash, rooms, yapan, web)
}
func (s *Store) addAgent(ctx context.Context, name, kind, tokenHash string, rooms []string, yapan string, web bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if web {
		if err := requireYoneticiTx(ctx, tx, yapan); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO agents (name, token_hash, kind, created_at) VALUES (?,?,?,?)`,
		name, tokenHash, kind, s.stamp()); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY") {
			return ErrExists
		}
		return err
	}
	for _, r := range rooms {
		if err := ensureRoomTx(ctx, tx, r, s.stamp()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO agent_rooms (agent, room) VALUES (?,?)`, name, r); err != nil {
			return err
		}
	}
	if yapan != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO yonetim_kayit(zaman,yapan,islem,hedef,ayrinti) VALUES (?,?,'hesap_acildi',?,?)`, s.stamp(), yapan, name, strings.Join(rooms, ",")); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RevokeAgent ajanı iptal eder (token artık 401 verir).
func (s *Store) RevokeAgent(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET revoked_at = COALESCE(revoked_at, ?) WHERE name = ?`, s.stamp(), name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Authenticate token'ı doğrular. İptal edilmiş ya da bilinmeyen token ErrUnauthorized verir.
// Özet ile arama yapılır (özet tahmin edilemez); sonuç ayrıca sabit zamanlı karşılaştırılır.
func (s *Store) Authenticate(ctx context.Context, token string) (Agent, error) {
	h := HashToken(token)
	var a Agent
	var stored string
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT name, kind, token_hash, revoked_at FROM agents WHERE token_hash = ?`, h).
		Scan(&a.Name, &a.Kind, &stored, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, ErrUnauthorized
	}
	if err != nil {
		return Agent{}, err
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(h)) != 1 || revoked.Valid {
		return Agent{}, ErrUnauthorized
	}
	return a, nil
}

// HasRoom ajanın odaya üyeliğini denetler.
func (s *Store) HasRoom(ctx context.Context, agent, room string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM agent_rooms WHERE agent = ? AND room = ?`, agent, room).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// requireRoomMemberTx yazma yetkisini değişikliğin transaction'ında denetler.
// İlk HTTP denetiminden sonra gelen iptal/üyelik değişimi eski yetkiyle yazamaz.
func requireRoomMemberTx(ctx context.Context, tx *sql.Tx, agent, room string) (string, error) {
	var kind string
	var revoked sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT kind,revoked_at FROM agents WHERE name=?`, agent).Scan(&kind, &revoked)
	if errors.Is(err, sql.ErrNoRows) || err == nil && revoked.Valid {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}
	var one int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM agent_rooms WHERE agent=? AND room=?`, agent, room).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrRoomForbidden
	}
	return kind, err
}

// AgentRooms ajanın odalarını ada göre sıralı döndürür; odası yoksa boş listedir.
func (s *Store) AgentRooms(ctx context.Context, name string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT room FROM agent_rooms WHERE agent = ? ORDER BY room`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var room string
		if err := rows.Scan(&room); err != nil {
			return nil, err
		}
		out = append(out, room)
	}
	return out, rows.Err()
}

// Touch last_seen alanını günceller.
func (s *Store) Touch(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET last_seen = ? WHERE name = ?`, s.stamp(), name)
	return err
}

// RoomAgents odadaki iptal edilmemiş ajanları listeler.
func (s *Store) RoomAgents(ctx context.Context, room string) ([]RoomAgent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.name, a.kind, COALESCE(a.last_seen, '') FROM agents a
		 JOIN agent_rooms r ON r.agent = a.name
		 WHERE r.room = ? AND a.revoked_at IS NULL ORDER BY a.name`, room)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoomAgent{}
	for rows.Next() {
		var a RoomAgent
		if err := rows.Scan(&a.Name, &a.Kind, &a.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAgents tüm ajanları (iptal edilenler dahil) listeler; token ya da özet içermez.
func (s *Store) ListAgents(ctx context.Context) ([]AgentRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, kind, created_at, COALESCE(last_seen,''), COALESCE(revoked_at,''), yonetici
		 FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var out []AgentRow
	for rows.Next() {
		var a AgentRow
		if err := rows.Scan(&a.Name, &a.Kind, &a.CreatedAt, &a.LastSeen, &a.RevokedAt, &a.Yonetici); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, a)
	}
	rows.Close() // tek bağlantı: odaları sorgulamadan önce bırak
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		rr, err := s.db.QueryContext(ctx,
			`SELECT room FROM agent_rooms WHERE agent = ? ORDER BY room`, out[i].Name)
		if err != nil {
			return nil, err
		}
		for rr.Next() {
			var room string
			if err := rr.Scan(&room); err != nil {
				rr.Close()
				return nil, err
			}
			out[i].Rooms = append(out[i].Rooms, room)
		}
		err = rr.Err()
		rr.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

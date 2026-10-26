package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Room başlık ile protokolde kullanılan sabit oda kimliğini ayırır.
type Room struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// AllRooms yönetim ekranındaki başlık seçimi içindir; API yönetici + TOTP ister.
func (s *Store) AllRooms(ctx context.Context) ([]Room, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name,title FROM rooms ORDER BY title,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Room{}
	for rows.Next() {
		var room Room
		if err := rows.Scan(&room.Name, &room.Title); err != nil {
			return nil, err
		}
		out = append(out, room)
	}
	return out, rows.Err()
}

func ensureRoomTx(ctx context.Context, tx *sql.Tx, room, stamp string) error {
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO rooms(name,title,created_at) VALUES (?,?,?)`, room, room, stamp)
	return err
}

// requireRoomTx web yolları içindir: web oda açmaz, odalar başlık ve üye seçimiyle
// ayrıca oluşturulur. SSH/CLI yolları ensureRoomTx ile oda açmaya devam eder.
func requireRoomTx(ctx context.Context, tx *sql.Tx, room string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM rooms WHERE name=?`, room).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnknownRoom
	}
	return err
}

// RoomsForAgent yalnız üye olunan odaların başlıklarını verir; genel oda kataloğu değildir.
func (s *Store) RoomsForAgent(ctx context.Context, agent string) ([]Room, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ar.room,COALESCE(r.title,ar.room) FROM agent_rooms ar
		JOIN agents a ON a.name=ar.agent AND a.revoked_at IS NULL
		LEFT JOIN rooms r ON r.name=ar.room WHERE ar.agent=? ORDER BY ar.room`, agent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Room{}
	for rows.Next() {
		var r Room
		if err := rows.Scan(&r.Name, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateRoom rolü, üyeleri, odayı ve denetim kaydını aynı transaction'da denetler/yazar.
// Oluşturan yönetici kendiliğinden üyedir; başka hesaplar yalnız açık seçimle eklenir.
func (s *Store) CreateRoom(ctx context.Context, creator, title string, members []string) (Room, error) {
	title = strings.TrimSpace(title)
	if title == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 80 || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return Room{}, ErrBadRoomTitle
	}
	if len(members) > 100 {
		return Room{}, ErrBadMember
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Room{}, err
	}
	r := Room{Name: "oda-" + hex.EncodeToString(id[:]), Title: title}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Room{}, err
	}
	defer tx.Rollback()
	if err := requireYoneticiTx(ctx, tx, creator); err != nil {
		return Room{}, err
	}
	selected := []string{creator}
	seen := map[string]bool{creator: true}
	for _, name := range members {
		if !seen[name] {
			selected = append(selected, name)
			seen[name] = true
		}
	}
	for _, name := range selected {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM agents WHERE name=? AND revoked_at IS NULL`, name).Scan(&one); err != nil {
			if err == sql.ErrNoRows {
				return Room{}, ErrBadMember
			}
			return Room{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rooms(name,title,created_at,created_by) VALUES (?,?,?,?)`, r.Name, r.Title, s.stamp(), creator); err != nil {
		return Room{}, err
	}
	for _, name := range selected {
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_rooms(agent,room) VALUES (?,?)`, name, r.Name); err != nil {
			return Room{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO yonetim_kayit(zaman,yapan,islem,hedef,ayrinti) VALUES (?,?,'oda_olusturuldu',?,?)`, s.stamp(), creator, r.Name, r.Title); err != nil {
		return Room{}, err
	}
	for _, name := range selected {
		if _, err := tx.ExecContext(ctx, `INSERT INTO yonetim_kayit(zaman,yapan,islem,hedef,ayrinti) VALUES (?,?,'odaya_eklendi',?,?)`, s.stamp(), creator, name, r.Name); err != nil {
			return Room{}, err
		}
	}
	return r, tx.Commit()
}

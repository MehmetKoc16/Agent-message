package store

import (
	"context"
	"database/sql"
	"errors"
)

var ErrYoneticiProtected = errors.New("yoneticiye_dokunulamaz")

func requireYoneticiTx(ctx context.Context, tx *sql.Tx, ad string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM agents WHERE name=? AND kind='insan' AND yonetici=1 AND revoked_at IS NULL`, ad).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnauthorized
	}
	return err
}

// ChangeAccount hedef denetimini, değişikliği ve kaydı tek transaction'da yapar.
// SSH ataması ile web değişikliği yarışsa da yönetici korunur.
func (s *Store) ChangeAccount(ctx context.Context, yapan, islem, ad, oda, secret string, protect bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if protect {
		if err := requireYoneticiTx(ctx, tx, yapan); err != nil {
			return err
		}
	}
	var yes bool
	var kind string
	var revoked sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT yonetici,kind,revoked_at FROM agents WHERE name=?`, ad).Scan(&yes, &kind, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if protect && yes {
		return ErrYoneticiProtected
	}
	switch islem {
	case "yonetici-yap":
		if kind != "insan" || revoked.Valid {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `UPDATE agents SET yonetici=1,totp_secret=?,totp_last_step=0 WHERE name=?`, secret, ad)
	case "yonetici-al":
		_, err = tx.ExecContext(ctx, `UPDATE agents SET yonetici=0,totp_secret=NULL,totp_last_step=0 WHERE name=?`, ad)
	case "odaya-ekle", "odaya_eklendi":
		if revoked.Valid {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_rooms(agent,room) VALUES (?,?)`, ad, oda)
	case "odadan-cikar", "odadan_cikarildi":
		_, err = tx.ExecContext(ctx, `DELETE FROM agent_rooms WHERE agent=? AND room=?`, ad, oda)
	case "revoke", "hesap_iptal":
		_, err = tx.ExecContext(ctx, `UPDATE agents SET revoked_at=COALESCE(revoked_at,?) WHERE name=?`, s.stamp(), ad)
	default:
		return errors.New("geçersiz yönetim işlemi")
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO yonetim_kayit(zaman,yapan,islem,hedef,ayrinti) VALUES (?,?,?,?,?)`, s.stamp(), yapan, islem, ad, oda); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AddRoom(ctx context.Context, ad, oda string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var one int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM agents WHERE name=? AND revoked_at IS NULL`, ad).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_rooms(agent,room) VALUES (?,?)`, ad, oda); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) RemoveRoom(ctx context.Context, ad, oda string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var one int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM agents WHERE name=?`, ad).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_rooms WHERE agent=? AND room=?`, ad, oda); err != nil {
		return err
	}
	return tx.Commit()
}
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) SetYonetici(ctx context.Context, ad, secret string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE agents SET yonetici=1,totp_secret=?,totp_last_step=0 WHERE name=? AND kind='insan' AND revoked_at IS NULL`, secret, ad))
}
func (s *Store) ClearYonetici(ctx context.Context, ad string) error {
	return affected(s.db.ExecContext(ctx, `UPDATE agents SET yonetici=0,totp_secret=NULL,totp_last_step=0 WHERE name=?`, ad))
}
func (s *Store) IsYonetici(ctx context.Context, ad string) (bool, error) {
	var yes bool
	err := s.db.QueryRowContext(ctx, `SELECT yonetici FROM agents WHERE name=?`, ad).Scan(&yes)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return yes, err
}
func (s *Store) TOTPState(ctx context.Context, ad string) (string, int64, error) {
	var secret string
	var step int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(totp_secret,''),totp_last_step FROM agents WHERE name=?`, ad).Scan(&secret, &step)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return secret, step, err
}
func (s *Store) MarkTOTPStep(ctx context.Context, ad string, step int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE agents SET totp_last_step=? WHERE name=? AND totp_last_step < ? AND yonetici=1 AND revoked_at IS NULL`, step, ad, step)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if n == 0 && err == nil {
		_, _, err = s.TOTPState(ctx, ad)
	}
	return n > 0, err
}

type Kayit struct {
	ID      int64  `json:"id"`
	Zaman   string `json:"zaman"`
	Yapan   string `json:"yapan"`
	Islem   string `json:"islem"`
	Hedef   string `json:"hedef"`
	Ayrinti string `json:"ayrinti"`
}

func (s *Store) AddKayit(ctx context.Context, yapan, islem, hedef, ayrinti string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO yonetim_kayit(zaman,yapan,islem,hedef,ayrinti) VALUES (?,?,?,?,?)`, s.stamp(), yapan, islem, hedef, ayrinti)
	return err
}
func (s *Store) ListKayit(ctx context.Context, limit int) ([]Kayit, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,zaman,yapan,islem,hedef,ayrinti FROM yonetim_kayit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Kayit{}
	for rows.Next() {
		var k Kayit
		if err := rows.Scan(&k.ID, &k.Zaman, &k.Yapan, &k.Islem, &k.Hedef, &k.Ayrinti); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

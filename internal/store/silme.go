package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeleteMessage yöneticinin odadaki tek bir mesajı kalıcı olarak silmesidir ve
// mesajın konuşmasını döndürür. Yetki, silme ve denetim kaydı aynı
// transaction'dadır; mesaj gövdesi kayda yazılmaz.
func (s *Store) DeleteMessage(ctx context.Context, yapan, room string, id int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := requireYoneticiTx(ctx, tx, yapan); err != nil {
		return "", err
	}
	if err := requireRoomTx(ctx, tx, room); err != nil {
		return "", err
	}
	var thread string
	err = tx.QueryRowContext(ctx, `DELETE FROM messages WHERE room=? AND id=? RETURNING thread`, room, id).Scan(&thread)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO yonetim_kayit(zaman,yapan,islem,hedef,ayrinti) VALUES (?,?,?,?,?)`,
		s.stamp(), yapan, "mesaj_silindi", room, fmt.Sprintf("#%d · %s", id, thread)); err != nil {
		return "", err
	}
	return thread, tx.Commit()
}

// DeleteThread konuşmanın bütün mesajlarını siler ve silinen sayıyı döndürür.
// Konuşma satırı Purge'deki gibi kalır: T-<n> yeniden kullanılmaz, tur sayacı ve
// taraflar korunur, konuşma açıksa yeni mesaj alabilir.
func (s *Store) DeleteThread(ctx context.Context, yapan, room, thread string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := requireYoneticiTx(ctx, tx, yapan); err != nil {
		return 0, err
	}
	if err := requireRoomTx(ctx, tx, room); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE room=? AND thread=?`, room, thread)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO yonetim_kayit(zaman,yapan,islem,hedef,ayrinti) VALUES (?,?,?,?,?)`,
		s.stamp(), yapan, "konusma_silindi", room, fmt.Sprintf("%s · %d mesaj", thread, n)); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

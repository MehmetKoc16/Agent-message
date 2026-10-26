package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// AttemptTOTP persists the sliding window and lock in the same transaction as
// replay protection. Multiple server instances and restarts share the budget.
func (s *Store) AttemptTOTP(ctx context.Context, name, code string, match func(string, string, time.Time) (int64, bool)) (secret string, accepted, locked bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, false, err
	}
	defer tx.Rollback()
	if err := requireYoneticiTx(ctx, tx, name); err != nil {
		return "", false, false, err
	}
	var raw, until string
	var attempts []time.Time
	err = tx.QueryRowContext(ctx, `SELECT attempts,locked FROM yonetim_failures WHERE agent=?`, name).Scan(&raw, &until)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, false, err
	}
	if err == nil && json.Unmarshal([]byte(raw), &attempts) != nil {
		return "", false, false, errors.New("TOTP deneme kaydı bozuk")
	}
	now := s.Now()
	if until != "" {
		deadline, err := time.Parse(time.RFC3339Nano, until)
		if err != nil {
			return "", false, false, errors.New("TOTP kilit kaydı bozuk")
		}
		if now.Before(deadline) {
			return "", false, true, tx.Commit()
		}
	}
	var last int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(totp_secret,''),totp_last_step FROM agents WHERE name=?`, name).Scan(&secret, &last); err != nil {
		return "", false, false, err
	}
	secret, err = s.openTOTP(name, secret)
	if err != nil {
		return "", false, false, err
	}
	step, ok := match(secret, code, now)
	if ok && step > last {
		if _, err = tx.ExecContext(ctx, `UPDATE agents SET totp_last_step=? WHERE name=?`, step, name); err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM yonetim_failures WHERE agent=?`, name)
		}
		if err != nil {
			return "", false, false, err
		}
		return secret, true, false, tx.Commit()
	}
	recent := []time.Time{}
	for _, at := range attempts {
		if at.After(now.Add(-15 * time.Minute)) {
			recent = append(recent, at)
		}
	}
	recent = append(recent, now)
	until = ""
	locked = len(recent) >= 5
	if locked {
		until = now.Add(15 * time.Minute).UTC().Format(time.RFC3339Nano)
	}
	data, _ := json.Marshal(recent)
	if _, err := tx.ExecContext(ctx, `INSERT INTO yonetim_failures(agent,attempts,locked) VALUES (?,?,?) ON CONFLICT(agent) DO UPDATE SET attempts=excluded.attempts,locked=excluded.locked`, name, string(data), until); err != nil {
		return "", false, false, err
	}
	return "", false, locked, tx.Commit()
}

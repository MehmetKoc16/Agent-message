// Package store, relay verisini SQLite üzerinde saklar.
//
// Eşzamanlılık: veritabanına TEK bağlantı açılır (SetMaxOpenConns(1)). Böylece
// tüm okuma ve yazmalar sırayla çalışır, "database is locked" yarışı olmaz ve
// bellek en düşük kalır. Ajan sayısı az olduğundan (<=10) ve sorgular
// milisaniyenin altında olduğundan bu bir darboğaz değildir. Admin komutu ayrı
// süreçtir; onunla çakışmayı busy_timeout ve BEGIN IMMEDIATE çözer.
// Uzun bekleme (long-poll) veritabanını tutmaz: sorgu biter, bağlantı hemen
// bırakılır, bekleme bellekte (server.Hub) yapılır.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // sürücü adı: "sqlite"
)

const schemaVersion = 1

// Hata değerleri; server paketi bunları HTTP koduna çevirir.
var (
	ErrUnauthorized   = errors.New("yetkisiz")
	ErrThreadNotFound = errors.New("konuşma yok")
	ErrThreadClosed   = errors.New("konuşma kilitli ya da kapalı")
	ErrBadRecipient   = errors.New("alıcı geçersiz")
	ErrBadReplyTo     = errors.New("reply_to geçersiz")
	ErrNotParty       = errors.New("konuşmanın tarafı değil")
	ErrNotFound       = errors.New("bulunamadı")
	ErrExists         = errors.New("zaten var")
)

// Store veritabanı erişim katmanıdır.
type Store struct {
	db       *sql.DB
	maxTurns int
	// Now testlerde saati değiştirmek içindir.
	Now func() time.Time
}

// Open veritabanını açar, WAL'ı ve şemayı hazırlar.
func Open(path string, maxTurns int) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_pragma=cache_size(-2000)" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("veritabanı açılamadı: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	s := &Store{db: db, maxTurns: maxTurns, Now: time.Now}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close veritabanını kapatır.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return fmt.Errorf("şema sürümü okunamadı: %w", err)
	}
	if v > schemaVersion {
		return fmt.Errorf("veritabanı şema sürümü (%d) bu sürümden yeni (%d)", v, schemaVersion)
	}
	if v == schemaVersion {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE agents (
			name TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL CHECK (kind IN ('ajan','insan')),
			created_at TEXT NOT NULL,
			revoked_at TEXT,
			last_seen TEXT)`,
		`CREATE TABLE agent_rooms (
			agent TEXT NOT NULL REFERENCES agents(name),
			room TEXT NOT NULL,
			PRIMARY KEY (agent, room))`,
		`CREATE TABLE threads (
			room TEXT NOT NULL,
			id TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('acik','kilitli','kapali')),
			turns INTEGER NOT NULL,
			turn_limit INTEGER NOT NULL,
			created_by TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (room, id))`,
		`CREATE TABLE thread_parties (
			room TEXT NOT NULL,
			thread TEXT NOT NULL,
			agent TEXT NOT NULL,
			PRIMARY KEY (room, thread, agent))`,
		`CREATE TABLE messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			room TEXT NOT NULL,
			from_agent TEXT NOT NULL,
			to_agent TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL,
			thread TEXT NOT NULL,
			reply_to INTEGER,
			turn INTEGER NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL)`,
		`CREATE INDEX messages_room_id ON messages(room, id)`,
		`CREATE INDEX messages_thread ON messages(room, thread, id)`,
		`CREATE INDEX messages_created ON messages(created_at)`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("şema kurulamadı: %w", err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) stamp() string { return FormatTime(s.Now()) }

// FormatTime zamanı sıralanabilir UTC metnine çevirir.
func FormatTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// Ping veritabanının yanıt verdiğini denetler.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

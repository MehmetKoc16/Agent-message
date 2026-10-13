package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "relay.db"), 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// NEDEN: token'ın kendisi diske yazılırsa DB sızıntısı = tüm ajanların ele geçirilmesi.
// Hiçbir sütunda token geçmemeli; yalnız SHA-256 özeti (64 hex) durmalı.
func TestTokenOzetOlarakSaklanir(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	token := "gizli-token-DEGERI-12345"
	if err := s.AddAgent(ctx, "ali", "ajan", HashToken(token), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	var name, hash, kind, created string
	var revoked, seen sql.NullString
	if err := s.db.QueryRow(`SELECT name, token_hash, kind, created_at, revoked_at, last_seen FROM agents`).
		Scan(&name, &hash, &kind, &created, &revoked, &seen); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{name, hash, kind, created} {
		if strings.Contains(col, token) {
			t.Fatalf("token düz metin saklanmış: %q", col)
		}
	}
	if len(hash) != 64 || hash != HashToken(token) {
		t.Fatalf("özet SHA-256 hex olmalı: %q", hash)
	}
	if a, err := s.Authenticate(ctx, token); err != nil || a.Name != "ali" {
		t.Fatalf("doğru token geçmeli: %v %v", a, err)
	}
	if _, err := s.Authenticate(ctx, hash); err == nil {
		t.Fatal("özetin kendisi token olarak kabul edilmemeli")
	}
}

// NEDEN: şema sürümlüdür; daha yeni bir veritabanını eski ikili sessizce bozmamalı.
func TestSemaSurumuKorunur(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.db")
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != schemaVersion {
		t.Fatalf("user_version=%d err=%v", v, err)
	}
	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode=%q", mode)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s2, err := Open(path, 20); err == nil {
		s2.Close()
		t.Fatal("daha yeni şema reddedilmeli")
	}
}

// NEDEN: SQL enjeksiyonu denemesi (parametreli sorgular) veriyi bozmamalı; metin olarak saklanmalı.
func TestSorgularParametreli(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "ali", "ajan", HashToken("t"), []string{"r1"}); err != nil {
		t.Fatal(err)
	}
	evil := "x'); DROP TABLE messages; --"
	if _, err := s.Post(ctx, PostInput{Room: "r1", From: "ali", Type: "bilgi", Body: evil}); err != nil {
		t.Fatal(err)
	}
	msgs, err := s.Fetch(ctx, "r1", "veli", 0, 10)
	if err != nil || len(msgs) != 1 || msgs[0].Body != evil {
		t.Fatalf("%v %v", msgs, err)
	}
	if _, err := s.Authenticate(ctx, "' OR '1'='1"); err == nil {
		t.Fatal("enjeksiyon token'ı geçmemeli")
	}
}

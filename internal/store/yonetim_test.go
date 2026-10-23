package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// NEDEN: canlı v1 verisi silinmeden yalnız yeni sütun ve kayıt tablosu eklenmeli.
func TestGocV1denV2VeriKorunur(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`CREATE TABLE agents(name TEXT PRIMARY KEY,token_hash TEXT NOT NULL UNIQUE,kind TEXT NOT NULL CHECK(kind IN ('ajan','insan')),created_at TEXT NOT NULL,revoked_at TEXT,last_seen TEXT)`,
		`CREATE TABLE agent_rooms(agent TEXT NOT NULL REFERENCES agents(name),room TEXT NOT NULL,PRIMARY KEY(agent,room))`,
		`CREATE TABLE threads(room TEXT NOT NULL,id TEXT NOT NULL,status TEXT NOT NULL,turns INTEGER NOT NULL,turn_limit INTEGER NOT NULL,created_by TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(room,id))`,
		`CREATE TABLE thread_parties(room TEXT NOT NULL,thread TEXT NOT NULL,agent TEXT NOT NULL,PRIMARY KEY(room,thread,agent))`,
		`CREATE TABLE messages(id INTEGER PRIMARY KEY AUTOINCREMENT,room TEXT NOT NULL,from_agent TEXT NOT NULL,to_agent TEXT NOT NULL DEFAULT '',type TEXT NOT NULL,thread TEXT NOT NULL,reply_to INTEGER,turn INTEGER NOT NULL,body TEXT NOT NULL,created_at TEXT NOT NULL)`,
		`CREATE INDEX messages_room_id ON messages(room,id)`, `CREATE INDEX messages_thread ON messages(room,thread,id)`, `CREATE INDEX messages_created ON messages(created_at)`,
		`INSERT INTO agents VALUES ('ali','hash','insan','2026-01-01T00:00:00Z',NULL,'2026-02-01T00:00:00Z')`,
		`INSERT INTO agent_rooms VALUES ('ali','r1')`,
		`INSERT INTO threads VALUES ('r1','T-1','acik',1,20,'ali','2026-01-01T00:00:00Z')`,
		`INSERT INTO thread_parties VALUES ('r1','T-1','ali')`,
		`INSERT INTO messages VALUES (7,'r1','ali','','bilgi','T-1',NULL,1,'gerçek mesaj','2026-01-01T00:00:00Z')`,
		`PRAGMA user_version=1`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != schemaVersion {
		t.Fatalf("sürüm=%d hata=%v", v, err)
	}
	var name, hash, kind, created, seen string
	var revoked, secret sql.NullString
	var yes bool
	var step int64
	if err := s.db.QueryRow(`SELECT name,token_hash,kind,created_at,revoked_at,last_seen,yonetici,totp_secret,totp_last_step FROM agents`).Scan(&name, &hash, &kind, &created, &revoked, &seen, &yes, &secret, &step); err != nil {
		t.Fatal(err)
	}
	if name != "ali" || hash != "hash" || kind != "insan" || created != "2026-01-01T00:00:00Z" || seen != "2026-02-01T00:00:00Z" || revoked.Valid || secret.Valid || yes || step != 0 {
		t.Fatal("hesap verisi veya varsayılanlar değişti")
	}
	ctx := context.Background()
	member, err := s.HasRoom(ctx, "ali", "r1")
	if err != nil || !member {
		t.Fatal("üyelik kayboldu")
	}
	msgs, err := s.RoomLog(ctx, "r1", 0, 100)
	if err != nil || len(msgs) != 1 || msgs[0].ID != 7 || msgs[0].Body != "gerçek mesaj" || msgs[0].From != "ali" || msgs[0].Thread != "T-1" {
		t.Fatalf("mesaj değişti: %+v %v", msgs, err)
	}
	th, err := s.GetThread(ctx, "r1", "T-1")
	if err != nil || th.Turns != 1 || th.TurnLimit != 20 {
		t.Fatal("konuşma kayboldu")
	}
	party, err := s.IsThreadParty(ctx, "r1", "T-1", "ali")
	if err != nil || !party {
		t.Fatal("taraf kayboldu")
	}
	if err := s.migrate(); err != nil {
		t.Fatal("v2 yeniden açılış", err)
	}
}

// NEDEN: bilinmeyen/iptal hesap ayrımı ve idempotent üyelik işlemleri korunmalı.
func TestYonetimStoreHatalar(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	for _, fn := range []func() error{func() error { return s.AddRoom(ctx, "yok", "r") }, func() error { return s.RemoveRoom(ctx, "yok", "r") }, func() error { return s.SetYonetici(ctx, "yok", "s") }, func() error { return s.ClearYonetici(ctx, "yok") }} {
		if !errors.Is(fn(), ErrNotFound) {
			t.Fatal("bilinmeyen hesap kabul edildi")
		}
	}
	if _, _, err := s.TOTPState(ctx, "yok"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.MarkTOTPStep(ctx, "yok", 1); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.AddAgent(ctx, "a", "insan", HashToken("sentetik"), nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.AddRoom(ctx, "a", "r"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := s.RemoveRoom(ctx, "a", "r"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetYonetici(ctx, "a", "TEST"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.MarkTOTPStep(ctx, "a", 100)
			if err != nil {
				t.Error(err)
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatal("atomik adım kabul sayısı", accepted)
	}
	if ok, err := s.MarkTOTPStep(ctx, "a", 99); err != nil || ok {
		t.Fatal("eski adım kabul edildi")
	}
	if err := s.RevokeAgent(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRoom(ctx, "a", "r"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

// NEDEN: denetim kaydı yazılamayan yönetim değişikliği commit edilmemeli.
func TestYonetimKayitAtomik(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "a", "insan", HashToken("sentetik"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER kayit_hata BEFORE INSERT ON yonetim_kayit BEGIN SELECT RAISE(ABORT,'test'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangeAccount(ctx, "ssh", "odaya-ekle", "a", "r", "", false); err == nil {
		t.Fatal("kayıt hatası kayboldu")
	}
	member, err := s.HasRoom(ctx, "a", "r")
	if err != nil || member {
		t.Fatal("kayıtsız değişiklik")
	}
	if err := s.AddAgentWithKayit(ctx, "b", "ajan", HashToken("sentetik-b"), []string{"r"}, "ssh", false); err == nil {
		t.Fatal("kayıtsız hesap")
	}
	if _, err := s.Authenticate(ctx, "sentetik-b"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("hesap geri alınmadı")
	}
}

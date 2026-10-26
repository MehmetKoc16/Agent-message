package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGocV3OdaAdlariniVeUyeligiKorur(t *testing.T) {
	// NEDEN: mevcut odalar başlık desteği eklenince kaybolmamalı veya herkese açılmamalı.
	path := filepath.Join(t.TempDir(), "v3.db")
	ctx := context.Background()
	s, err := Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddAgent(ctx, "ali", "ajan", HashToken("test-only"), []string{"agent-message", "proje"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`DROP TABLE rooms`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`PRAGMA user_version=3`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`ALTER TABLE threads DROP COLUMN public_turns`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rooms, err := s.RoomsForAgent(ctx, "ali")
	if err != nil || len(rooms) != 2 || rooms[0].Name != "agent-message" || rooms[0].Title != "agent-message" || rooms[1].Title != "proje" {
		t.Fatal(rooms, err)
	}
	if err = s.migrate(); err != nil {
		t.Fatal("tekrar açılış", err)
	}
}

func TestOdaOlusturmaYoneticiUyeVeKaydiBirlikteYazar(t *testing.T) {
	// NEDEN: oda yalnız seçilen üyelerle açılmalı, rol denetimi transaction içinde olmalı.
	s := openTemp(t)
	ctx := context.Background()
	for _, name := range []string{"admin", "davetli", "diger"} {
		if err := s.AddAgent(ctx, name, "insan", HashToken(name), nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateRoom(ctx, "admin", "Başlık", nil); err != ErrUnauthorized {
		t.Fatal(err)
	}
	if err := s.SetYonetici(ctx, "admin", "test-secret"); err != nil {
		t.Fatal(err)
	}
	room, err := s.CreateRoom(ctx, "admin", "  Admin Sohbeti  ", []string{"davetli", "davetli"})
	if err != nil || room.Title != "Admin Sohbeti" || room.Name == room.Title {
		t.Fatal(room, err)
	}
	for _, tc := range []struct {
		name  string
		count int
	}{{"admin", 1}, {"davetli", 1}, {"diger", 0}} {
		rooms, err := s.RoomsForAgent(ctx, tc.name)
		if err != nil || len(rooms) != tc.count {
			t.Fatal(tc, rooms, err)
		}
	}
	if err := s.AddRoom(ctx, "diger", room.Name); err != nil {
		t.Fatal(err)
	}
	rooms, err := s.RoomsForAgent(ctx, "diger")
	if err != nil || rooms[0].Title != "Admin Sohbeti" {
		t.Fatal(rooms, err)
	}
	var creates, members int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM yonetim_kayit WHERE islem='oda_olusturuldu' AND hedef=?`, room.Name).Scan(&creates); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM yonetim_kayit WHERE islem='odaya_eklendi' AND ayrinti=?`, room.Name).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if creates != 1 || members != 2 {
		t.Fatal(creates, members)
	}
	if err := s.ClearYonetici(ctx, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRoom(ctx, "admin", "Yetkisiz", nil); err != ErrUnauthorized {
		t.Fatal(err)
	}
}

func TestGecersizUyedeOdaVeUyelikYazilmaz(t *testing.T) {
	// NEDEN: kötü üye listesinin bir kısmı geçerli olsa bile yarım oda oluşmamalı.
	s := openTemp(t)
	ctx := context.Background()
	if err := s.AddAgent(ctx, "admin", "insan", HashToken("admin"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetYonetici(ctx, "admin", "test-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRoom(ctx, "admin", "Başlık", []string{"admin", "olmayan"}); err != ErrBadMember {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM rooms`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	rooms, err := s.RoomsForAgent(ctx, "admin")
	if err != nil || len(rooms) != 0 {
		t.Fatal(rooms, err)
	}
}

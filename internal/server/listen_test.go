package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/MehmetKoc16/Agent-message/internal/config"
)

// NEDEN: gerçek süreç ortamını okumadan aktivasyon ve temizleme sınanmalı.
func Test_systemd_ortam_dogrulamasi(t *testing.T) {
	for _, tc := range []struct {
		pid, fds string
		ok       bool
	}{
		{"42", "1", true}, {"43", "1", false}, {"42", "2", false}, {"42", "0", false}, {"", "1", false}, {"42", "", false}, {"", "", false},
	} {
		var read, cleaned []string
		err := systemdOrtam(func(k string) string {
			read = append(read, k)
			if k == "LISTEN_PID" {
				return tc.pid
			}
			if k == "LISTEN_FDS" {
				return tc.fds
			}
			t.Fatalf("yasak ortam okuması: %s", k)
			return ""
		}, func(k string) error { cleaned = append(cleaned, k); return nil }, 42)
		if (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc, err)
		}
		if !reflect.DeepEqual(read, []string{"LISTEN_PID", "LISTEN_FDS"}) || !reflect.DeepEqual(cleaned, []string{"LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES"}) {
			t.Fatal(read, cleaned)
		}
	}
}

// NEDEN: gerçek AF_UNIX HTTP alışverişi ve kapanışta silme Windows'ta da çalışmalı.
func Test_unix_sokette_dinler_ve_cevap_verir(t *testing.T) {
	e := newEnv(t, nil)
	c := e.cfg
	c.Host, c.Port = "", 0
	path := filepath.Join(t.TempDir(), "s")
	c.Listen = "unix:" + path
	ln, err := Listen(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.srv.Serve(ctx, ln) }()
	t.Cleanup(func() { cancel(); _ = ln.Close() })
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 || string(body) != "{\"ok\":true}\n" {
		t.Fatalf("%d %s %v", resp.StatusCode, body, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("kapanmadı")
	}
	_ = ln.Close()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("soket silinmedi: %v", err)
	}
}

// NEDEN: soket yolu normal dosyaya işaret ederse veri silinmemeli.
func Test_unix_soket_disinda_dosya_varsa_baslamaz(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s")
	if err := os.WriteFile(path, []byte("koru"), 0600); err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.DB = "test.db"
	c.Host, c.Port = "", 0
	c.Listen = "unix:" + path
	if ln, err := Listen(c); err == nil {
		ln.Close()
		t.Fatal("normal dosya kabul edildi")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "koru" {
		t.Fatal("dosya değişti", err)
	}
}

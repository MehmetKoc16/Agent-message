package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/MehmetKoc16/Agent-message/internal/config"
)

// systemdOrtam yalnız iki aktivasyon değişkenini okur. Enjeksiyon testlerin gerçek
// süreç ortamına erişmesini önler; isimler dahil mirası her sonuçta temizler.
func systemdOrtam(get func(string) string, unset func(string) error, pid int) error {
	p, f := get("LISTEN_PID"), get("LISTEN_FDS")
	var clean error
	for _, name := range []string{"LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES"} {
		clean = errors.Join(clean, unset(name))
	}
	if p != strconv.Itoa(pid) || f != "1" {
		return errors.New("systemd: LISTEN_PID bu süreç olmalı ve LISTEN_FDS tam 1 olmalı")
	}
	return clean
}

// Listen dinleyiciyi açar; systemd soketinin ömrü PID 1'e aittir.
func Listen(c config.Server) (net.Listener, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	switch {
	case c.Listen == "systemd":
		if err := systemdOrtam(os.Getenv, os.Unsetenv, os.Getpid()); err != nil {
			return nil, err
		}
		f := os.NewFile(3, "systemd-relay")
		if f == nil {
			return nil, errors.New("systemd: fd 3 yok")
		}
		ln, err := net.FileListener(f)
		_ = f.Close() // FileListener kendi kopyasını tutar.
		if err != nil {
			return nil, fmt.Errorf("systemd fd 3: %w", err)
		}
		u, ok := ln.(*net.UnixListener)
		if !ok {
			_ = ln.Close()
			return nil, errors.New("systemd: fd 3 Unix dinleyici olmalı")
		}
		u.SetUnlinkOnClose(false)
		return u, nil
	case strings.HasPrefix(c.Listen, "unix:"):
		path := strings.TrimPrefix(c.Listen, "unix:")
		fi, err := os.Lstat(path)
		if err == nil {
			if fi.Mode()&os.ModeSocket == 0 {
				return nil, errors.New("unix: hedef var ve soket değil")
			}
			if err := os.Remove(path); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			return nil, err
		}
		if err := os.Chmod(path, 0o660); err != nil {
			_ = ln.Close()
			return nil, err
		}
		return ln, nil // ListenUnix kapanışta dosyayı siler.
	default:
		return net.Listen("tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	}
}

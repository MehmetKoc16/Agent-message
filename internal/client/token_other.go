//go:build !windows && !linux

package client

import (
	"errors"
	"os"
)

func setupEcho(f *os.File) (bool, func() error, error) {
	st, err := f.Stat()
	terminal := err == nil && st.Mode()&os.ModeCharDevice != 0
	return terminal, func() error { return nil }, errors.New("bu platformda terminal yansıtması kapatılamıyor")
}

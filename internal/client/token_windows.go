//go:build windows

package client

import (
	"os"
	"syscall"
	"unsafe"
)

func setupEcho(f *os.File) (bool, func() error, error) {
	k := syscall.NewLazyDLL("kernel32.dll")
	get, set := k.NewProc("GetConsoleMode"), k.NewProc("SetConsoleMode")
	var old uint32
	noop := func() error { return nil }
	ok, _, err := get.Call(f.Fd(), uintptr(unsafe.Pointer(&old)))
	if ok == 0 {
		return false, noop, err // boru/dosya konsol değildir
	}
	const enableEchoInput = 0x0004
	ok, _, err = set.Call(f.Fd(), uintptr(old&^enableEchoInput))
	if ok == 0 {
		return true, noop, err
	}
	return true, func() error {
		ok, _, err := set.Call(f.Fd(), uintptr(old))
		if ok == 0 {
			return err
		}
		return nil
	}, nil
}

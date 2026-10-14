//go:build linux

package client

import (
	"os"
	"syscall"
	"unsafe"
)

func setupEcho(f *os.File) (bool, func() error, error) {
	var old syscall.Termios
	noop := func() error { return nil }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&old)))
	if errno != 0 {
		return false, noop, errno // boru/dosya terminal değildir
	}
	mode := old
	mode.Lflag &^= syscall.ECHO
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&mode)))
	if errno != 0 {
		return true, noop, errno
	}
	return true, func() error {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&old)))
		if errno != 0 {
			return errno
		}
		return nil
	}, nil
}

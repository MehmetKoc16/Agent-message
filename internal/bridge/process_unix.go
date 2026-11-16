//go:build unix

package bridge

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// isolate ajanı kendi süreç grubunda başlatır; zaman aşımı ya da iptalde yalnız ana
// süreç değil, başlattığı alt süreçler de (grubu terk etmedilerse) öldürülür.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

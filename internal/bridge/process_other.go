//go:build !unix

package bridge

import "os/exec"

// Unix dışında süreç grubu kurulmaz: iptalde yalnız ana süreç öldürülür (exec varsayılanı).
func isolate(*exec.Cmd) {}

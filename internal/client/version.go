package client

import (
	"fmt"
	"runtime"
)

// Yayın betiği bu değerleri -ldflags -X ile doldurur.
var (
	version = "dev"
	commit  = "dev"
)

func runVersion(args []string, env Env) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "kullanım: relay version")
		return ExitError
	}
	fmt.Fprintf(env.Stdout, "relay sürüm=%s commit=%s Go=%s\n", version, commit, runtime.Version())
	return ExitOK
}

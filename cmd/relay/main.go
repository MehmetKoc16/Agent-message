// relay: Agent-message istemcisi (setup, version, doctor, send, wait, inbox, agents, thread, close).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/MehmetKoc16/Agent-message/internal/client"
)

func main() {
	restore := client.SetupConsole()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := client.Run(ctx, os.Args[1:], client.Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr})
	stop()
	restore()
	os.Exit(code)
}

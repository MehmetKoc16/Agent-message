//go:build !windows

package client

// SetupConsole Windows dışında gerekmez.
func SetupConsole() func() { return func() {} }

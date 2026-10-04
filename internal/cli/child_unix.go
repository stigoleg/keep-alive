//go:build !windows

package cli

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// runSignals are handled by `keepalive run`.
func runSignals() []os.Signal { return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} }

// forwardSignal reports whether sig should be passed on to the child. A
// Ctrl+C typed into the terminal already reached the child, which shares our
// foreground process group, so only a SIGINT from elsewhere is forwarded.
func forwardSignal(sig os.Signal) bool {
	if sig != syscall.SIGINT {
		return true
	}
	pgrp, err := unix.IoctlGetInt(int(os.Stdin.Fd()), unix.TIOCGPGRP)
	return err != nil || pgrp != syscall.Getpgrp()
}

func childExitCode(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

func signalExitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 1
}

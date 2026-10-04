//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// stopSignals end the session gracefully. SIGTSTP (Ctrl+Z) is included so a
// suspended process never keeps holding the power assertion.
func stopSignals() []os.Signal {
	return []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGTSTP}
}

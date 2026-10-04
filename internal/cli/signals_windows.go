//go:build windows

package cli

import (
	"os"
	"syscall"
)

func stopSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

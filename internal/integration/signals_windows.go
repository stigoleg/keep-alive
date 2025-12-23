//go:build windows
// +build windows

package integration

import (
	"os"
	"syscall"
)

func getUnixSignals() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
	}
}

func getUnixSignalsWithSIGTSTP() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
	}
}

func sendSIGTSTP(proc *os.Process) error {
	// SIGTSTP not available on Windows, use SIGTERM instead
	return proc.Signal(syscall.SIGTERM)
}


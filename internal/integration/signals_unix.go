//go:build !windows
// +build !windows

package integration

import (
	"os"
	"syscall"
)

func getUnixSignals() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	}
}

func getUnixSignalsWithSIGTSTP() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGTSTP,
	}
}

func sendSIGTSTP(proc *os.Process) error {
	return proc.Signal(syscall.SIGTSTP)
}


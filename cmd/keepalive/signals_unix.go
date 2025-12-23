//go:build !windows
// +build !windows

package main

import (
	"os"
	"syscall"
)

func getSignalsForPlatform() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGTSTP,
	}
}

func isSIGTSTPForPlatform(sig os.Signal) bool {
	return sig == syscall.SIGTSTP
}


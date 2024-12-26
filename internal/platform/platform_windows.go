//go:build windows

package platform

import (
	"context"
	"syscall"
)

const (
	esSystemRequired = 0x00000001
	esContinuous     = 0x80000000
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

func setWindowsKeepAlive() error {
	r1, _, err := procSetThreadExecutionState.Call(uintptr(esSystemRequired | esContinuous))
	if r1 == 0 {
		return err
	}
	return nil
}

func stopWindowsKeepAlive() error {
	r1, _, err := procSetThreadExecutionState.Call(uintptr(esContinuous))
	if r1 == 0 {
		return err
	}
	return nil
}

// windowsKeepAlive implements the KeepAlive interface for Windows
type windowsKeepAlive struct{}

// Start initiates the keep-alive functionality
func (k *windowsKeepAlive) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		stopWindowsKeepAlive()
	}()
	return setWindowsKeepAlive()
}

// Stop terminates the keep-alive functionality
func (k *windowsKeepAlive) Stop() error {
	return stopWindowsKeepAlive()
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &windowsKeepAlive{}, nil
}

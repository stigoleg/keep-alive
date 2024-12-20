//go:build windows

package platform

import (
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

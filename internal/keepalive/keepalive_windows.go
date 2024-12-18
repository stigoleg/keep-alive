//go:build windows

package keepalive

import (
	"golang.org/x/sys/windows"
)

var (
	modkernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procSetThreadExecutionState = modkernel32.NewProc("SetThreadExecutionState")
)

// setWindowsKeepAlive sets thread execution state to prevent sleep.
func setWindowsKeepAlive() error {
	const ES_CONTINUOUS = 0x80000000
	const ES_SYSTEM_REQUIRED = 0x00000001
	const ES_DISPLAY_REQUIRED = 0x00000002

	r, _, err := procSetThreadExecutionState.Call(uintptr(ES_CONTINUOUS | ES_SYSTEM_REQUIRED | ES_DISPLAY_REQUIRED))
	if r == 0 {
		return err
	}
	return nil
}

// stopWindowsKeepAlive resets the thread execution state.
func stopWindowsKeepAlive() error {
	const ES_CONTINUOUS = 0x80000000
	r, _, err := procSetThreadExecutionState.Call(uintptr(ES_CONTINUOUS))
	if r == 0 {
		return err
	}
	return nil
}

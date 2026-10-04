package service

import (
	"fmt"
	"os"
	"os/user"
	"syscall"

	"golang.org/x/sys/windows"
)

func newManager() (Manager, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("service: current user: %w", err)
	}
	return &schtasks{run: execRunner{timeout: CommandTimeout}, user: u.Username, tempDir: os.TempDir()}, nil
}

// sysProcAttr keeps schtasks from flashing a console window.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
}

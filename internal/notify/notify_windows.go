package notify

import (
	"context"
	"os/exec"
	"syscall"
)

// createNoWindow keeps PowerShell from opening a console window.
const createNoWindow = 0x08000000

type toast struct{}

func newPlatform() Notifier { return toast{} }

// Notify shows a toast attributed to Windows PowerShell.
func (toast) Notify(ctx context.Context, title, body string) error {
	return run(ctx, "powershell.exe", powershellArgs(toastScript(title, body)), func(cmd *exec.Cmd) {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
	})
}

func available() (bool, string) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		return false, "powershell.exe not found"
	}
	return true, "toast notifications via Windows PowerShell"
}

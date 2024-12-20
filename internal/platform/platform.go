package platform

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

type KeepAlive interface {
	Start(ctx context.Context) error
	Stop() error
}

func NewKeepAlive() (KeepAlive, error) {
	switch runtime.GOOS {
	case "darwin":
		return &darwinKeepAlive{}, nil
	case "linux":
		return &linuxKeepAlive{}, nil
	case "windows":
		return &windowsKeepAlive{}, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

type darwinKeepAlive struct {
	cmd *exec.Cmd
}

func (d *darwinKeepAlive) Start(ctx context.Context) error {
	d.cmd = exec.CommandContext(ctx, "caffeinate", "-dims")
	return d.cmd.Start()
}

func (d *darwinKeepAlive) Stop() error {
	if d.cmd != nil && d.cmd.Process != nil {
		return d.cmd.Process.Kill()
	}
	return nil
}

type linuxKeepAlive struct {
	cmd *exec.Cmd
}

func (l *linuxKeepAlive) Start(ctx context.Context) error {
	l.cmd = exec.CommandContext(ctx, "systemd-inhibit", "--what=idle", "--mode=block", "bash", "-c", "while true; do sleep 3600; done")
	return l.cmd.Start()
}

func (l *linuxKeepAlive) Stop() error {
	if l.cmd != nil && l.cmd.Process != nil {
		return l.cmd.Process.Kill()
	}
	return nil
}

type windowsKeepAlive struct{}

func (w *windowsKeepAlive) Start(ctx context.Context) error {
	return setWindowsKeepAlive()
}

func (w *windowsKeepAlive) Stop() error {
	return stopWindowsKeepAlive()
}

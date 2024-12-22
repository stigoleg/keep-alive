package platform

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"
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
	mu          sync.Mutex
	cmd         *exec.Cmd
	ctx         context.Context
	cancelFunc  context.CancelFunc
	assertTimer *time.Timer
	stopped     bool
}

func (d *darwinKeepAlive) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cmd != nil {
		return fmt.Errorf("keep-alive is already running")
	}

	d.ctx, d.cancelFunc = context.WithCancel(ctx)
	d.stopped = false

	// Start caffeinate with all necessary flags
	// -d: prevent display sleep
	// -i: prevent system idle sleep
	// -m: prevent disk idle sleep
	// -s: prevent system sleep on AC power
	// -u: declare user activity periodically
	d.cmd = exec.CommandContext(d.ctx, "caffeinate", "-dimsu")

	if err := d.cmd.Start(); err != nil {
		d.cleanup()
		return fmt.Errorf("failed to start caffeinate: %v", err)
	}

	// Start a goroutine to monitor the caffeinate process
	go func() {
		err := d.cmd.Wait()
		d.mu.Lock()
		defer d.mu.Unlock()
		if err != nil && d.ctx.Err() == nil && !d.stopped {
			fmt.Printf("caffeinate process exited unexpectedly: %v\n", err)
			// Attempt to restart caffeinate
			d.cmd = exec.CommandContext(d.ctx, "caffeinate", "-dimsu")
			d.cmd.Start()
		}
	}()

	// Create and start the assertion timer
	d.assertTimer = time.NewTimer(time.Second)
	go func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			case <-d.assertTimer.C:
				d.assertActivity()
				d.assertTimer.Reset(30 * time.Second)
			}
		}
	}()

	return nil
}

func (d *darwinKeepAlive) assertActivity() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	// Use both methods to assert activity
	exec.CommandContext(d.ctx, "caffeinate", "-u").Run()

	// Also update power assertion
	exec.CommandContext(d.ctx, "pmset", "touch").Run()
}

func (d *darwinKeepAlive) cleanup() {
	if d.cancelFunc != nil {
		d.cancelFunc()
		d.cancelFunc = nil
	}

	if d.assertTimer != nil {
		d.assertTimer.Stop()
		d.assertTimer = nil
	}

	if d.cmd != nil && d.cmd.Process != nil {
		d.cmd.Process.Kill()
		d.cmd.Process.Release()
		d.cmd = nil
	}

	// Force kill any remaining caffeinate processes
	exec.Command("pkill", "-9", "caffeinate").Run()

	// Restore default power management settings
	restoreCommands := [][]string{
		{"pmset", "displaysleep", "2"},
		{"pmset", "sleep", "1"},
	}

	for _, cmd := range restoreCommands {
		exec.Command(cmd[0], cmd[1:]...).Run()
	}
}

func (d *darwinKeepAlive) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	d.cleanup()
	return nil
}

type linuxKeepAlive struct {
	cmds       []*exec.Cmd
	ctx        context.Context
	cancelFunc context.CancelFunc
}

func (l *linuxKeepAlive) Start(ctx context.Context) error {
	l.ctx, l.cancelFunc = context.WithCancel(ctx)

	// Commands to prevent sleep and screen locking on Linux
	commands := [][]string{
		{"xset", "s", "off"},     // Disable screen saver
		{"xset", "-dpms"},        // Disable DPMS (Energy Star) features
		{"xset", "s", "noblank"}, // Disable screen blanking
		{"gsettings", "set", "org.gnome.desktop.session", "idle-delay", "0"}, // Disable idle timeout
	}

	// Execute each command
	for _, cmd := range commands {
		c := exec.CommandContext(l.ctx, cmd[0], cmd[1:]...)
		if err := c.Start(); err != nil {
			l.Stop() // Clean up any previously started commands
			continue // Try next command if this one fails
		}
		l.cmds = append(l.cmds, c)
	}

	// Start systemd-inhibit as a fallback
	inhibitCmd := exec.CommandContext(l.ctx, "systemd-inhibit", "--what=idle:sleep:handle-lid-switch", "--who=keepalive", "--why=Preventing system sleep", "--mode=block", "sleep", "infinity")
	if err := inhibitCmd.Start(); err == nil {
		l.cmds = append(l.cmds, inhibitCmd)
	}

	return nil
}

func (l *linuxKeepAlive) Stop() error {
	if l.cancelFunc != nil {
		l.cancelFunc()
		l.cancelFunc = nil
	}

	// Restore system settings
	restoreCommands := [][]string{
		{"xset", "s", "on"}, // Enable screen saver
		{"xset", "+dpms"},   // Enable DPMS
		{"gsettings", "reset", "org.gnome.desktop.session", "idle-delay"}, // Reset idle timeout
		{"pkill", "-f", "systemd-inhibit"},                                // Kill any remaining systemd-inhibit processes
	}

	// Execute restore commands
	for _, cmd := range restoreCommands {
		if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
			fmt.Printf("Warning: failed to restore setting with %v: %v\n", cmd[0], err)
		}
	}

	// Kill all running commands
	for i, cmd := range l.cmds {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Process.Release()
		}
		l.cmds[i] = nil
	}
	l.cmds = nil

	return nil
}

type windowsKeepAlive struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	cmd        *exec.Cmd
}

func (w *windowsKeepAlive) Start(ctx context.Context) error {
	w.ctx, w.cancelFunc = context.WithCancel(ctx)

	// Use PowerShell to prevent sleep and screen lock
	script := `
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;

public class Sleep {
    [DllImport("kernel32.dll", CharSet = CharSet.Auto, SetLastError = true)]
    public static extern uint SetThreadExecutionState(uint esFlags);

    public static void PreventSleep() {
        SetThreadExecutionState(
            0x80000002 | // ES_CONTINUOUS
            0x00000002 | // ES_DISPLAY_REQUIRED
            0x00000001   // ES_SYSTEM_REQUIRED
        );
    }
}
"@

while ($true) {
    [Sleep]::PreventSleep()
    Start-Sleep -Seconds 30
}
`
	w.cmd = exec.CommandContext(w.ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return w.cmd.Start()
}

func (w *windowsKeepAlive) Stop() error {
	if w.cancelFunc != nil {
		w.cancelFunc()
		w.cancelFunc = nil
	}

	// Kill the PowerShell process
	if w.cmd != nil && w.cmd.Process != nil {
		w.cmd.Process.Kill()
		w.cmd.Process.Release()
		w.cmd = nil
	}

	// Reset power settings using PowerShell
	resetScript := `
	powercfg /change monitor-timeout-ac 10
	powercfg /change standby-timeout-ac 30
	`
	exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", resetScript).Run()

	return nil
}

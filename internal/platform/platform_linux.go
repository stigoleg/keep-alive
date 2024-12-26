//go:build linux

package platform

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// linuxKeepAlive implements the KeepAlive interface for Linux
type linuxKeepAlive struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
}

// trySystemdInhibit attempts to use systemd-inhibit
func trySystemdInhibit(ctx context.Context) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "systemd-inhibit", "--what=idle:sleep:handle-lid-switch", 
		"--who=keep-alive", "--why=Prevent system sleep", "--mode=block",
		"sleep", "infinity")
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// tryXsetMethod attempts to use xset to prevent screen sleep
func tryXsetMethod() error {
	// Disable screen saver
	if err := exec.Command("xset", "s", "off").Run(); err != nil {
		return err
	}
	// Disable DPMS (Display Power Management Signaling)
	if err := exec.Command("xset", "-dpms").Run(); err != nil {
		return err
	}
	return nil
}

// tryGnomeMethod attempts to use gsettings to prevent idle
func tryGnomeMethod() error {
	settings := []struct {
		key   string
		value string
	}{
		{"org.gnome.desktop.session idle-delay", "0"},
		{"org.gnome.settings-daemon.plugins.power sleep-inactive-ac-type", "nothing"},
		{"org.gnome.settings-daemon.plugins.power sleep-inactive-battery-type", "nothing"},
	}

	for _, s := range settings {
		if err := exec.Command("gsettings", "set", s.key, s.value).Run(); err != nil {
			return err
		}
	}
	return nil
}

// Start initiates the keep-alive functionality
func (k *linuxKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	// Create a cancellable context
	ctx, k.cancel = context.WithCancel(ctx)

	// Try systemd-inhibit first
	cmd, err := trySystemdInhibit(ctx)
	if err == nil {
		k.cmd = cmd
		k.wg.Add(1)
		go func() {
			defer k.wg.Done()
			k.cmd.Wait()
		}()
	} else {
		// Fallback to xset and GNOME methods
		if err := tryXsetMethod(); err != nil {
			// If xset fails, try GNOME method
			if err := tryGnomeMethod(); err != nil {
				k.cancel()
				return err
			}
		}
	}

	// Start periodic activity simulation
	k.activityTick = time.NewTicker(30 * time.Second)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.activityTick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-k.activityTick.C:
				// Simulate user activity
				exec.Command("xdotool", "mousemove_relative", "1", "0").Run()
				exec.Command("xdotool", "mousemove_relative", "-1", "0").Run()
			}
		}
	}()

	k.isRunning = true
	return nil
}

// Stop terminates the keep-alive functionality
func (k *linuxKeepAlive) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isRunning {
		return nil
	}

	if k.cancel != nil {
		k.cancel()
	}

	if k.cmd != nil && k.cmd.Process != nil {
		k.cmd.Process.Kill()
	}

	// Restore default settings
	exec.Command("xset", "s", "on").Run()
	exec.Command("xset", "+dpms").Run()

	// Wait for all goroutines to finish
	k.wg.Wait()

	k.isRunning = false
	return nil
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &linuxKeepAlive{}, nil
}

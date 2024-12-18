package keepalive

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Keeper manages the keep-alive state across platforms.
type Keeper struct {
	mu           sync.Mutex
	cancel       context.CancelFunc
	platformStop func() error
	running      bool
}

// StartIndefinite keeps the system awake indefinitely using platform-specific methods.
func (k *Keeper) StartIndefinite() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.running {
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		// Windows-specific logic in keepalive_windows.go
		if err := setWindowsKeepAlive(); err != nil {
			return err
		}
		k.platformStop = stopWindowsKeepAlive

	case "darwin":
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "caffeinate", "-dims")
		if err := cmd.Start(); err != nil {
			cancel()
			return err
		}
		k.cancel = func() {
			cancel()
			_ = cmd.Wait()
		}
		k.platformStop = func() error { return nil }

	case "linux":
		ctx, cancel := context.WithCancel(context.Background())
		// systemd-inhibit prevents the system from going idle/sleep.
		cmd := exec.CommandContext(ctx, "systemd-inhibit", "--what=idle", "--mode=block", "bash", "-c", "while true; do sleep 3600; done")
		if err := cmd.Start(); err != nil {
			cancel()
			return err
		}
		k.cancel = func() {
			cancel()
			_ = cmd.Wait()
		}
		k.platformStop = func() error { return nil }

	default:
		return errors.New("unsupported platform")
	}

	k.running = true
	return nil
}

// StartTimed keeps the system awake for the specified number of minutes, then stops.
func (k *Keeper) StartTimed(minutes int) error {
	if minutes <= 0 {
		return errors.New("minutes must be > 0")
	}
	if err := k.StartIndefinite(); err != nil {
		return err
	}

	go func() {
		time.Sleep(time.Duration(minutes) * time.Minute)
		_ = k.Stop()
	}()
	return nil
}

// Stop stops keeping the system awake, restoring normal behavior.
func (k *Keeper) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.running {
		return nil
	}
	if k.cancel != nil {
		k.cancel()
	}
	if k.platformStop != nil {
		if err := k.platformStop(); err != nil {
			return err
		}
	}
	k.running = false
	return nil
}

// IsRunning returns whether the system is currently being kept awake.
func (k *Keeper) IsRunning() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.running
}

package keepalive

import (
	"errors"
	"sync"
	"time"
)

// Keeper manages the system's keep-alive state
type Keeper struct {
	running bool
	mu      sync.Mutex
	timer   *time.Timer
}

// IsRunning returns whether the keep-alive is currently active
func (k *Keeper) IsRunning() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.running
}

// StartIndefinite starts keeping the system alive indefinitely
func (k *Keeper) StartIndefinite() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.running {
		return errors.New("keep-alive already running")
	}

	if err := setWindowsKeepAlive(); err != nil {
		return err
	}

	k.running = true
	return nil
}

// StartTimed starts keeping the system alive for the specified duration
func (k *Keeper) StartTimed(d time.Duration) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.running {
		return errors.New("keep-alive already running")
	}

	if err := setWindowsKeepAlive(); err != nil {
		return err
	}

	k.running = true
	k.timer = time.AfterFunc(d, func() {
		k.Stop()
	})

	return nil
}

// Stop stops keeping the system alive
func (k *Keeper) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.running {
		return nil
	}

	if k.timer != nil {
		k.timer.Stop()
		k.timer = nil
	}

	if err := stopWindowsKeepAlive(); err != nil {
		return err
	}

	k.running = false
	return nil
}

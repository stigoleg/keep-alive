package keepalive

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/stigoleg/keep-alive/internal/platform"
)

// Keeper manages the system's keep-alive state
type Keeper struct {
	running bool
	mu      sync.Mutex
	timer   *time.Timer
	keeper  platform.KeepAlive
	ctx     context.Context
	cancel  context.CancelFunc
	endTime time.Time

	simulateActivity bool
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

	// Initialize the platform-specific keeper if needed
	if k.keeper == nil {
		var err error
		k.keeper, err = platform.NewKeepAlive()
		if err != nil {
			return err
		}
	}

	// Create a new context for this session
	k.ctx, k.cancel = context.WithCancel(context.Background())

	// Start the platform-specific keep-alive
	k.keeper.SetSimulateActivity(k.simulateActivity)
	if err := k.keeper.Start(k.ctx); err != nil {
		k.cancel()
		return err
	}

	k.running = true
	log.Printf("keeper: started (indefinite)")
	return nil
}

// StartTimed starts keeping the system alive for the specified duration
func (k *Keeper) StartTimed(d time.Duration) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.running {
		return errors.New("keep-alive already running")
	}

	// Initialize the platform-specific keeper if needed
	if k.keeper == nil {
		var err error
		k.keeper, err = platform.NewKeepAlive()
		if err != nil {
			return err
		}
	}

	// Create a new context for this session
	k.ctx, k.cancel = context.WithTimeout(context.Background(), d)

	// Start the platform-specific keep-alive
	k.keeper.SetSimulateActivity(k.simulateActivity)
	if err := k.keeper.Start(k.ctx); err != nil {
		k.cancel()
		return err
	}

	k.running = true
	k.endTime = time.Now().Add(d)
	k.timer = time.AfterFunc(d, func() {
		k.Stop()
	})

	log.Printf("keeper: started (timed=%s)", d)
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

	if k.cancel != nil {
		k.cancel()
		k.cancel = nil
	}

	if k.keeper != nil {
		if err := k.keeper.Stop(); err != nil {
			return err
		}
	}

	k.running = false
	log.Printf("keeper: stopped")
	return nil
}

// TimeRemaining returns the remaining duration for timed mode
func (k *Keeper) TimeRemaining() time.Duration {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.running {
		return 0
	}

	if k.endTime.IsZero() {
		return 0
	}

	remaining := time.Until(k.endTime)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (k *Keeper) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.simulateActivity = simulate
}

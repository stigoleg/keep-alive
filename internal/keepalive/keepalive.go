package keepalive

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stigoleg/keep-alive/internal/platform"
)

// SimulationHealth represents the runtime health of activity simulation
type SimulationHealth int

const (
	SimulationHealthUnknown SimulationHealth = iota
	SimulationHealthOK
	SimulationHealthFailed
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

	// simulationFailCount tracks consecutive simulation failures (atomic for thread-safety)
	simulationFailCount int64
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
	return k.StopWithTimeout(0)
}

// StopWithTimeout stops keeping the system alive with a timeout
func (k *Keeper) StopWithTimeout(timeout time.Duration) error {
	k.mu.Lock()
	if !k.running {
		k.mu.Unlock()
		return nil
	}

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Stop timer and cancel context while holding the lock
	if k.timer != nil {
		k.timer.Stop()
		k.timer = nil
	}

	if k.cancel != nil {
		k.cancel()
		k.cancel = nil
	}

	// Capture keeper reference and mark as not running
	keeper := k.keeper
	k.running = false
	k.mu.Unlock()

	// Stop the platform keeper without holding the lock (may take time)
	if keeper == nil {
		log.Printf("keeper: stopped")
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- keeper.Stop()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("keeper: stopped with error: %v", err)
			return err
		}
		log.Printf("keeper: stopped")
		return nil
	case <-ctx.Done():
		log.Printf("keeper: stop timeout exceeded after %v", timeout)
		return ctx.Err()
	}
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

// GetSimulationHealth returns the current health of activity simulation
func (k *Keeper) GetSimulationHealth() SimulationHealth {
	failCount := atomic.LoadInt64(&k.simulationFailCount)
	if failCount > 0 {
		return SimulationHealthFailed
	}
	return SimulationHealthOK
}

// RecordSimulationFailure increments the simulation failure counter
func (k *Keeper) RecordSimulationFailure() {
	atomic.AddInt64(&k.simulationFailCount, 1)
}

// ResetSimulationHealth resets the simulation failure counter
func (k *Keeper) ResetSimulationHealth() {
	atomic.StoreInt64(&k.simulationFailCount, 0)
}

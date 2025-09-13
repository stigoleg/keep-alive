//go:build darwin

package platform

import (
	"context"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// darwinKeepAlive implements the KeepAlive interface for macOS
type darwinKeepAlive struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	activeMethod string
}

// Start initiates the keep-alive functionality
func (k *darwinKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	// Create a cancellable context
	ctx, k.cancel = context.WithCancel(ctx)

	// Capability probes
	if _, err := exec.LookPath("caffeinate"); err != nil {
		k.cancel()
		return err
	}
	pmsetAvailable := true
	if _, err := exec.LookPath("pmset"); err != nil {
		pmsetAvailable = false
		log.Printf("darwin: pmset not available; proceeding without pmset touch assertion")
	}

	// Start caffeinate with comprehensive flags
	k.cmd = exec.CommandContext(ctx, "caffeinate", "-s", "-d", "-m", "-i", "-u")
	k.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := k.cmd.Start(); err != nil {
		k.cancel()
		return err
	}

	// Start monitoring goroutine
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		k.cmd.Wait()
	}()

	// Start periodic activity assertion
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
				// Assert user activity using pmset when available
				if pmsetAvailable {
					exec.Command("pmset", "touch").Run()
				}
				// Additional caffeinate touch
				exec.Command("caffeinate", "-u", "-t", "1").Run()
			}
		}
	}()

	if pmsetAvailable {
		// Best-effort verification of assertion presence
		if out, err := exec.Command("pmset", "-g", "assertions").CombinedOutput(); err == nil {
			log.Printf("darwin: started keep-alive; pmset assertions bytes=%d", len(out))
		} else {
			log.Printf("darwin: pmset assertions check failed: %v", err)
		}
	}

	k.activeMethod = "caffeinate"
	if pmsetAvailable {
		k.activeMethod = "caffeinate+pmset"
	}
	log.Printf("darwin: active method: %s", k.activeMethod)

	k.isRunning = true
	return nil
}

func (k *darwinKeepAlive) killProcess() {
	if k.cmd == nil || k.cmd.Process == nil {
		return
	}

	pid := k.cmd.Process.Pid

	// Try SIGTERM first
	if err := k.cmd.Process.Signal(syscall.SIGTERM); err == nil {
		// Give it time to terminate gracefully with backoff
		done := make(chan struct{})
		go func() {
			k.cmd.Process.Wait()
			close(done)
		}()

		timeouts := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 200 * time.Millisecond}
		for _, to := range timeouts {
			select {
			case <-done:
				return
			case <-time.After(to):
				// continue waiting/backing off
			}
		}
	}

	// Process didn't terminate with SIGTERM, try SIGKILL
	k.cmd.Process.Kill()

	// Kill the process group as well
	syscall.Kill(-pid, syscall.SIGKILL)

	// Use pkill as a last resort
	exec.Command("pkill", "-9", "caffeinate").Run()
}

// Stop terminates the keep-alive functionality
func (k *darwinKeepAlive) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isRunning {
		return nil
	}

	// Cancel context and stop activity ticker
	if k.cancel != nil {
		k.cancel()
	}

	// Kill the process if it's still running
	k.killProcess()

	// Wait for all goroutines to finish
	k.wg.Wait()

	k.isRunning = false
	log.Printf("darwin: stopped; cleanup complete")
	return nil
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &darwinKeepAlive{}, nil
}

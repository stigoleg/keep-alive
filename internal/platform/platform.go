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

// runBestEffort executes a command ignoring any errors (best-effort)
func runBestEffort(name string, args ...string) {
	if err := exec.Command(name, args...).Run(); err != nil {
		log.Printf("darwin: best-effort command %s failed: %v", name, err)
	}
}

// run executes a command and returns any error
func run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// darwinKeepAlive implements the KeepAlive interface for macOS
type darwinKeepAlive struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	activeMethod string

	simulateActivity bool
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
					runBestEffort("pmset", "touch")
				}
				// Additional caffeinate touch
				runBestEffort("caffeinate", "-u", "-t", "1")

				if k.simulateActivity {
					// Use osascript to jitter the mouse by 1 pixel (non-intrusive)
					// This is a zero-dependency way to simulate HID activity on macOS
					script := `tell application "System Events"
						set {x, y} to mouse location of (mouse location)
						move mouse to {x + 1, y}
						move mouse to {x, y}
					end tell`
					runBestEffort("osascript", "-e", script)
				}
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
	} else {
		log.Printf("darwin: failed to send SIGTERM to process %d: %v", pid, err)
	}

	// Process didn't terminate with SIGTERM, try SIGKILL on the process
	if err := k.cmd.Process.Kill(); err != nil {
		log.Printf("darwin: failed to kill process %d: %v", pid, err)
	}

	// Kill the process group as well using negative pgid
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		log.Printf("darwin: failed to kill process group %d: %v", pid, err)
	}
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

func (k *darwinKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.simulateActivity = simulate
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &darwinKeepAlive{}, nil
}

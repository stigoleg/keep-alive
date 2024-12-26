//go:build darwin

package platform

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// darwinKeepAlive implements the KeepAlive interface for macOS
type darwinKeepAlive struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	isRunning bool
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

	// Start caffeinate in the background with its own process group
	k.cmd = exec.CommandContext(ctx, "caffeinate", "-i")
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
		// Give it a short time to terminate gracefully
		done := make(chan struct{})
		go func() {
			k.cmd.Process.Wait()
			close(done)
		}()
		
		select {
		case <-done:
			return
		case <-time.After(100 * time.Millisecond):
			// Process didn't terminate gracefully
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

	// Cancel context first
	if k.cancel != nil {
		k.cancel()
	}

	// Kill the process and its group
	k.killProcess()

	// Wait for monitoring goroutine to finish
	k.wg.Wait()

	k.isRunning = false
	return nil
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &darwinKeepAlive{}, nil
}

//go:build windows

package platform

import (
	"context"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
	esContinuous      = 0x80000000
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

// windowsKeepAlive implements the KeepAlive interface for Windows
type windowsKeepAlive struct {
	mu           sync.Mutex
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
}

func setWindowsKeepAlive() error {
	r1, _, err := procSetThreadExecutionState.Call(
		uintptr(esSystemRequired | esDisplayRequired | esContinuous),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

func stopWindowsKeepAlive() error {
	r1, _, err := procSetThreadExecutionState.Call(uintptr(esContinuous))
	if r1 == 0 {
		return err
	}
	return nil
}

func setPowerShellKeepAlive() error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", `
		$code = @"
		using System;
		using System.Runtime.InteropServices;

		public class Sleep {
			[DllImport("kernel32.dll", CharSet = CharSet.Auto, SetLastError = true)]
			public static extern uint SetThreadExecutionState(uint esFlags);
		}
"@

		Add-Type -TypeDefinition $code
		[Sleep]::SetThreadExecutionState(0x80000003)
	`)
	return cmd.Run()
}

// Start initiates the keep-alive functionality
func (k *windowsKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	// Create a cancellable context
	ctx, k.cancel = context.WithCancel(ctx)

	// Try primary method first
	err := setWindowsKeepAlive()
	if err != nil {
		// Fall back to PowerShell method
		err = setPowerShellKeepAlive()
		if err != nil {
			k.cancel()
			return err
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
				// Refresh the keep-alive state
				setWindowsKeepAlive()
			}
		}
	}()

	k.isRunning = true
	return nil
}

// Stop terminates the keep-alive functionality
func (k *windowsKeepAlive) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isRunning {
		return nil
	}

	if k.cancel != nil {
		k.cancel()
	}

	// Wait for activity goroutine to finish
	k.wg.Wait()

	k.isRunning = false
	return stopWindowsKeepAlive()
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &windowsKeepAlive{}, nil
}

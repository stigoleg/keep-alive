//go:build windows

package platform

import (
	"context"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// runBestEffort executes a command ignoring any errors (best-effort)
func runBestEffort(name string, args ...string) {
	if err := exec.Command(name, args...).Run(); err != nil {
		log.Printf("windows: best-effort command %s failed: %v", name, err)
	}
}

// run executes a command and returns any error
func run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

const (
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
	esContinuous      = 0x80000000

	inputMouse     = 0
	mouseEventMove = 0x0001
)

type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type input struct {
	inputType uint32
	mi        mouseInput
}

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	user32                      = syscall.NewLazyDLL("user32.dll")
	procSendInput               = user32.NewProc("SendInput")
)

// windowsKeepAlive implements the KeepAlive interface for Windows
type windowsKeepAlive struct {
	mu           sync.Mutex
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	activeMethod string

	simulateActivity bool
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
	return run("powershell", "-NoProfile", "-NonInteractive", "-Command", `
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
}

// Start initiates the keep-alive functionality
func (k *windowsKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	while_locked := func() error {
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
			k.activeMethod = "PowerShell"
		} else {
			k.activeMethod = "SetThreadExecutionState"
		}
		log.Printf("windows: active method: %s", k.activeMethod)

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

					if k.simulateActivity {
						// Simulate tiny mouse move: 1 pixel right, then 1 pixel left
						var i [2]input
						i[0].inputType = inputMouse
						i[0].mi = mouseInput{dx: 1, dy: 0, dwFlags: mouseEventMove}
						i[1].inputType = inputMouse
						i[1].mi = mouseInput{dx: -1, dy: 0, dwFlags: mouseEventMove}

						procSendInput.Call(
							uintptr(2),
							uintptr(unsafe.Pointer(&i[0])),
							uintptr(unsafe.Sizeof(i[0])),
						)
					}
				}
			}
		}()

		k.isRunning = true
		return nil
	}
	defer k.mu.Unlock()
	return while_locked()
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

func (k *windowsKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.simulateActivity = simulate
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &windowsKeepAlive{}, nil
}

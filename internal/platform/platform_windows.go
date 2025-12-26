//go:build windows

package platform

import (
	"context"
	"log"
	"math/rand"
	"os/exec"
	"sync"
	"sync/atomic"
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
	procGetLastInputInfo        = user32.NewProc("GetLastInputInfo")
	procGetTickCount            = kernel32.NewProc("GetTickCount")
)

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

func getIdleTime() (time.Duration, error) {
	var lii lastInputInfo
	lii.cbSize = uint32(unsafe.Sizeof(lii))
	r1, _, err := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii)))
	if r1 == 0 {
		return 0, err
	}

	r1, _, _ = procGetTickCount.Call()
	now := uint32(r1)

	// tick count wraps every 49.7 days, this subtraction handles it correctly for uint32
	idleMillis := now - lii.dwTime
	return time.Duration(idleMillis) * time.Millisecond, nil
}

type windowsCapabilities struct {
	nativeAPIAvailable  bool
	powerShellAvailable bool
}

// windowsKeepAlive implements the KeepAlive interface for Windows
type windowsKeepAlive struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	chatAppTick  *time.Ticker
	activeMethod string

	simulateActivity bool

	// last time we logged that user is active (to avoid spam)
	lastActiveLogNS int64

	// random source and pattern generator for natural mouse movements
	rnd        *rand.Rand
	patternGen *MousePatternGenerator
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

func detectWindowsCapabilities() windowsCapabilities {
	return windowsCapabilities{
		nativeAPIAvailable:  true, // Always available on Windows
		powerShellAvailable: hasCommandWindows("powershell"),
	}
}

func hasCommandWindows(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (k *windowsKeepAlive) activateKeepAliveMethod() error {
	err := setWindowsKeepAlive()
	if err != nil {
		// Fall back to PowerShell method
		err = setPowerShellKeepAlive()
		if err != nil {
			return err
		}
		k.activeMethod = "PowerShell"
	} else {
		k.activeMethod = "SetThreadExecutionState"
	}
	log.Printf("windows: active method: %s", k.activeMethod)
	return nil
}

func (k *windowsKeepAlive) startActivityTickerLocked(ctx context.Context) {
	k.activityTick = time.NewTicker(ActivityInterval)
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
}

func (k *windowsKeepAlive) startChatAppTickerLocked(ctx context.Context) {
	if !k.simulateActivity {
		return
	}

	k.chatAppTick = time.NewTicker(ChatAppActivityInterval)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.chatAppTick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-k.chatAppTick.C:
				k.simulateChatAppActivity()
			}
		}
	}()
}

func (k *windowsKeepAlive) simulateChatAppActivity() {
	idle, err := getIdleTime()
	if err != nil {
		log.Printf("windows: idle detection failed: %v", err)
		return
	}

	nowNS := time.Now().UnixNano()
	lastActiveLog := atomic.LoadInt64(&k.lastActiveLogNS)

	if idle <= IdleThreshold {
		// Log occasionally (every 2 minutes) that we're skipping due to active use
		if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > 2*time.Minute {
			atomic.StoreInt64(&k.lastActiveLogNS, nowNS)
			log.Printf("windows: user is active (idle: %v); skipping simulation to avoid interference", idle)
		}
		return
	}

	// User became idle - log if we were previously active
	if lastActiveLog != 0 {
		atomic.StoreInt64(&k.lastActiveLogNS, 0)
		log.Printf("windows: user became idle (%v); resuming activity simulation", idle)
	}

	points := k.patternGen.GenerateShapePoints()
	k.executeMousePattern(points)
}

func (k *windowsKeepAlive) executeMousePattern(points []MousePoint) {
	for i, pt := range points {
		dx := int32(pt.X)
		dy := int32(pt.Y)

		var inputEv input
		inputEv.inputType = inputMouse
		inputEv.mi = mouseInput{dx: dx, dy: dy, dwFlags: mouseEventMove}

		procSendInput.Call(
			uintptr(1),
			uintptr(unsafe.Pointer(&inputEv)),
			uintptr(unsafe.Sizeof(inputEv)),
		)

		distance := SegmentDistance(points, i)
		delay := k.patternGen.MovementDelay(distance)
		time.Sleep(delay)

		if k.patternGen.ShouldPause() {
			time.Sleep(k.patternGen.PauseDelay())
		}

		if k.patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := k.patternGen.IntermediatePoint(points, i, delay)
			var midInput input
			midInput.inputType = inputMouse
			midInput.mi = mouseInput{dx: int32(midPt.X), dy: int32(midPt.Y), dwFlags: mouseEventMove}
			procSendInput.Call(
				uintptr(1),
				uintptr(unsafe.Pointer(&midInput)),
				uintptr(unsafe.Sizeof(midInput)),
			)
			time.Sleep(midDelay)
		}
	}

	// Return to origin
	lastPt := points[len(points)-1]
	returnDelay := k.patternGen.ReturnDelay()
	var returnInput input
	returnInput.inputType = inputMouse
	returnInput.mi = mouseInput{dx: -int32(lastPt.X), dy: -int32(lastPt.Y), dwFlags: mouseEventMove}
	procSendInput.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&returnInput)),
		uintptr(unsafe.Sizeof(returnInput)),
	)
	time.Sleep(returnDelay)
}

// Start initiates the keep-alive functionality
func (k *windowsKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	k.ctx, k.cancel = context.WithCancel(ctx)

	// Initialize random source and pattern generator
	k.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	k.patternGen = NewMousePatternGenerator(k.rnd)

	// Activate keep-alive method
	if err := k.activateKeepAliveMethod(); err != nil {
		k.cancel()
		return err
	}

	k.startActivityTickerLocked(k.ctx)
	k.startChatAppTickerLocked(k.ctx)

	k.isRunning = true
	return nil
}

// Stop terminates the keep-alive functionality
func (k *windowsKeepAlive) Stop() error {
	k.mu.Lock()
	if !k.isRunning {
		k.mu.Unlock()
		return nil
	}

	if k.cancel != nil {
		k.cancel()
	}

	// Stop tickers first to prevent new operations
	if k.activityTick != nil {
		k.activityTick.Stop()
	}
	if k.chatAppTick != nil {
		k.chatAppTick.Stop()
		k.chatAppTick = nil
	}

	k.mu.Unlock()

	// Wait for activity goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		k.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("windows: all goroutines completed")
	case <-time.After(2 * time.Second):
		log.Printf("windows: warning: some goroutines did not complete within timeout")
	}

	// Reset keep-alive state
	var stopErr error
	if err := stopWindowsKeepAlive(); err != nil {
		log.Printf("windows: error resetting keep-alive state: %v", err)
		stopErr = err
	} else {
		log.Printf("windows: keep-alive state reset successfully")
	}

	k.mu.Lock()
	k.isRunning = false
	k.ctx = nil
	k.cancel = nil
	k.activityTick = nil
	k.mu.Unlock()

	log.Printf("windows: stopped; cleanup complete")
	return stopErr
}

func (k *windowsKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.simulateActivity = simulate

	if !k.isRunning {
		return
	}

	if simulate {
		// Start chat app ticker if not already running
		if k.chatAppTick == nil {
			k.startChatAppTickerLocked(k.ctx)
		}
	} else {
		// Stop chat app ticker
		if k.chatAppTick != nil {
			k.chatAppTick.Stop()
			k.chatAppTick = nil
		}
	}
}

// GetDependencyMessage returns empty string on Windows (no external dependencies needed)
func GetDependencyMessage() string {
	return ""
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &windowsKeepAlive{}, nil
}

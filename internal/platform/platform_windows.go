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

	// last time we executed activity jitter (unix nanos)
	lastJitterNS int64

	// last time user activity was observed (unix nanos)
	lastUserActiveNS int64

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
	ticker := time.NewTicker(ActivityInterval)
	k.activityTick = ticker
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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

	ticker := time.NewTicker(ChatAppCheckInterval)
	k.chatAppTick = ticker
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				k.simulateChatAppActivity()
			}
		}
	}()
}

func (k *windowsKeepAlive) simulateChatAppActivity() {
	k.mu.Lock()
	simulate := k.simulateActivity
	k.mu.Unlock()
	if !simulate {
		return
	}

	idle, err := getIdleTime()
	if err != nil {
		log.Printf("windows: idle detection failed: %v", err)
		return
	}

	nowNS := time.Now().UnixNano()
	lastActiveLog := atomic.LoadInt64(&k.lastActiveLogNS)
	lastJitterNS := atomic.LoadInt64(&k.lastJitterNS)
	lastUserActiveNS := atomic.LoadInt64(&k.lastUserActiveNS)

	if lastJitterNS != 0 {
		expectedIdle := time.Duration(nowNS - lastJitterNS)
		if expectedIdle > 0 && idle+SyntheticIdleResetTolerance < expectedIdle {
			atomic.StoreInt64(&k.lastJitterNS, 0)
			atomic.StoreInt64(&k.lastUserActiveNS, observedActiveTimestamp(nowNS, idle))
			if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > 2*time.Minute {
				atomic.StoreInt64(&k.lastActiveLogNS, nowNS)
				log.Printf("windows: user activity detected (idle: %v); pausing activity simulation", idle)
			}
			return
		}
	}

	idleQualified := idle >= IdleThreshold || lastJitterNS != 0
	if !idleQualified {
		atomic.StoreInt64(&k.lastJitterNS, 0)
		atomic.StoreInt64(&k.lastUserActiveNS, observedActiveTimestamp(nowNS, idle))
		// Log occasionally (every 2 minutes) that we're skipping due to active use
		if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > 2*time.Minute {
			atomic.StoreInt64(&k.lastActiveLogNS, nowNS)
			log.Printf("windows: user is active (idle: %v); skipping simulation to avoid interference", idle)
		}
		return
	}

	if lastUserActiveNS != 0 && time.Duration(nowNS-lastUserActiveNS) < IdleThreshold {
		return
	}

	// User became idle - log if we were previously active
	if lastActiveLog != 0 {
		atomic.StoreInt64(&k.lastActiveLogNS, 0)
		log.Printf("windows: user became idle (%v); resuming activity simulation", idle)
	}

	if lastJitterNS != 0 && time.Duration(nowNS-lastJitterNS) < ChatAppActivityInterval {
		return
	}

	points := k.patternGen.GenerateRoundJitterPoints()
	sessionDuration := k.patternGen.JitterSessionDuration()
	k.executeMousePattern(points, sessionDuration)
	atomic.StoreInt64(&k.lastJitterNS, nowNS)
	log.Printf("windows: idle detected (%v); jittered round mouse pattern (%v)", idle, sessionDuration)
}

func (k *windowsKeepAlive) executeMousePattern(points []MousePoint, sessionDuration time.Duration) {
	if len(points) == 0 {
		return
	}

	stepDelay := jitterStepDelay(sessionDuration, len(points))

	currentX := 0
	currentY := 0

	for _, pt := range points {
		dx, dy, targetX, targetY := relativeStepToPoint(currentX, currentY, pt)

		if dx != 0 || dy != 0 {
			k.sendMouseMove(int32(dx), int32(dy))
			currentX = targetX
			currentY = targetY
		}

		time.Sleep(stepDelay)
	}

	// Return to origin
	if currentX != 0 || currentY != 0 {
		k.sendMouseMove(int32(-currentX), int32(-currentY))
	}
	time.Sleep(stepDelay)
}

func (k *windowsKeepAlive) sendMouseMove(dx, dy int32) {
	var inputEv input
	inputEv.inputType = inputMouse
	inputEv.mi = mouseInput{dx: dx, dy: dy, dwFlags: mouseEventMove}

	r1, _, err := procSendInput.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&inputEv)),
		uintptr(unsafe.Sizeof(inputEv)),
	)
	if r1 == 0 {
		log.Printf("windows: SendInput move failed dx=%d dy=%d: %v", dx, dy, err)
	}
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
	atomic.StoreInt64(&k.lastActiveLogNS, 0)
	atomic.StoreInt64(&k.lastJitterNS, 0)
	atomic.StoreInt64(&k.lastUserActiveNS, time.Now().UnixNano())

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
	atomic.StoreInt64(&k.lastActiveLogNS, 0)
	atomic.StoreInt64(&k.lastJitterNS, 0)
	atomic.StoreInt64(&k.lastUserActiveNS, 0)
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
		// Keep ticker alive and gate behavior via simulateActivity flag.
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

//go:build darwin

package platform

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	jitterWarnEvery = 60 * time.Second

	// scriptExecutionTimeout limits how long we wait for osascript to complete.
	// This protects against hangs if Accessibility is misconfigured or the
	// scripting environment is not responding.
	scriptExecutionTimeout = 3 * time.Second
)

type darwinCapabilities struct {
	caffeinateAvailable bool
	pmsetAvailable      bool
	osascriptAvailable  bool
}

// getIdleTime returns the system idle time on macOS
func getIdleTime() (time.Duration, error) {
	idle, err := getIdleTimeIOReg()
	if err == nil {
		return idle, nil
	}

	return getIdleTimeCoreGraphics()
}

func getIdleTimeCoreGraphics() (time.Duration, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return 0, err
	}

	script := `
ObjC.import('CoreGraphics');
var seconds = $.CGEventSourceSecondsSinceLastEventType($.kCGEventSourceStateHIDSystemState, $.kCGAnyInputEventType);
console.log(seconds);
`

	out, err := runJXAScript(script)
	if err != nil {
		return 0, err
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, fmt.Errorf("empty idle-time output")
	}

	seconds, parseErr := strconv.ParseFloat(trimmed, 64)
	if parseErr != nil {
		re := regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)`)
		match := re.FindStringSubmatch(trimmed)
		if len(match) < 2 {
			return 0, fmt.Errorf("failed to parse coregraphics idle output %q: %v", trimmed, parseErr)
		}
		seconds, parseErr = strconv.ParseFloat(match[1], 64)
		if parseErr != nil {
			return 0, fmt.Errorf("failed to parse coregraphics idle output %q: %v", trimmed, parseErr)
		}
	}

	if seconds < 0 {
		seconds = 0
	}

	return time.Duration(seconds * float64(time.Second)), nil
}

func getIdleTimeIOReg() (time.Duration, error) {
	out, err := exec.Command("ioreg", "-c", "IOHIDSystem").Output()
	if err != nil {
		return 0, err
	}

	// Extract HIDIdleTime value (in nanoseconds, decimal or hex)
	re := regexp.MustCompile(`"HIDIdleTime"\s*=\s*(0x[0-9a-fA-F]+|\d+)`)
	matches := re.FindSubmatch(out)
	if len(matches) < 2 {
		return 0, fmt.Errorf("HIDIdleTime not found in ioreg output")
	}

	idleValue := string(matches[1])
	nanos, err := strconv.ParseUint(idleValue, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse HIDIdleTime %q: %v", idleValue, err)
	}
	if nanos > math.MaxInt64 {
		nanos = math.MaxInt64
	}

	return time.Duration(int64(nanos)), nil
}

// darwinKeepAlive implements the KeepAlive interface for macOS
type darwinKeepAlive struct {
	mu                  sync.Mutex
	cmd                 *exec.Cmd
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	isRunning           bool
	activityTick        *time.Ticker
	chatAppActivityTick *time.Ticker
	activeMethod        string

	// 0 or 1
	simulateActivity atomic.Bool

	// closed when cmd.Wait returns
	waitDone chan struct{}

	// last time we warned about jitter failure, unix nanos
	lastJitterWarnNS int64

	// random source for jitter
	rnd *rand.Rand

	// mouse pattern generator for natural movement patterns
	patternGen *MousePatternGenerator

	// shared activity controller for idle-gated jitter
	activityCtrl *ActivityController
}

// Start initiates the keep-alive functionality.
func (k *darwinKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	k.ctx, k.cancel = context.WithCancel(ctx)
	k.rnd = newCryptoSeededRand()
	k.patternGen = NewMousePatternGenerator(k.rnd)
	k.activityCtrl = NewActivityController("darwin", k.patternGen)
	atomic.StoreInt64(&k.lastJitterWarnNS, 0)

	caps, err := detectDarwinCapabilities()
	if err != nil {
		k.cancel()
		return err
	}

	if err := k.startCaffeinateLocked(); err != nil {
		k.cancel()
		return err
	}

	k.maybeStartChatAppTickerLocked()
	k.logPmsetAssertions(caps)
	k.setActiveMethod(caps)

	k.isRunning = true
	return nil
}

func detectDarwinCapabilities() (darwinCapabilities, error) {
	var caps darwinCapabilities

	if _, err := exec.LookPath("caffeinate"); err != nil {
		return caps, err
	}
	caps.caffeinateAvailable = true

	if _, err := exec.LookPath("pmset"); err != nil {
		log.Printf("darwin: pmset not available; proceeding without pmset touch assertion")
	} else {
		caps.pmsetAvailable = true
	}

	if _, err := exec.LookPath("osascript"); err != nil {
		log.Printf("darwin: osascript not available; mouse jitter will not work: %v", err)
	} else {
		caps.osascriptAvailable = true
	}

	return caps, nil
}

func (k *darwinKeepAlive) startCaffeinateLocked() error {
	k.cmd = exec.CommandContext(k.ctx, "caffeinate", "-s", "-d", "-m", "-i")
	k.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := k.cmd.Start(); err != nil {
		return err
	}

	k.waitDone = make(chan struct{})

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		_ = k.cmd.Wait()
		close(k.waitDone)
	}()

	return nil
}

func (k *darwinKeepAlive) maybeStartChatAppTickerLocked() {
	if !k.simulateActivity.Load() || k.ctx == nil {
		return
	}

	if k.chatAppActivityTick != nil {
		return
	}

	ticker := time.NewTicker(ChatAppCheckInterval)
	k.chatAppActivityTick = ticker

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer ticker.Stop()

		for {
			select {
			case <-k.ctx.Done():
				return
			case <-ticker.C:
				k.simulateChatAppActivity()
			}
		}
	}()
}

func (k *darwinKeepAlive) logPmsetAssertions(caps darwinCapabilities) {
	if !caps.pmsetAvailable {
		return
	}

	out, err := exec.Command("pmset", "-g", "assertions").CombinedOutput()
	if err != nil {
		log.Printf("darwin: pmset assertions check failed: %v", err)
		return
	}

	log.Printf("darwin: started keep alive; pmset assertions bytes=%d", len(out))
}

func (k *darwinKeepAlive) setActiveMethod(caps darwinCapabilities) {
	_ = caps
	k.activeMethod = "caffeinate"
	log.Printf("darwin: active method: %s", k.activeMethod)
}

// simulateChatAppActivity simulates natural user activity to keep Teams/Slack active.
// Only triggers when the user is idle to avoid interfering with actual computer use.
func (k *darwinKeepAlive) simulateChatAppActivity() {
	if !k.simulateActivity.Load() {
		return
	}

	k.activityCtrl.MaybeJitter(
		getIdleTime,
		func(points []MousePoint, sessionDuration time.Duration) {
			if err := k.jitterMouseRoundPattern(sessionDuration); err != nil {
				k.warnJitterFailureOnce(err)
			}
		},
	)
}

func runJXAScript(script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), scriptExecutionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", script)
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("osascript timed out after %s", scriptExecutionTimeout)
	}

	return out, err
}

func (k *darwinKeepAlive) warnJitterFailureOnce(err error) {
	nowNS := time.Now().UnixNano()
	last := atomic.LoadInt64(&k.lastJitterWarnNS)
	if last != 0 && time.Duration(nowNS-last) < jitterWarnEvery {
		return
	}
	atomic.StoreInt64(&k.lastJitterWarnNS, nowNS)

	log.Printf("darwin: mouse jitter failed (%v). This can happen in headless/remote sessions where cursor warping is unavailable.", err)
}

// jitterMouseRoundPattern applies a small random round pattern and returns to origin.
func (k *darwinKeepAlive) jitterMouseRoundPattern(sessionDuration time.Duration) error {
	points := k.patternGen.GenerateRoundJitterPoints()
	script := k.buildMouseMovementScript(points, sessionDuration)

	out, err := runJXAScript(script)
	if err != nil {
		return fmt.Errorf("osascript failed: %v (output: %q)", err, string(out))
	}
	return nil
}

func (k *darwinKeepAlive) buildMouseMovementScript(points []MousePoint, sessionDuration time.Duration) string {
	stepDelay := jitterStepDelay(sessionDuration, len(points))

	// Use CGEventCreateMouseEvent + CGEventPost to generate real HID mouse-move
	// events. These are recognized by applications (Slack, Teams) as genuine user
	// input, unlike CGWarpMouseCursorPosition which only repositions the cursor.
	script := `
ObjC.import('CoreGraphics');

function loc() {
	var ev = $.CGEventCreate(null);
	var p = $.CGEventGetLocation(ev);
	return {x: p.x, y: p.y};
}

function moveTo(x, y) {
	var pt = $.CGPointMake(x, y);
	var ev = $.CGEventCreateMouseEvent(null, $.kCGEventMouseMoved, pt, $.kCGMouseButtonLeft);
	if (ev != null) {
		$.CGEventPost($.kCGHIDEventTap, ev);
	} else {
		// Fallback: warp cursor directly if event creation fails
		$.CGWarpMouseCursorPosition(pt);
	}
}

var origin = loc();
var x0 = origin.x;
var y0 = origin.y;
`

	for _, pt := range points {
		d := k.patternGen.JitterStepDelayWithVariance(stepDelay)
		script += fmt.Sprintf("moveTo(x0 + %f, y0 + %f);\ndelay(%f);\n", pt.X, pt.Y, d.Seconds())
	}

	returnD := k.patternGen.JitterStepDelayWithVariance(stepDelay)
	script += fmt.Sprintf("\n// Return to origin\nmoveTo(x0, y0);\ndelay(%f);\n", returnD.Seconds())
	script += `console.log("ok");
`
	return script
}

func (k *darwinKeepAlive) killProcessLocked() {
	if k.cmd == nil || k.cmd.Process == nil {
		return
	}

	pid := k.cmd.Process.Pid

	// Try SIGTERM first
	if err := k.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		log.Printf("darwin: failed to send SIGTERM to caffeinate (pid %d): %v", pid, err)
	}

	// Wait briefly for clean shutdown
	if k.waitDone != nil {
		timeouts := []time.Duration{150 * time.Millisecond, 250 * time.Millisecond, 400 * time.Millisecond}
		for _, to := range timeouts {
			select {
			case <-k.waitDone:
				log.Printf("darwin: caffeinate process (pid %d) terminated cleanly", pid)
				return
			case <-time.After(to):
			}
		}
	}

	// Escalate to SIGKILL
	log.Printf("darwin: caffeinate process (pid %d) did not terminate, sending SIGKILL", pid)
	if err := k.cmd.Process.Kill(); err != nil {
		log.Printf("darwin: failed to kill caffeinate process (pid %d): %v", pid, err)
	}

	// Also try killing the process group
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		log.Printf("darwin: failed to kill caffeinate process group (pgid %d): %v", pid, err)
	}

	if k.waitDone != nil {
		select {
		case <-k.waitDone:
			log.Printf("darwin: caffeinate process (pid %d) terminated after SIGKILL", pid)
		case <-time.After(500 * time.Millisecond):
			log.Printf("darwin: warning: caffeinate process (pid %d) may still be running", pid)
		}
	}
}

// verifyProcessTerminated checks if the caffeinate process has actually terminated
func (k *darwinKeepAlive) verifyProcessTerminated() bool {
	if k.cmd == nil || k.cmd.Process == nil {
		return true
	}

	pid := k.cmd.Process.Pid

	// Check if process still exists by sending signal 0 (doesn't kill, just checks)
	err := syscall.Kill(pid, 0)
	if err == nil {
		log.Printf("darwin: warning: caffeinate process (pid %d) still exists", pid)
		return false
	}

	if err == syscall.ESRCH {
		log.Printf("darwin: verified caffeinate process (pid %d) has terminated", pid)
		return true
	}

	log.Printf("darwin: could not verify caffeinate process (pid %d) status: %v", pid, err)
	return false
}

// Stop terminates the keep alive functionality
func (k *darwinKeepAlive) Stop() error {
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
	if k.chatAppActivityTick != nil {
		k.chatAppActivityTick.Stop()
	}

	k.killProcessLocked()
	k.mu.Unlock()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		k.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("darwin: all goroutines completed")
	case <-time.After(2 * time.Second):
		log.Printf("darwin: warning: some goroutines did not complete within timeout")
	}

	k.mu.Lock()

	// Verify process termination
	if !k.verifyProcessTerminated() {
		log.Printf("darwin: warning: caffeinate process may still be running")
	}

	k.isRunning = false
	k.cmd = nil
	k.ctx = nil
	k.cancel = nil
	k.activityTick = nil
	k.chatAppActivityTick = nil
	k.waitDone = nil
	if k.activityCtrl != nil {
		k.activityCtrl.Reset()
	}
	atomic.StoreInt64(&k.lastJitterWarnNS, 0)
	k.mu.Unlock()

	log.Printf("darwin: stopped; cleanup complete")
	return nil
}

func (k *darwinKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if simulate {
		k.simulateActivity.Store(true)
		// Start chat app activity ticker if not already running and we have a context.
		// When simulate is toggled off, the goroutine stays alive but is gated by the
		// atomic flag, so chatAppActivityTick remains non-nil. This intentionally
		// prevents spawning duplicate goroutines on repeated on/off toggles.
		if k.chatAppActivityTick == nil && k.isRunning && k.ctx != nil {
			ticker := time.NewTicker(ChatAppCheckInterval)
			k.chatAppActivityTick = ticker
			k.wg.Add(1)
			go func() {
				defer k.wg.Done()
				defer ticker.Stop()

				for {
					select {
					case <-k.ctx.Done():
						return
					case <-ticker.C:
						k.simulateChatAppActivity()
					}
				}
			}()
		}
	} else {
		k.simulateActivity.Store(false)
		// The ticker goroutine remains alive but no-ops via the atomic flag check.
		// It will be cleaned up when the context is cancelled (on Stop).
	}
}

// GetDependencyMessage returns empty string on macOS (no external dependencies needed)
func GetDependencyMessage() string {
	return ""
}

// NewKeepAlive creates a new platform specific keep alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &darwinKeepAlive{}, nil
}

//go:build darwin

package platform

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	permissionWarnEvery = 60 * time.Second

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

// runBestEffort executes a command ignoring any errors (best effort)
func runBestEffort(name string, args ...string) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		log.Printf("darwin: best effort command %s failed: %v (output: %q)", name, err, string(out))
	}
}

// run executes a command and returns any error
func run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// getIdleTime returns the system idle time on macOS
func getIdleTime() (time.Duration, error) {
	out, err := exec.Command("ioreg", "-c", "IOHIDSystem").Output()
	if err != nil {
		return 0, err
	}

	// Extract HIDIdleTime value (in nanoseconds)
	re := regexp.MustCompile(`"HIDIdleTime"\s*=\s*(\d+)`)
	matches := re.FindSubmatch(out)
	if len(matches) < 2 {
		return 0, fmt.Errorf("HIDIdleTime not found in ioreg output")
	}

	nanos, err := strconv.ParseInt(string(matches[1]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse HIDIdleTime: %v", err)
	}

	return time.Duration(nanos), nil
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
	simulateActivity uint32

	// closed when cmd.Wait returns
	waitDone chan struct{}

	// last time we warned about Accessibility, unix nanos
	lastPermWarnNS int64

	// random source for jitter
	rnd *rand.Rand

	// mouse pattern generator for natural movement patterns
	patternGen *MousePatternGenerator
}

// Start initiates the keep-alive functionality.
func (k *darwinKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	k.ctx, k.cancel = context.WithCancel(ctx)
	k.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	k.patternGen = NewMousePatternGenerator(k.rnd)

	caps, err := detectDarwinCapabilities()
	if err != nil {
		k.cancel()
		return err
	}

	if err := k.startCaffeinateLocked(); err != nil {
		k.cancel()
		return err
	}

	k.startActivityTickerLocked(caps)
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
	k.cmd = exec.CommandContext(k.ctx, "caffeinate", "-s", "-d", "-m", "-i", "-u")
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

func (k *darwinKeepAlive) startActivityTickerLocked(caps darwinCapabilities) {
	k.activityTick = time.NewTicker(ActivityInterval)

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.activityTick.Stop()

		for {
			select {
			case <-k.ctx.Done():
				return
			case <-k.activityTick.C:
				if caps.pmsetAvailable {
					runBestEffort("pmset", "touch")
				}
				runBestEffort("caffeinate", "-u", "-t", "1")
			}
		}
	}()
}

func (k *darwinKeepAlive) maybeStartChatAppTickerLocked() {
	if atomic.LoadUint32(&k.simulateActivity) != 1 || k.ctx == nil {
		return
	}

	if k.chatAppActivityTick != nil {
		return
	}

	k.chatAppActivityTick = time.NewTicker(ChatAppActivityInterval)

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.chatAppActivityTick.Stop()

		for {
			select {
			case <-k.ctx.Done():
				return
			case <-k.chatAppActivityTick.C:
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
	method := "caffeinate"
	if caps.pmsetAvailable {
		method = "caffeinate+pmset"
	}

	k.activeMethod = method
	log.Printf("darwin: active method: %s", k.activeMethod)
}

// simulateChatAppActivity simulates natural user activity to keep Teams/Slack active
// Only triggers when the user is idle to avoid interfering with actual computer use
func (k *darwinKeepAlive) simulateChatAppActivity() {
	// Check if user is idle - only simulate when idle
	idle, err := getIdleTime()
	if err != nil {
		log.Printf("darwin: idle detection failed: %v", err)
		// If we can't detect idle, don't simulate to avoid interfering
		return
	}

	// Only simulate activity if user has been idle for more than threshold
	if idle <= IdleThreshold {
		return
	}

	// User is idle - simulate natural, human-like activity
	// First, try a simple keyboard event (Shift key press/release)
	// This is often more effective than mouse movement alone
	if err := k.simulateKeyboardActivity(); err != nil {
		// Only log if it consistently fails (warnAccessibilityOnce handles rate limiting)
		log.Printf("darwin: keyboard simulation failed: %v", err)
	}

	// Use the natural, user-like mouse movement patterns
	// This creates realistic movement patterns (circles, squares, zigzags, random walks)
	// with variable speeds, pauses, and acceleration/deceleration
	if err := k.jitterMouseRandomShape(); err != nil {
		k.warnAccessibilityOnce(err)
		return
	}

	log.Printf("darwin: idle detected (%v); simulated natural user activity", idle)
}

// simulateKeyboardActivity simulates a keyboard event (Shift key press/release)
// This generates system-level keyboard activity that Teams/Slack can detect
func (k *darwinKeepAlive) simulateKeyboardActivity() error {
	script := `
ObjC.import('CoreGraphics');

var shiftKeyDown = $.CGEventCreateKeyboardEvent(null, 0x38, true);  // Shift key down
var shiftKeyUp = $.CGEventCreateKeyboardEvent(null, 0x38, false);  // Shift key up

$.CGEventPost($.kCGHIDEventTap, shiftKeyDown);
delay(0.01);
$.CGEventPost($.kCGHIDEventTap, shiftKeyUp);

console.log("ok");
`
	out, err := runJXAScript(script)
	if err != nil {
		return fmt.Errorf("keyboard simulation failed: %v (output: %q)", err, string(out))
	}
	return nil
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

func (k *darwinKeepAlive) warnAccessibilityOnce(err error) {
	nowNS := time.Now().UnixNano()
	last := atomic.LoadInt64(&k.lastPermWarnNS)
	if last != 0 && time.Duration(nowNS-last) < permissionWarnEvery {
		return
	}
	atomic.StoreInt64(&k.lastPermWarnNS, nowNS)

	log.Printf(
		"darwin: mouse jitter blocked or failed (%v). On macOS you must enable Accessibility for the process doing the warp. If you run from Terminal, enable Terminal. If this is a packaged app, enable the app in System Settings, Privacy and Security, Accessibility.",
		err,
	)
}

// jitterMouseRandomShape moves the mouse in a random pattern and returns it to origin
func (k *darwinKeepAlive) jitterMouseRandomShape() error {
	points := k.patternGen.GenerateShapePoints()
	script := k.buildMouseMovementScript(points)

	out, err := runJXAScript(script)
	if err != nil {
		return fmt.Errorf("osascript failed: %v (output: %q)", err, string(out))
	}
	return nil
}

func (k *darwinKeepAlive) buildMouseMovementScript(points []MousePoint) string {
	script := `
ObjC.import('CoreGraphics');

function loc() {
	var ev = $.CGEventCreate(null);
	var p = $.CGEventGetLocation(ev);
	return {x: p.x, y: p.y};
}

function moveMouse(x, y) {
	// Use CGEventPost to create actual mouse move events that applications can detect
	var moveEvent = $.CGEventCreateMouseEvent(null, $.kCGEventMouseMoved, {x: x, y: y}, $.kCGMouseButtonLeft);
	$.CGEventPost($.kCGHIDEventTap, moveEvent);
}

var origin = loc();
var x0 = origin.x;
var y0 = origin.y;

// Test if mouse movement works (small test movement)
moveMouse(x0 + 1, y0 + 1);
delay(0.02);
var test = loc();
if (Math.abs(test.x - (x0 + 1)) > 0.5 || Math.abs(test.y - (y0 + 1)) > 0.5) {
	moveMouse(x0, y0);
	throw new Error("mouse movement appears blocked (Accessibility not granted)");
}

// Move through points with variable speed (natural user-like movement)
`

	for i, pt := range points {
		distance := SegmentDistance(points, i)
		delay := k.patternGen.MovementDelay(distance)

		script += fmt.Sprintf("moveMouse(x0 + %f, y0 + %f);\ndelay(%f);\n", pt.X, pt.Y, delay.Seconds())

		if k.patternGen.ShouldPause() {
			pauseDelay := k.patternGen.PauseDelay()
			script += fmt.Sprintf("// Pause\ndelay(%f);\n", pauseDelay.Seconds())
		}

		if k.patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := k.patternGen.IntermediatePoint(points, i, delay)
			script += fmt.Sprintf("moveMouse(x0 + %f, y0 + %f);\ndelay(%f);\n", midPt.X, midPt.Y, midDelay.Seconds())
		}
	}

	script += `
// Return to origin with variable speed (natural return movement)
`

	returnDelay := k.patternGen.ReturnDelay()
	script += fmt.Sprintf("moveMouse(x0, y0);\ndelay(%f);\n", returnDelay.Seconds())
	script += `
console.log("ok");
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
	k.mu.Unlock()

	log.Printf("darwin: stopped; cleanup complete")
	return nil
}

func (k *darwinKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if simulate {
		atomic.StoreUint32(&k.simulateActivity, 1)
		// Start chat app activity ticker if not already running and we have a context
		if k.chatAppActivityTick == nil && k.isRunning && k.ctx != nil {
			k.chatAppActivityTick = time.NewTicker(ChatAppActivityInterval)
			k.wg.Add(1)
			go func() {
				defer k.wg.Done()
				defer k.chatAppActivityTick.Stop()

				for {
					select {
					case <-k.ctx.Done():
						return
					case <-k.chatAppActivityTick.C:
						k.simulateChatAppActivity()
					}
				}
			}()
		}
	} else {
		atomic.StoreUint32(&k.simulateActivity, 0)
		// Stop chat app activity ticker
		if k.chatAppActivityTick != nil {
			k.chatAppActivityTick.Stop()
			k.chatAppActivityTick = nil
		}
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

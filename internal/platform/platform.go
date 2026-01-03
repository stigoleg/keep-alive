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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/stigoleg/keep-alive/internal/platform/patterns"
)

const (
	permissionWarnEvery = 60 * time.Second

	// scriptExecutionTimeout limits how long we wait for osascript to complete.
	// This protects against hangs if Accessibility is misconfigured or the
	// scripting environment is not responding.
	scriptExecutionTimeout = 3 * time.Second
)

// checkAccessibilityPermission checks if Accessibility is enabled using AXIsProcessTrusted
func checkAccessibilityPermission() bool {
	script := `ObjC.import('ApplicationServices'); $.AXIsProcessTrusted()`
	out, err := runJXAScript(script)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// CheckActivitySimulationCapability checks if the platform can simulate user activity.
// On macOS, this checks if Accessibility permissions are granted.
func CheckActivitySimulationCapability() SimulationCapability {
	if checkAccessibilityPermission() {
		return SimulationCapability{CanSimulate: true}
	}

	return SimulationCapability{
		CanSimulate:  false,
		ErrorMessage: "Accessibility permission required for activity simulation",
		Instructions: `To enable activity simulation on macOS:

1. Open System Settings > Privacy & Security > Accessibility
2. Click the "+" button to add an application
3. Add your terminal app (Terminal, iTerm2, etc.) or the keepalive app
4. Ensure the checkbox is enabled
5. Restart keepalive

The system will prompt you to grant permission.`,
		CanPrompt: true,
	}
}

// PromptActivitySimulationPermission triggers the system Accessibility permission dialog.
// On macOS, this uses AXIsProcessTrustedWithOptions with the prompt option.
func PromptActivitySimulationPermission() {
	// AXIsProcessTrustedWithOptions with kAXTrustedCheckOptionPrompt triggers the dialog
	script := `
ObjC.import('CoreFoundation');
ObjC.import('ApplicationServices');

// Create the options dictionary with kAXTrustedCheckOptionPrompt = true
var key = $.kAXTrustedCheckOptionPrompt;
var value = $.kCFBooleanTrue;
var options = $.CFDictionaryCreate(
    null,
    Ref([key]),
    Ref([value]),
    1,
    $.kCFTypeDictionaryKeyCallBacks,
    $.kCFTypeDictionaryValueCallBacks
);

// This will trigger the system permission dialog
$.AXIsProcessTrustedWithOptions(options);
`
	// Run in background, don't wait for result
	_, _ = runJXAScript(script)
}

type darwinCapabilities struct {
	caffeinateAvailable bool
	pmsetAvailable      bool
	osascriptAvailable  bool
}

// runBestEffort executes a command ignoring any errors (best effort).
func runBestEffort(name string, args ...string) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		log.Printf("darwin: best-effort command %s failed: %v (output: %q)", name, err, string(out))
	}
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
		return 0, fmt.Errorf("failed to parse HIDIdleTime: %w", err)
	}

	return time.Duration(nanos), nil
}

// darwinKeepAlive implements the KeepAlive interface for macOS
type darwinKeepAlive struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	chatAppTick  *time.Ticker
	activeMethod string

	simulateActivity bool

	// closed when cmd.Wait returns
	waitDone chan struct{}

	// last time we warned about Accessibility, unix nanos
	lastPermWarnNS int64

	// idle tracker for rate-limited logging
	idleTracker *patterns.IdleTracker

	// random source for jitter
	rnd *rand.Rand

	// mouse pattern generator for natural movement patterns
	patternGen *patterns.Generator
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
	k.patternGen = patterns.NewGenerator(k.rnd)
	k.idleTracker = patterns.NewIdleTracker()

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
	k.activityTick = time.NewTicker(patterns.ActivityInterval)

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
	if !k.simulateActivity || k.ctx == nil {
		return
	}

	if k.chatAppTick != nil {
		return
	}

	k.chatAppTick = time.NewTicker(patterns.ChatAppActivityInterval)

	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.chatAppTick.Stop()

		for {
			select {
			case <-k.ctx.Done():
				return
			case <-k.chatAppTick.C:
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

	// Use idle tracker for rate-limited logging
	result := k.idleTracker.CheckIdle(idle, nil, "darwin")
	if result.LogMessage != "" {
		log.Print(result.LogMessage)
	}
	if !result.ShouldSimulate {
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
		return fmt.Errorf("keyboard simulation failed (output: %q): %w", string(out), err)
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
		return fmt.Errorf("osascript failed (output: %q): %w", string(out), err)
	}
	return nil
}

func (k *darwinKeepAlive) buildMouseMovementScript(points []patterns.Point) string {
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
		distance := patterns.SegmentDistance(points, i)
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
	if k.chatAppTick != nil {
		k.chatAppTick.Stop()
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
	k.chatAppTick = nil
	k.waitDone = nil
	k.mu.Unlock()

	log.Printf("darwin: stopped; cleanup complete")
	return nil
}

func (k *darwinKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if simulate {
		k.simulateActivity = true
		// Start chat app activity ticker if not already running and we have a context
		if k.chatAppTick == nil && k.isRunning && k.ctx != nil {
			k.chatAppTick = time.NewTicker(patterns.ChatAppActivityInterval)
			k.wg.Add(1)
			go func() {
				defer k.wg.Done()
				defer k.chatAppTick.Stop()

				for {
					select {
					case <-k.ctx.Done():
						return
					case <-k.chatAppTick.C:
						k.simulateChatAppActivity()
					}
				}
			}()
		}
	} else {
		k.simulateActivity = false
		// Stop chat app activity ticker
		if k.chatAppTick != nil {
			k.chatAppTick.Stop()
			k.chatAppTick = nil
		}
	}
}

// GetDependencyMessage returns dependency information for macOS.
// On macOS, the main dependency is Accessibility permission for mouse simulation.
func GetDependencyMessage() string {
	cap := CheckActivitySimulationCapability()
	if !cap.CanSimulate {
		return fmt.Sprintf(`
Accessibility Permission Required

%s

%s
`, cap.ErrorMessage, cap.Instructions)
	}
	return ""
}

// NewKeepAlive creates a new platform specific keep alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &darwinKeepAlive{}, nil
}

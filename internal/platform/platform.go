//go:build darwin

package platform

import (
	"context"
	"errors"
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
	jitterWarnEvery = 60 * time.Second
)

type darwinCapabilities struct {
	caffeinateAvailable bool
	pmsetAvailable      bool
}

// getIdleTime returns the system idle time on macOS.
//
// We always go through CoreGraphics because coreGraphicsIdleTime queries both
// the HID and combined-session state counters and returns the larger of the
// two — the combined-session counter is what Chromium/Electron (Teams, Slack)
// use to decide whether the user is away. The old `ioreg` fast path only saw
// HIDIdleTime and would underreport when only the combined counter advanced.
func getIdleTime() (time.Duration, error) {
	return coreGraphicsIdleTime()
}

func parseDarwinBatteryPercentage(output string) (int, error) {
	re := regexp.MustCompile(`(\d+)%`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0, fmt.Errorf("battery percentage not found")
	}

	percentage, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse battery percentage %q: %v", matches[1], err)
	}
	if percentage < 0 || percentage > 100 {
		return 0, fmt.Errorf("battery percentage out of range: %d", percentage)
	}
	return percentage, nil
}

func GetBatteryStatus() (BatteryStatus, error) {
	out, err := exec.Command("pmset", "-g", "batt").CombinedOutput()
	if err != nil {
		return BatteryStatus{}, fmt.Errorf("failed to read battery status: %v", err)
	}

	percentage, err := parseDarwinBatteryPercentage(string(out))
	if err != nil {
		return BatteryStatus{}, err
	}

	return BatteryStatus{Percentage: percentage, Available: true}, nil
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

	// postAccessChecked is set once we've called CGRequestPostEventAccess to
	// trigger the system Accessibility prompt. We only want to prompt once
	// per session — subsequent checks use CGPreflightPostEventAccess.
	postAccessChecked atomic.Bool

	// postAccessGranted is the cached result of the latest Accessibility
	// check. False means CGEventPost will silently no-op and we should fall
	// back to caffeinate -u for chat-app activity.
	postAccessGranted atomic.Bool

	// lastPostAccessRecheckNS is the last time we re-queried Accessibility
	// when the cached state was "denied", so users can grant permission while
	// the CLI is running without restarting it.
	lastPostAccessRecheckNS int64

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

	k.refreshPostEventAccess()

	k.activityCtrl.MaybeJitter(
		getIdleTime,
		func(points []MousePoint, sessionDuration time.Duration) {
			if err := k.jitterMouseRoundPattern(sessionDuration); err != nil {
				k.warnJitterFailureOnce(err)
				if errors.Is(err, errCoreGraphicsPostFailed) {
					k.runCaffeinateUserActive()
				}
			}
		},
	)
}

// refreshPostEventAccess checks (and on first call, requests) Accessibility
// permission. CGEventPost silently no-ops without it, so without this gate
// we'd jitter blindly and Teams/Slack would still go away.
func (k *darwinKeepAlive) refreshPostEventAccess() {
	if !k.postAccessChecked.Load() {
		granted := coreGraphicsRequestPostAccess()
		k.postAccessGranted.Store(granted)
		k.postAccessChecked.Store(true)
		if !granted {
			log.Printf("darwin: Accessibility not granted; CGEventPost will no-op. " +
				"Grant the Keep-Alive app (and the bundled keepalive binary) under " +
				"System Settings → Privacy & Security → Accessibility. " +
				"Falling back to `caffeinate -u` for chat-app activity.")
		}
		return
	}

	if k.postAccessGranted.Load() {
		return
	}

	nowNS := time.Now().UnixNano()
	last := atomic.LoadInt64(&k.lastPostAccessRecheckNS)
	if last != 0 && time.Duration(nowNS-last) < time.Minute {
		return
	}
	atomic.StoreInt64(&k.lastPostAccessRecheckNS, nowNS)
	if coreGraphicsPreflightPostAccess() {
		k.postAccessGranted.Store(true)
		log.Printf("darwin: Accessibility now granted; resuming CoreGraphics activity simulation")
	}
}

// runCaffeinateUserActive fires a short-lived `caffeinate -u -t 30` so the
// system registers a UserIsActive IOPMAssertion. This is the belt to the
// CGEventPost suspenders — IOPMAssertion does not require Accessibility and
// touches the HID-side IOKit state, which on many setups is enough to keep
// Teams/Slack out of Away even when synthetic mouse events are blocked.
func (k *darwinKeepAlive) runCaffeinateUserActive() {
	if k.ctx == nil {
		return
	}
	cmd := exec.CommandContext(k.ctx, "caffeinate", "-u", "-t", "30")
	if err := cmd.Start(); err != nil {
		log.Printf("darwin: caffeinate -u fallback failed to start: %v", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}

func (k *darwinKeepAlive) warnJitterFailureOnce(err error) {
	nowNS := time.Now().UnixNano()
	last := atomic.LoadInt64(&k.lastJitterWarnNS)
	if last != 0 && time.Duration(nowNS-last) < jitterWarnEvery {
		return
	}
	atomic.StoreInt64(&k.lastJitterWarnNS, nowNS)

	log.Printf("darwin: mouse jitter failed (%v). This can happen when Accessibility permission is missing or in headless/remote sessions.", err)
}

// jitterMouseRoundPattern applies a small random round pattern and returns to origin.
func (k *darwinKeepAlive) jitterMouseRoundPattern(sessionDuration time.Duration) error {
	if k.postAccessChecked.Load() && !k.postAccessGranted.Load() {
		return errCoreGraphicsPostFailed
	}

	points := k.patternGen.GenerateRoundJitterPoints()
	if len(points) == 0 {
		return nil
	}

	originX, originY, err := coreGraphicsMouseLocation()
	if err != nil {
		return err
	}

	stepDelay := jitterStepDelay(sessionDuration, len(points))

	for _, pt := range points {
		select {
		case <-k.ctx.Done():
			_ = coreGraphicsPostMouseMove(originX, originY)
			return nil
		default:
		}

		if err := coreGraphicsPostMouseMove(originX+pt.X, originY+pt.Y); err != nil {
			_ = coreGraphicsPostMouseMove(originX, originY)
			return err
		}
		time.Sleep(k.patternGen.JitterStepDelayWithVariance(stepDelay))
	}

	if err := coreGraphicsPostMouseMove(originX, originY); err != nil {
		return err
	}
	time.Sleep(k.patternGen.JitterStepDelayWithVariance(stepDelay))

	return nil
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

func GetActivitySimulationStatus() ActivitySimulationStatus {
	return ActivitySimulationStatus{
		Available: true,
		Method:    "CoreGraphics mouse events",
		Message:   "Active status simulation uses direct CoreGraphics mouse events. macOS Accessibility permission is required for the app or terminal that starts KeepAlive.",
	}
}

// NewKeepAlive creates a new platform specific keep alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &darwinKeepAlive{}, nil
}

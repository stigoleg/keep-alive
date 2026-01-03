//go:build linux

package linux

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/stigoleg/keep-alive/internal/platform/patterns"
)

// Timing constants.
const (
	healthCheckInterval = 30 * time.Second
	stopTimeout         = 2 * time.Second
)

// KeepAlive implements the KeepAlive interface for Linux systems.
type KeepAlive struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	chatAppTick  *time.Ticker
	inhibitors   []Inhibitor
	uinput       *UinputSimulator

	simulateActivity bool

	// idle tracker for rate-limited logging
	idleTracker *patterns.IdleTracker

	// random source and pattern generator for natural mouse movements
	rnd        *rand.Rand
	patternGen *patterns.Generator
}

// NewKeepAlive creates a new Linux-specific keep-alive instance.
func NewKeepAlive() (*KeepAlive, error) {
	return &KeepAlive{}, nil
}

// Start initiates the keep-alive functionality.
func (k *KeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	k.ctx, k.cancel = context.WithCancel(ctx)
	k.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	k.patternGen = patterns.NewGenerator(k.rnd)
	k.idleTracker = patterns.NewIdleTracker()

	// Detect capabilities and log diagnostics
	caps := DetectCapabilities()
	log.Printf("linux: === Startup Diagnostics ===")
	log.Printf("linux: Desktop Environment: %s", caps.DesktopEnvironment)
	log.Printf("linux: Display Server: %s", caps.DisplayServer)
	log.Printf("linux: Available tools: xdotool=%v, ydotool=%v, wtype=%v, xprintidle=%v",
		caps.XdotoolAvailable, caps.YdotoolAvailable, caps.WtypeAvailable, caps.XprintidleAvailable)

	// Check uinput permissions and log status
	hasUinputAccess, uinputErrMsg := CheckUinputPermissions()
	log.Printf("linux: uinput access: %v", hasUinputAccess)
	if !hasUinputAccess && uinputErrMsg != "" {
		log.Printf("linux: uinput permission issue: %s", uinputErrMsg)
	}

	// Activate inhibitors
	activeCount, err := k.activateInhibitors(k.ctx)
	if err != nil {
		k.cancel()
		enhancedErr := fmt.Errorf("%w\n\nTroubleshooting:\n- Ensure systemd-inhibit is available: which systemd-inhibit\n- Check DBus services: dbus-send --session --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames\n- For Cosmic/GNOME: ensure org.gnome.SessionManager is available", err)
		return enhancedErr
	}

	// Setup uinput if available
	k.setupUinput()

	hasUinput := k.uinput != nil
	if k.uinput != nil {
		log.Printf("linux: uinput mouse simulation: enabled")
	} else {
		log.Printf("linux: uinput mouse simulation: disabled (permissions or unavailable)")
	}

	// Check for missing dependencies and log messages
	missingDeps := CheckMissingDependencies(caps, hasUinput)
	if len(missingDeps) > 0 {
		depMessage := FormatDependencyMessages(missingDeps, caps.DisplayServer, hasUinput)
		log.Printf("linux: missing dependencies detected:\n%s", depMessage)
	}

	// Log mouse simulation capabilities
	mouseMethods := k.getAvailableMouseMethods(caps)
	if len(mouseMethods) == 0 {
		log.Printf("linux: warning: no mouse simulation methods available")
	} else {
		log.Printf("linux: mouse simulation methods: %s", strings.Join(mouseMethods, ", "))
	}

	log.Printf("linux: === End Diagnostics ===")
	log.Printf("linux: started successfully; active inhibitors: %d", activeCount)

	// Start periodic inhibitor health checks
	k.startInhibitorHealthCheck(k.ctx)

	// Start system-level activity ticker to maintain keep-alive
	k.startActivityTicker(k.ctx)

	// Start chat app activity ticker if enabled
	k.startChatAppTicker(k.ctx, caps)

	k.isRunning = true
	return nil
}

func (k *KeepAlive) getAvailableMouseMethods(caps Capabilities) []string {
	var methods []string
	if k.uinput != nil {
		methods = append(methods, "uinput")
	}
	if caps.YdotoolAvailable {
		methods = append(methods, "ydotool")
	}
	if caps.XdotoolAvailable && caps.DisplayServer == DisplayServerX11 {
		methods = append(methods, "xdotool")
	}
	return methods
}

func (k *KeepAlive) activateInhibitors(ctx context.Context) (int, error) {
	allInhibitors := BuildInhibitors()
	activeCount := 0
	var activationErrors []string

	for _, inh := range allInhibitors {
		err := inh.Activate(ctx)
		if err != nil {
			log.Printf("linux: inhibitor %s failed: %v", inh.Name(), err)
			activationErrors = append(activationErrors, fmt.Sprintf("%s: %v", inh.Name(), err))
			continue
		}

		verified := k.verifyInhibitorActivation(inh)
		if !verified {
			log.Printf("linux: warning: inhibitor %s activated but verification failed", inh.Name())
		}

		k.inhibitors = append(k.inhibitors, inh)
		if verified {
			log.Printf("linux: activated and verified inhibitor: %s", inh.Name())
		}
		activeCount++
	}

	if activeCount == 0 {
		errorMsg := "linux: no keep-alive method successfully activated"
		if len(activationErrors) > 0 {
			errorMsg += "\nFailed inhibitors:\n" + strings.Join(activationErrors, "\n")
		}
		return 0, fmt.Errorf("%s", errorMsg)
	}

	log.Printf("linux: successfully activated %d inhibitor(s) out of %d attempted", activeCount, len(allInhibitors))
	return activeCount, nil
}

func (k *KeepAlive) verifyInhibitorActivation(inh Inhibitor) bool {
	switch v := inh.(type) {
	case *SystemdInhibitor:
		if cmd := v.Cmd(); cmd != nil && cmd.Process != nil {
			err := cmd.Process.Signal(syscall.Signal(0))
			if err == nil {
				log.Printf("linux: verified systemd-inhibit process (pid %d) is running", cmd.Process.Pid)
				return true
			}
			log.Printf("linux: warning: systemd-inhibit process verification failed: %v", err)
		}
		return false
	case *DBusInhibitor:
		if v.Cookie() != 0 {
			log.Printf("linux: verified DBus inhibitor %s with cookie %d", v.Name(), v.Cookie())
			return true
		}
		log.Printf("linux: warning: DBus inhibitor %s activated but no cookie received", v.Name())
		return false
	case *GsettingsInhibitor, *XsetInhibitor:
		return true
	default:
		return false
	}
}

func (k *KeepAlive) setupUinput() {
	hasAccess, errMsg := CheckUinputPermissions()
	if !hasAccess {
		log.Printf("linux: uinput not available: %s", errMsg)
		k.uinput = nil
		return
	}

	k.uinput = &UinputSimulator{}
	if err := k.uinput.Setup(); err != nil {
		log.Printf("linux: uinput setup failed: %v", err)
		k.uinput = nil
		return
	}
	log.Printf("linux: native uinput mouse simulation activated")
}

func (k *KeepAlive) startActivityTicker(ctx context.Context) {
	k.activityTick = time.NewTicker(patterns.ActivityInterval)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.activityTick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-k.activityTick.C:
				SimulateSystemActivity()
			}
		}
	}()
}

func (k *KeepAlive) startInhibitorHealthCheck(ctx context.Context) {
	healthCheckTicker := time.NewTicker(healthCheckInterval)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer healthCheckTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-healthCheckTicker.C:
				k.verifyInhibitors()
			}
		}
	}()
}

func (k *KeepAlive) verifyInhibitors() {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isRunning {
		return
	}

	for _, inh := range k.inhibitors {
		switch v := inh.(type) {
		case *SystemdInhibitor:
			if cmd := v.Cmd(); cmd != nil && cmd.Process != nil {
				if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
					log.Printf("linux: warning: systemd-inhibit process (pid %d) is not running: %v", cmd.Process.Pid, err)
					k.reactivateInhibitor(inh)
				}
			} else {
				log.Printf("linux: warning: systemd-inhibit process is nil, attempting to reactivate")
				k.reactivateInhibitor(inh)
			}
		case *DBusInhibitor:
			if v.Cookie() == 0 {
				log.Printf("linux: warning: DBus inhibitor %s has invalid cookie (0), attempting to reactivate", v.Name())
				k.reactivateInhibitor(inh)
			}
		}
	}
}

func (k *KeepAlive) reactivateInhibitor(inh Inhibitor) {
	if k.ctx == nil {
		return
	}

	name := inh.Name()
	log.Printf("linux: attempting to reactivate %s", name)
	if err := inh.Activate(k.ctx); err != nil {
		log.Printf("linux: error: failed to reactivate %s: %v", name, err)
		return
	}
	log.Printf("linux: successfully reactivated %s", name)
}

func (k *KeepAlive) startChatAppTicker(ctx context.Context, caps Capabilities) {
	if !k.simulateActivity {
		return
	}

	k.chatAppTick = time.NewTicker(patterns.ChatAppActivityInterval)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.chatAppTick.Stop()

		if !caps.XprintidleAvailable {
			log.Printf("linux: xprintidle not found; will simulate activity without idle check")
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-k.chatAppTick.C:
				k.simulateChatAppActivity(ctx, caps)
			}
		}
	}()
}

func (k *KeepAlive) simulateChatAppActivity(ctx context.Context, caps Capabilities) {
	var idle time.Duration
	var idleErr error

	if caps.XprintidleAvailable {
		idle, idleErr = GetIdleTime()
		if idleErr != nil {
			log.Printf("linux: idle time check failed: %v (will simulate anyway)", idleErr)
		}
	} else if caps.DisplayServer == DisplayServerWayland {
		// Log only once at startup (handled in startChatAppTicker)
		// No idle detection available, always simulate
		idleErr = fmt.Errorf("xprintidle not available on Wayland")
	}

	result := k.idleTracker.CheckIdle(idle, idleErr, "linux")
	if result.LogMessage != "" {
		log.Print(result.LogMessage)
	}
	if !result.ShouldSimulate {
		return
	}

	points := k.patternGen.GenerateShapePoints()
	k.executeMousePattern(points, caps)
}

func (k *KeepAlive) executeMousePattern(points []patterns.Point, caps Capabilities) {
	// Try uinput first
	if k.uinput != nil {
		mover := &UinputMover{Sim: k.uinput}
		if ExecutePattern(points, mover, k.patternGen) {
			return
		}
	}

	// Try ydotool
	if caps.YdotoolAvailable {
		mover := &CommandMover{Cmd: "ydotool", Args: []string{"mousemove", "--"}}
		if ExecutePattern(points, mover, k.patternGen) {
			return
		}
	}

	// Try xdotool (X11 only)
	if caps.DisplayServer == DisplayServerX11 && caps.XdotoolAvailable {
		mover := &CommandMover{Cmd: "xdotool", Args: []string{"mousemove_relative", "--"}}
		if ExecutePattern(points, mover, k.patternGen) {
			return
		}
	}

	// Fallback to DBus simulation
	SimulateSystemActivity()

	if caps.DisplayServer == DisplayServerWayland {
		log.Printf("linux: warning: no Wayland-compatible mouse simulation method available. Install ydotool: sudo apt install ydotool (or equivalent for your distribution)")
	}
}

// Stop terminates the keep-alive functionality.
func (k *KeepAlive) Stop() error {
	k.mu.Lock()
	if !k.isRunning {
		k.mu.Unlock()
		return nil
	}

	if k.cancel != nil {
		k.cancel()
	}

	if k.activityTick != nil {
		k.activityTick.Stop()
		k.activityTick = nil
	}
	if k.chatAppTick != nil {
		k.chatAppTick.Stop()
		k.chatAppTick = nil
	}

	inhibitors := make([]Inhibitor, len(k.inhibitors))
	copy(inhibitors, k.inhibitors)

	k.mu.Unlock()

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		k.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("linux: all goroutines completed")
	case <-time.After(stopTimeout):
		log.Printf("linux: warning: some goroutines did not complete within timeout")
	}

	// Deactivate inhibitors in reverse order
	var deactivateErrors []error
	for i := len(inhibitors) - 1; i >= 0; i-- {
		inh := inhibitors[i]
		if err := inh.Deactivate(); err != nil {
			log.Printf("linux: error deactivating inhibitor %s: %v", inh.Name(), err)
			deactivateErrors = append(deactivateErrors, err)
		} else {
			log.Printf("linux: deactivated inhibitor %s", inh.Name())
		}
	}

	k.mu.Lock()

	if k.uinput != nil {
		k.uinput.Close()
		k.uinput = nil
		log.Printf("linux: uinput device closed")
	}

	k.inhibitors = nil
	k.isRunning = false
	k.ctx = nil
	k.cancel = nil
	k.mu.Unlock()

	if len(deactivateErrors) > 0 {
		log.Printf("linux: stopped with %d inhibitor deactivation errors", len(deactivateErrors))
		return fmt.Errorf("linux: %d inhibitors failed to deactivate", len(deactivateErrors))
	}

	log.Printf("linux: stopped; cleanup complete")
	return nil
}

// SetSimulateActivity enables or disables activity simulation.
func (k *KeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.simulateActivity = simulate

	if !k.isRunning {
		return
	}

	if simulate {
		if k.chatAppTick == nil {
			caps := DetectCapabilities()
			k.startChatAppTicker(k.ctx, caps)
		}
	} else {
		if k.chatAppTick != nil {
			k.chatAppTick.Stop()
			k.chatAppTick = nil
		}
	}
}

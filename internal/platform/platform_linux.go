//go:build linux

package platform

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// inhibitor defines the common interface for various Linux sleep prevention methods.
type inhibitor interface {
	Name() string
	Activate(ctx context.Context) error
	Deactivate() error
}

// runVerbose executes a command, returns error and combined output (stdout+stderr)
func runVerbose(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// runBestEffort executes a command ignoring any errors (best-effort)
func runBestEffort(name string, args ...string) {
	if out, err := runVerbose(name, args...); err != nil {
		log.Printf("linux: best-effort command %s %s failed: %v (output: %q)", name, strings.Join(args, " "), err, out)
	}
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// --- systemd-inhibit strategy ---

type systemdInhibitor struct {
	cmd *exec.Cmd
}

func (s *systemdInhibitor) Name() string { return "systemd-inhibit" }
func (s *systemdInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("systemd-inhibit") {
		return fmt.Errorf("systemd-inhibit command not found")
	}
	s.cmd = exec.CommandContext(ctx, "systemd-inhibit",
		"--what=idle:sleep:handle-lid-switch",
		"--who=keep-alive",
		"--why=User requested keep-alive",
		"--mode=block",
		"sleep", "infinity")
	return s.cmd.Start()
}
func (s *systemdInhibitor) Deactivate() error {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}

// --- DBus Base strategy ---

type dbusStrategy struct {
	dest   string
	path   string
	iface  string
	method string
	args   []string
	cookie uint32
}

func (d *dbusStrategy) call(method string, args ...string) (string, error) {
	if hasCommand("dbus-send") {
		fullArgs := append([]string{"--print-reply", "--dest=" + d.dest, d.path, d.iface + "." + method}, args...)
		return runVerbose("dbus-send", fullArgs...)
	}
	if hasCommand("gdbus") {
		fullArgs := append([]string{"call", "--session", "--dest", d.dest, "--object-path", d.path, "--method", d.iface + "." + method}, args...)
		return runVerbose("gdbus", fullArgs...)
	}
	return "", fmt.Errorf("no dbus client (dbus-send/gdbus) found")
}

func (d *dbusStrategy) parseCookie(out string) (uint32, error) {
	// Simple parsing for both dbus-send and gdbus output (returns a uint32)
	parts := strings.Fields(out)
	if len(parts) > 0 {
		lastPart := strings.TrimRight(parts[len(parts)-1], ")")
		if val, err := strconv.ParseUint(lastPart, 10, 32); err == nil {
			return uint32(val), nil
		}
	}
	return 0, fmt.Errorf("failed to parse cookie from: %q", out)
}

type dbusInhibitor struct {
	dbusStrategy
	name         string
	unInhibitArg string
}

func (d *dbusInhibitor) Name() string { return d.name }
func (d *dbusInhibitor) Activate(ctx context.Context) error {
	out, err := d.call(d.method, d.args...)
	if err != nil {
		return err
	}
	cookie, err := d.parseCookie(out)
	if err != nil {
		return err
	}
	d.cookie = cookie
	return nil
}

func (d *dbusInhibitor) Deactivate() error {
	if d.cookie == 0 {
		return nil
	}
	_, err := d.call(d.unInhibitArg, "uint32:"+strconv.FormatUint(uint64(d.cookie), 10))
	return err
}

// --- GNOME specific fallback logic ---

type gsettingsInhibitor struct {
	prevSettings map[string]string
}

func (g *gsettingsInhibitor) Name() string { return "gsettings" }
func (g *gsettingsInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("gsettings") {
		return fmt.Errorf("gsettings command not found")
	}
	g.prevSettings = make(map[string]string)
	settings := []struct{ schema, key, value string }{
		{"org.gnome.desktop.session", "idle-delay", "0"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-ac-type", "'nothing'"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-battery-type", "'nothing'"},
	}
	for _, s := range settings {
		if out, err := runVerbose("gsettings", "get", s.schema, s.key); err == nil {
			g.prevSettings[s.schema+" "+s.key] = out
		}
		if out, err := runVerbose("gsettings", "set", s.schema, s.key, s.value); err != nil {
			return fmt.Errorf("gsettings set failed: %v (out: %q)", err, out)
		}
	}
	return nil
}
func (g *gsettingsInhibitor) Deactivate() error {
	for k, v := range g.prevSettings {
		parts := strings.SplitN(k, " ", 2)
		runBestEffort("gsettings", "set", parts[0], parts[1], v)
	}
	return nil
}

// --- X11 strategy ---

type xsetInhibitor struct{}

func (x *xsetInhibitor) Name() string { return "xset" }
func (x *xsetInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("xset") || os.Getenv("DISPLAY") == "" {
		return fmt.Errorf("xset not available or DISPLAY not set")
	}
	runBestEffort("xset", "s", "off")
	runBestEffort("xset", "-dpms")
	return nil
}
func (x *xsetInhibitor) Deactivate() error {
	runBestEffort("xset", "s", "on")
	runBestEffort("xset", "+dpms")
	return nil
}

// getLinuxIdleTime returns the system idle time on Linux using xprintidle (best-effort)
func getLinuxIdleTime() (time.Duration, error) {
	if !hasCommand("xprintidle") {
		return 0, fmt.Errorf("xprintidle not found")
	}
	out, err := runVerbose("xprintidle")
	if err != nil {
		return 0, err
	}
	millis, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse xprintidle output %q: %v", out, err)
	}
	return time.Duration(millis) * time.Millisecond, nil
}

// --- Native uinput Simulator ---

const (
	uinputDevicePath = "/dev/uinput"
	evSyn            = 0x00
	evRel            = 0x02
	relX             = 0x00
	relY             = 0x01
	uiSetEvbit       = 0x40045564 // _IOW('U', 100, int)
	uiSetRelbit      = 0x40045565 // _IOW('U', 101, int)
	uiDevCreate      = 0x5501     // _IO('U', 1)
	uiDevDestroy     = 0x5502     // _IO('U', 2)
)

type uinputUserDev struct {
	name [80]byte
	id   struct {
		bustype uint16
		vendor  uint16
		product uint16
		version uint16
	}
	ffEffectsMax uint32
	absmax       [64]int32
	absmin       [64]int32
	absfuzz      [64]int32
	absflat      [64]int32
}

type inputEvent struct {
	time  syscall.Timeval
	etype uint16
	code  uint16
	value int32
}

type uinputSimulator struct {
	fd uintptr
}

func (u *uinputSimulator) setup() error {
	f, err := os.OpenFile(uinputDevicePath, os.O_WRONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		return err
	}
	u.fd = f.Fd()

	// Enable relative axes
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetEvbit), uintptr(evRel)); errno != 0 {
		f.Close()
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetRelbit), uintptr(relX)); errno != 0 {
		f.Close()
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiSetRelbit), uintptr(relY)); errno != 0 {
		f.Close()
		return errno
	}

	// Create device
	var dev uinputUserDev
	copy(dev.name[:], "keep-alive-mouse")
	dev.id.bustype = 0x03 // BUS_USB
	dev.id.vendor = 0x1234
	dev.id.product = 0x5678

	if _, _, errno := syscall.Syscall(syscall.SYS_WRITE, u.fd, uintptr(unsafe.Pointer(&dev)), unsafe.Sizeof(dev)); errno != 0 {
		f.Close()
		return errno
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiDevCreate), 0); errno != 0 {
		f.Close()
		return errno
	}

	return nil
}

func (u *uinputSimulator) move(dx, dy int32) {
	events := []inputEvent{
		{etype: evRel, code: relX, value: dx},
		{etype: evRel, code: relY, value: dy},
		{etype: evSyn, code: 0, value: 0},
	}
	for _, ev := range events {
		syscall.Write(int(u.fd), (*[unsafe.Sizeof(ev)]byte)(unsafe.Pointer(&ev))[:])
	}
}

func (u *uinputSimulator) close() {
	if u.fd != 0 {
		syscall.Syscall(syscall.SYS_IOCTL, u.fd, uintptr(uiDevDestroy), 0)
		syscall.Close(int(u.fd))
		u.fd = 0
	}
}

// --- Platform Implementation ---

type linuxCapabilities struct {
	xdotoolAvailable    bool
	xprintidleAvailable bool
	uinputAvailable     bool
}

type linuxKeepAlive struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	chatAppTick  *time.Ticker
	inhibitors   []inhibitor
	uinput       *uinputSimulator

	simulateActivity bool

	// random source and pattern generator for natural mouse movements
	rnd        *rand.Rand
	patternGen *MousePatternGenerator
}

func detectLinuxCapabilities() linuxCapabilities {
	return linuxCapabilities{
		xdotoolAvailable:    hasCommand("xdotool"),
		xprintidleAvailable: hasCommand("xprintidle"),
		uinputAvailable:     true, // Will be tested during setup
	}
}

func buildLinuxInhibitors() []inhibitor {
	return []inhibitor{
		&systemdInhibitor{},
		&dbusInhibitor{
			name: "dbus-gnome",
			dbusStrategy: dbusStrategy{
				dest:   "org.gnome.SessionManager",
				path:   "/org/gnome/SessionManager",
				iface:  "org.gnome.SessionManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "uint32:0", "string:User requested keep-alive", "uint32:12"},
			},
			unInhibitArg: "Uninhibit",
		},
		&dbusInhibitor{
			name: "dbus-freedesktop",
			dbusStrategy: dbusStrategy{
				dest:   "org.freedesktop.ScreenSaver",
				path:   "/org/freedesktop/ScreenSaver",
				iface:  "org.freedesktop.ScreenSaver",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "string:Keep system awake"},
			},
			unInhibitArg: "UnInhibit",
		},
		&dbusInhibitor{
			name: "dbus-kde",
			dbusStrategy: dbusStrategy{
				dest:   "org.freedesktop.PowerManagement.Inhibit",
				path:   "/org/freedesktop/PowerManagement/Inhibit",
				iface:  "org.freedesktop.PowerManagement.Inhibit",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "string:Keep system awake"},
			},
			unInhibitArg: "UnInhibit",
		},
		&dbusInhibitor{
			name: "dbus-xfce",
			dbusStrategy: dbusStrategy{
				dest:   "org.xfce.PowerManager",
				path:   "/org/xfce/PowerManager",
				iface:  "org.xfce.PowerManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "string:Keep system awake"},
			},
			unInhibitArg: "UnInhibit",
		},
		&dbusInhibitor{
			name: "dbus-mate",
			dbusStrategy: dbusStrategy{
				dest:   "org.mate.SessionManager",
				path:   "/org/mate/SessionManager",
				iface:  "org.mate.SessionManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "uint32:0", "string:Keep system awake", "uint32:12"},
			},
			unInhibitArg: "Uninhibit",
		},
		&gsettingsInhibitor{},
		&xsetInhibitor{},
	}
}

func (k *linuxKeepAlive) activateInhibitors(ctx context.Context) (int, error) {
	allInhibitors := buildLinuxInhibitors()
	activeCount := 0

	for _, inh := range allInhibitors {
		if err := inh.Activate(ctx); err == nil {
			k.inhibitors = append(k.inhibitors, inh)
			log.Printf("linux: activated inhibitor: %s", inh.Name())
			activeCount++
		} else {
			log.Printf("linux: inhibitor %s skipped: %v", inh.Name(), err)
		}
	}

	if activeCount == 0 {
		return 0, fmt.Errorf("linux: no keep-alive method successfully activated")
	}

	return activeCount, nil
}

func (k *linuxKeepAlive) setupUinput() {
	k.uinput = &uinputSimulator{}
	if err := k.uinput.setup(); err != nil {
		log.Printf("linux: uinput setup failed (likely permissions): %v", err)
		k.uinput = nil
	} else {
		log.Printf("linux: native uinput mouse simulation activated")
	}
}

func (k *linuxKeepAlive) startActivityTickerLocked(ctx context.Context) {
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
				// System keep-alive refresh (no activity simulation here)
			}
		}
	}()
}

func (k *linuxKeepAlive) startChatAppTickerLocked(ctx context.Context, caps linuxCapabilities) {
	if !k.simulateActivity {
		return
	}

	k.chatAppTick = time.NewTicker(ChatAppActivityInterval)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.chatAppTick.Stop()

		if !caps.xprintidleAvailable {
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

func (k *linuxKeepAlive) simulateChatAppActivity(ctx context.Context, caps linuxCapabilities) {
	shouldSimulate := true
	if caps.xprintidleAvailable {
		idle, err := getLinuxIdleTime()
		if err == nil && idle <= IdleThreshold {
			shouldSimulate = false
		}
	}

	if !shouldSimulate {
		return
	}

	points := k.patternGen.GenerateShapePoints()
	k.executeMousePattern(points, caps)
}

func (k *linuxKeepAlive) executeMousePattern(points []MousePoint, caps linuxCapabilities) {
	// Execute pattern using available methods
	if k.uinput != nil {
		k.executePatternUinput(points)
	}

	if caps.xdotoolAvailable {
		k.executePatternXdotool(points)
	}

	// Soft simulation via DBus
	runBestEffort("dbus-send", "--dest=org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.SimulateUserActivity")
	runBestEffort("dbus-send", "--dest=org.gnome.ScreenSaver", "/org/gnome/ScreenSaver", "org.gnome.ScreenSaver.SimulateUserActivity")
}

func (k *linuxKeepAlive) executePatternUinput(points []MousePoint) {
	if k.uinput == nil {
		return
	}

	// Execute pattern with natural timing
	for i, pt := range points {
		dx := int32(pt.X)
		dy := int32(pt.Y)
		k.uinput.move(dx, dy)

		distance := SegmentDistance(points, i)
		delay := k.patternGen.MovementDelay(distance)
		time.Sleep(delay)

		if k.patternGen.ShouldPause() {
			time.Sleep(k.patternGen.PauseDelay())
		}

		if k.patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := k.patternGen.IntermediatePoint(points, i, delay)
			k.uinput.move(int32(midPt.X), int32(midPt.Y))
			time.Sleep(midDelay)
		}
	}

	// Return to origin
	lastPt := points[len(points)-1]
	returnDelay := k.patternGen.ReturnDelay()
	k.uinput.move(-int32(lastPt.X), -int32(lastPt.Y))
	time.Sleep(returnDelay)
}

func (k *linuxKeepAlive) executePatternXdotool(points []MousePoint) {
	for i, pt := range points {
		dx := int(pt.X)
		dy := int(pt.Y)
		runBestEffort("xdotool", "mousemove_relative", "--", fmt.Sprintf("%d", dx), fmt.Sprintf("%d", dy))

		distance := SegmentDistance(points, i)
		delay := k.patternGen.MovementDelay(distance)
		time.Sleep(delay)

		if k.patternGen.ShouldPause() {
			time.Sleep(k.patternGen.PauseDelay())
		}

		if k.patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := k.patternGen.IntermediatePoint(points, i, delay)
			runBestEffort("xdotool", "mousemove_relative", "--", fmt.Sprintf("%d", int(midPt.X)), fmt.Sprintf("%d", int(midPt.Y)))
			time.Sleep(midDelay)
		}
	}

	// Return to origin
	lastPt := points[len(points)-1]
	returnDelay := k.patternGen.ReturnDelay()
	runBestEffort("xdotool", "mousemove_relative", "--", fmt.Sprintf("%d", -int(lastPt.X)), fmt.Sprintf("%d", -int(lastPt.Y)))
	time.Sleep(returnDelay)
}

func (k *linuxKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	k.ctx, k.cancel = context.WithCancel(ctx)

	// Initialize random source and pattern generator
	k.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	k.patternGen = NewMousePatternGenerator(k.rnd)

	// Activate inhibitors
	activeCount, err := k.activateInhibitors(k.ctx)
	if err != nil {
		k.cancel()
		return err
	}

	// Setup uinput if available
	k.setupUinput()

	caps := detectLinuxCapabilities()
	if k.uinput != nil {
		caps.uinputAvailable = true
	}

	log.Printf("linux: started; active inhibitors: %d (Wayland=%v, DISPLAY=%q)", activeCount, os.Getenv("WAYLAND_DISPLAY") != "", os.Getenv("DISPLAY"))

	k.startActivityTickerLocked(k.ctx)
	k.startChatAppTickerLocked(k.ctx, caps)

	k.isRunning = true
	return nil
}

func (k *linuxKeepAlive) Stop() error {
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

	// Deactivate all inhibitors in reverse order, tracking failures
	var deactivateErrors []error
	inhibitors := make([]inhibitor, len(k.inhibitors))
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
	case <-time.After(2 * time.Second):
		log.Printf("linux: warning: some goroutines did not complete within timeout")
	}

	// Deactivate inhibitors (best effort - continue even if some fail)
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
	
	// Cleanup uinput device
	if k.uinput != nil {
		fdBeforeClose := k.uinput.fd
		k.uinput.close()
		// Verify uinput is closed by checking if fd was non-zero before close
		if fdBeforeClose != 0 {
			if k.uinput.fd == 0 {
				log.Printf("linux: uinput device closed successfully")
			} else {
				log.Printf("linux: warning: uinput device may not have closed properly (fd=%d)", k.uinput.fd)
			}
		}
		k.uinput = nil
	}

	k.inhibitors = nil
	k.isRunning = false
	k.ctx = nil
	k.cancel = nil
	k.activityTick = nil
	k.mu.Unlock()

	if len(deactivateErrors) > 0 {
		log.Printf("linux: stopped with %d inhibitor deactivation errors", len(deactivateErrors))
		return fmt.Errorf("linux: %d inhibitors failed to deactivate", len(deactivateErrors))
	}

	log.Printf("linux: stopped; cleanup complete")
	return nil
}

func (k *linuxKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.simulateActivity = simulate

	if !k.isRunning {
		return
	}

	if simulate {
		// Start chat app ticker if not already running
		if k.chatAppTick == nil {
			caps := detectLinuxCapabilities()
			if k.uinput != nil {
				caps.uinputAvailable = true
			}
			k.startChatAppTickerLocked(k.ctx, caps)
		}
	} else {
		// Stop chat app ticker
		if k.chatAppTick != nil {
			k.chatAppTick.Stop()
			k.chatAppTick = nil
		}
	}
}

func NewKeepAlive() (KeepAlive, error) {
	return &linuxKeepAlive{}, nil
}

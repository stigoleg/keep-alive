//go:build linux

package platform

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
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

// --- Platform Implementation ---

type linuxKeepAlive struct {
	mu           sync.Mutex
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	isRunning    bool
	activityTick *time.Ticker
	inhibitors   []inhibitor

	simulateActivity bool
}

func (k *linuxKeepAlive) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.isRunning {
		return nil
	}

	ctx, k.cancel = context.WithCancel(ctx)

	// Define all possible inhibitors in priority order
	allInhibitors := []inhibitor{
		&systemdInhibitor{},
		&dbusInhibitor{
			name: "dbus-gnome",
			dbusStrategy: dbusStrategy{
				dest:   "org.gnome.SessionManager",
				path:   "/org/gnome/SessionManager",
				iface:  "org.gnome.SessionManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "uint32:0", "string:User requested keep-alive", "uint32:4"},
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
				args:   []string{"string:keep-alive", "uint32:0", "string:Keep system awake", "uint32:4"},
			},
			unInhibitArg: "Uninhibit",
		},
		&gsettingsInhibitor{},
		&xsetInhibitor{},
	}

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
		k.cancel()
		return fmt.Errorf("linux: no keep-alive method successfully activated")
	}

	log.Printf("linux: started; active inhibitors: %d (Wayland=%v, DISPLAY=%q)", activeCount, os.Getenv("WAYLAND_DISPLAY") != "", os.Getenv("DISPLAY"))

	// Activity simulation (best-effort movement)
	k.activityTick = time.NewTicker(30 * time.Second)
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		defer k.activityTick.Stop()

		xdotoolAvailable := hasCommand("xdotool")
		for {
			select {
			case <-ctx.Done():
				return
			case <-k.activityTick.C:
				if xdotoolAvailable && k.simulateActivity {
					runBestEffort("xdotool", "mousemove_relative", "1", "0")
					runBestEffort("xdotool", "mousemove_relative", "-1", "0")
				}
			}
		}
	}()

	k.isRunning = true
	return nil
}

func (k *linuxKeepAlive) Stop() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.isRunning {
		return nil
	}

	if k.cancel != nil {
		k.cancel()
	}

	// Deactivate all inhibitors in reverse order
	for i := len(k.inhibitors) - 1; i >= 0; i-- {
		inh := k.inhibitors[i]
		if err := inh.Deactivate(); err != nil {
			log.Printf("linux: error deactivating inhibitor %s: %v", inh.Name(), err)
		} else {
			log.Printf("linux: deactivated inhibitor %s", inh.Name())
		}
	}
	k.inhibitors = nil

	k.wg.Wait()
	k.isRunning = false
	log.Printf("linux: stopped; cleanup complete")
	return nil
}

func (k *linuxKeepAlive) SetSimulateActivity(simulate bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.simulateActivity = simulate
}

func NewKeepAlive() (KeepAlive, error) {
	return &linuxKeepAlive{}, nil
}

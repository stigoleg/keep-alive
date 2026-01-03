//go:build linux

package linux

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Inhibitor timing constants.
const (
	inhibitorVerifyDelay = 100 * time.Millisecond
)

// GNOME SessionManager inhibit flags.
const (
	gnomeInhibitSuspend = 4  // Inhibit suspending the session
	gnomeInhibitIdle    = 8  // Inhibit the session being marked as idle
	gnomeInhibitBoth    = 12 // Inhibit both suspend and idle
)

// Inhibitor defines the common interface for various Linux sleep prevention methods.
type Inhibitor interface {
	Name() string
	Activate(ctx context.Context) error
	Deactivate() error
}

// SystemdInhibitor implements sleep prevention using systemd-inhibit.
type SystemdInhibitor struct {
	cmd *exec.Cmd
}

func (s *SystemdInhibitor) Name() string { return "systemd-inhibit" }

func (s *SystemdInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("systemd-inhibit") {
		return fmt.Errorf("systemd-inhibit command not found")
	}

	s.cmd = exec.CommandContext(ctx, "systemd-inhibit",
		"--what=idle:sleep:handle-lid-switch:shutdown",
		"--who=keep-alive",
		"--why=User requested keep-alive",
		"--mode=block",
		"sh", "-c", "while true; do sleep 1; done")

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start systemd-inhibit: %w", err)
	}

	if s.cmd.Process == nil {
		return fmt.Errorf("systemd-inhibit process is nil after Start()")
	}

	time.Sleep(inhibitorVerifyDelay)
	if err := s.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("systemd-inhibit process verification failed: %w", err)
	}

	log.Printf("linux: systemd-inhibit started successfully (pid %d)", s.cmd.Process.Pid)
	return nil
}

func (s *SystemdInhibitor) Deactivate() error {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}

// Cmd returns the underlying exec.Cmd for verification purposes.
func (s *SystemdInhibitor) Cmd() *exec.Cmd {
	return s.cmd
}

// dbusStrategy provides common functionality for DBus-based inhibitors.
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
	parts := strings.Fields(out)
	if len(parts) > 0 {
		lastPart := strings.TrimRight(parts[len(parts)-1], ")")
		if val, err := strconv.ParseUint(lastPart, 10, 32); err == nil {
			return uint32(val), nil
		}
	}
	return 0, fmt.Errorf("failed to parse cookie from: %q", out)
}

// DBusInhibitor implements sleep prevention using DBus calls.
type DBusInhibitor struct {
	dbusStrategy
	name         string
	unInhibitArg string
}

func (d *DBusInhibitor) Name() string { return d.name }

func (d *DBusInhibitor) Activate(ctx context.Context) error {
	out, err := d.call(d.method, d.args...)
	if err != nil {
		return fmt.Errorf("dbus call failed (output: %q): %w", out, err)
	}
	cookie, err := d.parseCookie(out)
	if err != nil {
		return fmt.Errorf("failed to parse cookie from dbus response (output: %q): %w", out, err)
	}
	if cookie == 0 {
		return fmt.Errorf("received invalid cookie (0) from dbus inhibitor %s", d.name)
	}
	d.cookie = cookie
	log.Printf("linux: dbus inhibitor %s activated with cookie %d", d.name, cookie)
	return nil
}

func (d *DBusInhibitor) Deactivate() error {
	if d.cookie == 0 {
		return nil
	}
	_, err := d.call(d.unInhibitArg, "uint32:"+strconv.FormatUint(uint64(d.cookie), 10))
	return err
}

// Cookie returns the inhibitor cookie for verification purposes.
func (d *DBusInhibitor) Cookie() uint32 {
	return d.cookie
}

// GsettingsInhibitor implements sleep prevention by modifying GNOME settings.
type GsettingsInhibitor struct {
	prevSettings map[string]string
}

func (g *GsettingsInhibitor) Name() string { return "gsettings" }

func (g *GsettingsInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("gsettings") {
		return fmt.Errorf("gsettings command not found")
	}
	g.prevSettings = make(map[string]string)

	settings := []struct{ schema, key, value string }{
		{"org.gnome.desktop.session", "idle-delay", "0"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-ac-type", "'nothing'"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-battery-type", "'nothing'"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-ac-timeout", "0"},
		{"org.gnome.settings-daemon.plugins.power", "sleep-inactive-battery-timeout", "0"},
		{"org.gnome.settings-daemon.plugins.power", "idle-dim", "false"},
	}

	var failedSettings []string
	for _, s := range settings {
		if out, err := runVerbose("gsettings", "get", s.schema, s.key); err == nil {
			g.prevSettings[s.schema+" "+s.key] = out
		}
		if out, err := runVerbose("gsettings", "set", s.schema, s.key, s.value); err != nil {
			failedSettings = append(failedSettings, fmt.Sprintf("%s.%s: %v", s.schema, s.key, err))
			log.Printf("linux: gsettings set failed for %s.%s: %v (out: %q)", s.schema, s.key, err, out)
		}
	}

	if len(failedSettings) > 0 {
		log.Printf("linux: gsettings: some settings failed to apply: %v", failedSettings)
		if len(failedSettings) == len(settings) {
			return fmt.Errorf("all gsettings failed to apply: %v", failedSettings)
		}
	}

	return nil
}

func (g *GsettingsInhibitor) Deactivate() error {
	for k, v := range g.prevSettings {
		parts := strings.SplitN(k, " ", 2)
		runBestEffort("gsettings", "set", parts[0], parts[1], v)
	}
	return nil
}

// XsetInhibitor implements sleep prevention using xset (X11 only).
type XsetInhibitor struct{}

func (x *XsetInhibitor) Name() string { return "xset" }

func (x *XsetInhibitor) Activate(ctx context.Context) error {
	if !hasCommand("xset") || os.Getenv("DISPLAY") == "" {
		return fmt.Errorf("xset not available or DISPLAY not set")
	}
	runBestEffort("xset", "s", "off")
	runBestEffort("xset", "-dpms")
	return nil
}

func (x *XsetInhibitor) Deactivate() error {
	runBestEffort("xset", "s", "on")
	runBestEffort("xset", "+dpms")
	return nil
}

// CreateGNOMESuspendInhibitor creates a DBus inhibitor for GNOME suspend prevention.
func CreateGNOMESuspendInhibitor(name string) *DBusInhibitor {
	return &DBusInhibitor{
		name: name,
		dbusStrategy: dbusStrategy{
			dest:   "org.gnome.SessionManager",
			path:   "/org/gnome/SessionManager",
			iface:  "org.gnome.SessionManager",
			method: "Inhibit",
			args:   []string{"string:keep-alive", "uint32:0", "string:Prevent system suspend", fmt.Sprintf("uint32:%d", gnomeInhibitSuspend)},
		},
		unInhibitArg: "Uninhibit",
	}
}

// CreateGNOMEIdleInhibitor creates a DBus inhibitor for GNOME idle prevention.
func CreateGNOMEIdleInhibitor(name string) *DBusInhibitor {
	return &DBusInhibitor{
		name: name,
		dbusStrategy: dbusStrategy{
			dest:   "org.gnome.SessionManager",
			path:   "/org/gnome/SessionManager",
			iface:  "org.gnome.SessionManager",
			method: "Inhibit",
			args:   []string{"string:keep-alive", "uint32:0", "string:Prevent session idle", fmt.Sprintf("uint32:%d", gnomeInhibitIdle)},
		},
		unInhibitArg: "Uninhibit",
	}
}

// BuildInhibitors builds a prioritized list of inhibitors based on detected desktop environment.
func BuildInhibitors() []Inhibitor {
	de := DetectDesktopEnvironment()
	displayServer := DetectDisplayServer()
	inhibitors := []Inhibitor{}

	// Always try systemd-inhibit first (works on all systems)
	inhibitors = append(inhibitors, &SystemdInhibitor{})

	// Add DE-specific inhibitors
	switch de {
	case DesktopCosmic:
		inhibitors = append(inhibitors, CreateGNOMESuspendInhibitor("dbus-cosmic-suspend"))
		inhibitors = append(inhibitors, CreateGNOMEIdleInhibitor("dbus-cosmic-idle"))
		inhibitors = append(inhibitors, &GsettingsInhibitor{})
	case DesktopGNOME:
		inhibitors = append(inhibitors, CreateGNOMESuspendInhibitor("dbus-gnome-suspend"))
		inhibitors = append(inhibitors, CreateGNOMEIdleInhibitor("dbus-gnome-idle"))
		inhibitors = append(inhibitors, &GsettingsInhibitor{})
	case DesktopKDE:
		inhibitors = append(inhibitors, &DBusInhibitor{
			name: "dbus-kde",
			dbusStrategy: dbusStrategy{
				dest:   "org.freedesktop.PowerManagement.Inhibit",
				path:   "/org/freedesktop/PowerManagement/Inhibit",
				iface:  "org.freedesktop.PowerManagement.Inhibit",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "string:Keep system awake"},
			},
			unInhibitArg: "UnInhibit",
		})
	case DesktopXFCE:
		inhibitors = append(inhibitors, &DBusInhibitor{
			name: "dbus-xfce",
			dbusStrategy: dbusStrategy{
				dest:   "org.xfce.PowerManager",
				path:   "/org/xfce/PowerManager",
				iface:  "org.xfce.PowerManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "string:Keep system awake"},
			},
			unInhibitArg: "UnInhibit",
		})
	case DesktopMATE:
		inhibitors = append(inhibitors, &DBusInhibitor{
			name: "dbus-mate",
			dbusStrategy: dbusStrategy{
				dest:   "org.mate.SessionManager",
				path:   "/org/mate/SessionManager",
				iface:  "org.mate.SessionManager",
				method: "Inhibit",
				args:   []string{"string:keep-alive", "uint32:0", "string:Keep system awake", fmt.Sprintf("uint32:%d", gnomeInhibitBoth)},
			},
			unInhibitArg: "Uninhibit",
		})
	}

	// Add freedesktop fallback
	inhibitors = append(inhibitors, &DBusInhibitor{
		name: "dbus-freedesktop",
		dbusStrategy: dbusStrategy{
			dest:   "org.freedesktop.ScreenSaver",
			path:   "/org/freedesktop/ScreenSaver",
			iface:  "org.freedesktop.ScreenSaver",
			method: "Inhibit",
			args:   []string{"string:keep-alive", "string:Keep system awake"},
		},
		unInhibitArg: "UnInhibit",
	})

	// xset only works on X11
	if displayServer == DisplayServerX11 {
		inhibitors = append(inhibitors, &XsetInhibitor{})
	}

	return inhibitors
}

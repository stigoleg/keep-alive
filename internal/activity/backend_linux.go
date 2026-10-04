//go:build linux

package activity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// cmdTimeout bounds every helper process (xdotool, ydotool, xprintidle).
const cmdTimeout = 3 * time.Second

const uinputPermissionHint = `give your user access to /dev/uinput: run "sudo usermod -aG input $USER", ` +
	`add the udev rule KERNEL=="uinput", MODE="0660", GROUP="input" to /etc/udev/rules.d/60-uinput.rules, ` +
	`then reboot or log out and back in`

// linuxEnv is the slice of the environment that picks backends.
type linuxEnv struct {
	display, wayland, desktop string
}

func currentEnv() linuxEnv {
	return linuxEnv{
		display: os.Getenv("DISPLAY"),
		wayland: os.Getenv("WAYLAND_DISPLAY"),
		desktop: strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")),
	}
}

// x11 reports a plain X11 session (not XWayland).
func (e linuxEnv) x11() bool { return e.display != "" && e.wayland == "" }

func (e linuxEnv) kde() bool { return strings.Contains(e.desktop, "KDE") }

func newBackend(keys bool) *backend {
	env := currentEnv()
	b := &backend{noIdleHint: noIdleHint(env)}
	sess, err := connectSessionBus()
	if err != nil {
		slog.Debug("activity: no session bus", "err", err)
	}
	sys, err := dbus.ConnectSystemBus()
	if err != nil {
		slog.Debug("activity: no system bus", "err", err)
		sys = nil
	}
	b.release = func() {
		for _, c := range []*dbus.Conn{sess, sys} {
			if c != nil {
				_ = c.Close()
			}
		}
	}
	b.sources = idleSources(env, sess, exec.LookPath)
	if len(b.sources) > 0 {
		b.idle = b.sources[0]
	}
	b.lock = newLinuxLock(sys, sess)
	switch b.lock.(type) {
	case logindLock:
		b.lockName = "logind LockedHint"
	case screensaverLock:
		b.lockName = "org.freedesktop.ScreenSaver"
	}
	b.open = func() (Injector, error) { return openLinux(env, keys) }
	b.candidates = func() []Injector { return linuxCandidates(env, keys) }
	b.env = env.describe()
	return b
}

// describe is the desktop as doctor shows it.
func (e linuxEnv) describe() []string {
	server := "none (no DISPLAY or WAYLAND_DISPLAY)"
	switch {
	case e.wayland != "" && e.display != "":
		server = "Wayland (with XWayland)"
	case e.wayland != "":
		server = "Wayland"
	case e.display != "":
		server = "X11"
	}
	desktop := e.desktop
	if desktop == "" {
		desktop = "unknown (XDG_CURRENT_DESKTOP is not set)"
	}
	return []string{"display server: " + server, "desktop: " + desktop}
}

func linuxCandidates(env linuxEnv, keys bool) []Injector {
	return []Injector{newUinput(keys), newYdotool(), newXdotool(env)}
}

// openLinux tries uinput, then ydotool 1.x, then xdotool (X11 only).
func openLinux(env linuxEnv, keys bool) (Injector, error) {
	candidates := linuxCandidates(env, keys)
	var reasons []string
	hint := uinputPermissionHint
	for i, inj := range candidates {
		err := inj.Available()
		if err == nil {
			return inj, nil
		}
		reasons = append(reasons, inj.Name()+": "+err.Error())
		var u *Unavailable
		if i == 0 && errors.As(err, &u) && u.Hint != "" {
			hint = u.Hint
		}
	}
	if env.x11() {
		hint = "install xdotool, or " + hint
	}
	return nil, &Unavailable{
		Reason: "no input method works (" + strings.Join(reasons, "; ") + ")",
		Hint:   hint,
	}
}

// noIdleHint explains how to get idle-aware simulation on this desktop.
func noIdleHint(env linuxEnv) string {
	switch {
	case env.wayland != "" && env.kde():
		return "KDE Plasma on Wayland does not share idle time with other programs; log in to a Plasma (X11) session for idle-aware simulation"
	case env.wayland != "":
		return "this Wayland compositor (e.g. sway, Hyprland) only reports idle time to Wayland clients; use GNOME or an X11 session with xprintidle for idle-aware simulation"
	default:
		return "install xprintidle for idle-aware simulation"
	}
}

// cmdRunner runs a helper with a timeout and returns its stdout and stderr.
type cmdRunner func(ctx context.Context, timeout time.Duration, env []string, name string, args ...string) (string, string, error)

func runCmd(ctx context.Context, timeout time.Duration, env []string, name string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	out, errOut := strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String())
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, errOut, fmt.Errorf("%s timed out after %s", name, timeout)
	}
	if err != nil {
		if line, _, _ := strings.Cut(errOut, "\n"); line != "" {
			return out, errOut, fmt.Errorf("%s: %w: %s", name, err, line)
		}
		return out, errOut, fmt.Errorf("%s: %w", name, err)
	}
	return out, errOut, nil
}

// dbusTimeout bounds every D-Bus call.
const dbusTimeout = 2 * time.Second

// connectSessionBus connects to an existing session bus. Unlike
// dbus.ConnectSessionBus it never starts one with dbus-launch.
func connectSessionBus() (*dbus.Conn, error) {
	conn, err := dbus.SessionBusPrivateNoAutoStartup()
	if err != nil {
		return nil, err
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func hasOwner(conn *dbus.Conn, name string) bool {
	if conn == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer cancel()
	var ok bool
	err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, name).Store(&ok)
	return err == nil && ok
}

// dbusCall calls method on dest/path and returns the reply body.
func dbusCall(conn *dbus.Conn, dest string, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer cancel()
	call := conn.Object(dest, path).CallWithContext(ctx, method, 0, args...)
	if call.Err != nil {
		return nil, call.Err
	}
	return call.Body, nil
}

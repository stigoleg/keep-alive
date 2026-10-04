//go:build linux

package activity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"
)

// screensaverServices are the screensaver interfaces Chromium asks
// (ui/base/idle/idle_linux.cc). GetActive is true while the screen is
// locked or blanked.
var screensaverServices = []struct {
	name  string
	path  dbus.ObjectPath
	iface string
}{
	{"org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver"},
	{"org.gnome.ScreenSaver", "/org/gnome/ScreenSaver", "org.gnome.ScreenSaver"},
	{"org.mate.ScreenSaver", "/org/mate/ScreenSaver", "org.mate.ScreenSaver"},
	{"org.cinnamon.ScreenSaver", "/org/cinnamon/ScreenSaver", "org.cinnamon.ScreenSaver"},
	{"org.xfce.ScreenSaver", "/org/xfce/ScreenSaver", "org.xfce.ScreenSaver"},
}

// newLinuxLock collects every lock source that answers: logind's
// LockedHint and each screensaver that runs. Sources that are absent or
// answer NotSupported are left out; nil when none answers.
func newLinuxLock(ctx context.Context, sys, sess *dbus.Conn) anyLock {
	var out anyLock
	if sys != nil {
		path, err := logindSession(ctx, sys)
		if err == nil {
			l := logindLock{ctx: ctx, conn: sys, path: path}
			if _, err = l.Locked(); err == nil {
				out = append(out, namedLock{"logind LockedHint", l})
			}
		}
		if err != nil {
			slog.Debug("activity: logind lock state unavailable", "err", err)
		}
	}
	for _, svc := range screensaverServices {
		if !hasOwner(ctx, sess, svc.name) {
			continue
		}
		s := screensaverLock{ctx: ctx, conn: sess, dest: svc.name, path: svc.path, iface: svc.iface}
		if _, err := s.Locked(); err != nil {
			slog.Debug("activity: screensaver lock state unavailable", "service", svc.name, "err", err)
			continue
		}
		out = append(out, namedLock{svc.name, s})
	}
	return out
}

// namedLock is a lock source with the name doctor shows.
type namedLock struct {
	name string
	LockSource
}

// anyLock reports the session locked when any source says so, as Chromium
// does: logind may miss a screensaver lock and the other way round. A
// source that fails is skipped; the state is unknown only when all fail.
type anyLock []namedLock

func (a anyLock) Locked() (bool, error) {
	var errs []error
	for _, s := range a {
		locked, err := s.Locked()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
			continue
		}
		if locked {
			return true, nil
		}
	}
	if len(errs) == len(a) {
		return false, errors.Join(errs...)
	}
	return false, nil
}

// String lists the sources, e.g. "logind LockedHint, org.gnome.ScreenSaver".
func (a anyLock) String() string {
	names := make([]string, len(a))
	for i, s := range a {
		names[i] = s.name
	}
	return strings.Join(names, ", ")
}

// logindSession finds our login session: by PID, then $XDG_SESSION_ID, then
// the user's graphical session (a terminal started by the systemd user
// manager belongs to no session).
func logindSession(ctx context.Context, conn *dbus.Conn) (dbus.ObjectPath, error) {
	const dest = "org.freedesktop.login1"
	if body, err := dbusCall(ctx, conn, dest, "/org/freedesktop/login1", "org.freedesktop.login1.Manager.GetSessionByPID", uint32(os.Getpid())); err == nil {
		if p, ok := singlePath(body); ok {
			return p, nil
		}
	}
	if id := os.Getenv("XDG_SESSION_ID"); id != "" {
		if body, err := dbusCall(ctx, conn, dest, "/org/freedesktop/login1", "org.freedesktop.login1.Manager.GetSession", id); err == nil {
			if p, ok := singlePath(body); ok {
				return p, nil
			}
		}
	}
	body, err := dbusCall(ctx, conn, dest, "/org/freedesktop/login1/user/self", "org.freedesktop.DBus.Properties.Get",
		"org.freedesktop.login1.User", "Display")
	if err != nil {
		return "", err
	}
	if v, ok := singleVariant(body); ok {
		// Display is (so): session id and object path.
		if pair, ok := v.Value().([]any); ok && len(pair) == 2 {
			if p, ok := pair[1].(dbus.ObjectPath); ok && p.IsValid() && p != "/" {
				return p, nil
			}
		}
	}
	return "", errors.New("no graphical login session")
}

type logindLock struct {
	ctx  context.Context
	conn *dbus.Conn
	path dbus.ObjectPath
}

func (l logindLock) Locked() (bool, error) {
	body, err := dbusCall(l.ctx, l.conn, "org.freedesktop.login1", l.path, "org.freedesktop.DBus.Properties.Get",
		"org.freedesktop.login1.Session", "LockedHint")
	if err != nil {
		return false, err
	}
	if v, ok := singleVariant(body); ok {
		if b, ok := v.Value().(bool); ok {
			return b, nil
		}
	}
	return false, fmt.Errorf("unexpected LockedHint reply %v", body)
}

// screensaverLock asks one screensaver service's GetActive.
type screensaverLock struct {
	ctx         context.Context
	conn        *dbus.Conn
	dest, iface string
	path        dbus.ObjectPath
}

func (s screensaverLock) Locked() (bool, error) {
	body, err := dbusCall(s.ctx, s.conn, s.dest, s.path, s.iface+".GetActive")
	if err != nil {
		return false, err
	}
	if len(body) == 1 {
		if b, ok := body[0].(bool); ok {
			return b, nil
		}
	}
	return false, fmt.Errorf("unexpected GetActive reply %v", body)
}

func singlePath(body []any) (dbus.ObjectPath, bool) {
	if len(body) != 1 {
		return "", false
	}
	p, ok := body[0].(dbus.ObjectPath)
	return p, ok && p.IsValid()
}

func singleVariant(body []any) (dbus.Variant, bool) {
	if len(body) != 1 {
		return dbus.Variant{}, false
	}
	v, ok := body[0].(dbus.Variant)
	return v, ok
}

//go:build linux

package activity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/godbus/dbus/v5"
)

// newLinuxLock prefers logind's LockedHint and falls back to the
// freedesktop screensaver; nil when neither is reachable.
func newLinuxLock(ctx context.Context, sys, sess *dbus.Conn) LockSource {
	if sys != nil {
		path, err := logindSession(ctx, sys)
		if err == nil {
			l := logindLock{ctx: ctx, conn: sys, path: path}
			if _, err = l.Locked(); err == nil {
				return l
			}
		}
		slog.Debug("activity: logind lock state unavailable", "err", err)
	}
	if hasOwner(ctx, sess, "org.freedesktop.ScreenSaver") {
		s := screensaverLock{ctx: ctx, conn: sess}
		if _, err := s.Locked(); err == nil {
			return s
		}
	}
	return nil
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

type screensaverLock struct {
	ctx  context.Context
	conn *dbus.Conn
}

func (s screensaverLock) Locked() (bool, error) {
	body, err := dbusCall(s.ctx, s.conn, "org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.GetActive")
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

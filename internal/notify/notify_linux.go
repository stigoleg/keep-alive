package notify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	busName = "org.freedesktop.Notifications"
	busPath = "/org/freedesktop/Notifications"
	// dbusTimeout leaves part of Timeout for the notify-send fallback.
	dbusTimeout = 3 * time.Second
)

type freedesktop struct{}

func newPlatform() Notifier { return freedesktop{} }

// Notify calls org.freedesktop.Notifications.Notify on the session bus and
// falls back to notify-send when that fails.
func (freedesktop) Notify(ctx context.Context, title, body string) error {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	dctx, dcancel := context.WithTimeout(ctx, dbusTimeout)
	err := onBus(dctx, func(conn *dbus.Conn) error {
		return conn.Object(busName, busPath).CallWithContext(dctx, busName+".Notify", 0, notifyArgs(title, body)...).Err
	})
	dcancel()
	if err == nil {
		return nil
	}
	if _, lerr := exec.LookPath("notify-send"); lerr != nil {
		return fmt.Errorf("notify: D-Bus: %w (notify-send is not installed)", err)
	}
	if serr := run(ctx, "notify-send", notifySendArgs(title, body), nil); serr != nil {
		return fmt.Errorf("notify: D-Bus: %v; %w", err, serr)
	}
	return nil
}

// notifyArgs are the Notify arguments: app_name, replaces_id, app_icon,
// summary, body, actions, hints, expire_timeout (-1: the server decides).
func notifyArgs(title, body string) []any {
	return []any{appName, uint32(0), "", title, body, []string{}, map[string]dbus.Variant{}, int32(-1)}
}

// onBus runs f on a private session-bus connection without ever starting a
// bus, and returns when ctx ends even if the bus does not answer.
func onBus(ctx context.Context, f func(*dbus.Conn) error) error {
	done := make(chan error, 1)
	go func() {
		conn, err := dbus.SessionBusPrivateNoAutoStartup(dbus.WithContext(ctx))
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		if err := conn.Auth(nil); err != nil {
			done <- err
			return
		}
		if err := conn.Hello(); err != nil {
			done <- err
			return
		}
		done <- f(conn)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func available() (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), dbusTimeout)
	defer cancel()
	var server, version string
	err := onBus(ctx, func(conn *dbus.Conn) error {
		var vendor, spec string
		return conn.Object(busName, busPath).CallWithContext(ctx, busName+".GetServerInformation", 0).
			Store(&server, &vendor, &version, &spec)
	})
	if err == nil {
		return true, fmt.Sprintf("D-Bus %s (%s %s)", busName, server, version)
	}
	if path, lerr := exec.LookPath("notify-send"); lerr == nil {
		return true, fmt.Sprintf("notify-send (%s); D-Bus failed: %v", path, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		err = errors.New("the session bus did not answer")
	}
	return false, fmt.Sprintf("no notification service: D-Bus: %v; notify-send not installed", err)
}

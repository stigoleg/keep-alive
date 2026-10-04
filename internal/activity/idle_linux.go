//go:build linux

package activity

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// idleSources returns every idle source that answers, best first: Mutter's
// IdleMonitor, xprintidle on X11, KDE's ScreenSaver on X11, and xprintidle
// under XWayland as a last resort (it only sees input sent to X11 clients).
func idleSources(ctx context.Context, env linuxEnv, sess *dbus.Conn, lookPath func(string) (string, error)) []IdleSource {
	var candidates []IdleSource
	if hasOwner(ctx, sess, "org.gnome.Mutter.IdleMonitor") {
		candidates = append(candidates, mutterIdle{ctx, sess})
	}
	_, xerr := lookPath("xprintidle")
	if env.x11() && xerr == nil {
		candidates = append(candidates, xprintidle{ctx: ctx, run: runCmd})
	}
	if env.x11() && env.kde() && hasOwner(ctx, sess, "org.freedesktop.ScreenSaver") {
		candidates = append(candidates, kdeIdle{ctx, sess})
	}
	if env.display != "" && env.wayland != "" && xerr == nil {
		candidates = append(candidates, xprintidle{ctx: ctx, run: runCmd, xwayland: true})
	}
	var out []IdleSource
	for _, s := range candidates {
		if _, err := s.Idle(); err != nil {
			slog.Debug("activity: idle source unusable", "source", s.Name(), "err", err)
			continue
		}
		out = append(out, s)
	}
	return out
}

// mutterIdle is GNOME's idle monitor: GetIdletime returns uint64
// milliseconds. ctx bounds every call (see newBackend).
type mutterIdle struct {
	ctx  context.Context
	conn *dbus.Conn
}

func (mutterIdle) Name() string { return "Mutter IdleMonitor" }

func (m mutterIdle) Idle() (time.Duration, error) {
	body, err := dbusCall(m.ctx, m.conn, "org.gnome.Mutter.IdleMonitor", "/org/gnome/Mutter/IdleMonitor/Core",
		"org.gnome.Mutter.IdleMonitor.GetIdletime")
	if err != nil {
		return 0, err
	}
	return mutterIdleFromReply(body)
}

// mutterIdleFromReply decodes the typed reply. (v1 parsed the text of a
// gdbus call and read the "64" of "uint64" as the idle time.)
func mutterIdleFromReply(body []any) (time.Duration, error) {
	if len(body) != 1 {
		return 0, fmt.Errorf("GetIdletime returned %d values, want 1", len(body))
	}
	ms, ok := body[0].(uint64)
	if !ok {
		return 0, fmt.Errorf("GetIdletime returned %T, want uint64", body[0])
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// kdeIdle is KDE's org.freedesktop.ScreenSaver.GetSessionIdleTime, which
// returns milliseconds (its interface XML says seconds) and only works on
// X11.
type kdeIdle struct {
	ctx  context.Context
	conn *dbus.Conn
}

func (kdeIdle) Name() string { return "KDE ScreenSaver" }

func (k kdeIdle) Idle() (time.Duration, error) {
	body, err := dbusCall(k.ctx, k.conn, "org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver",
		"org.freedesktop.ScreenSaver.GetSessionIdleTime")
	if err != nil {
		return 0, err
	}
	return kdeIdleFromReply(body)
}

func kdeIdleFromReply(body []any) (time.Duration, error) {
	if len(body) != 1 {
		return 0, fmt.Errorf("GetSessionIdleTime returned %d values, want 1", len(body))
	}
	switch v := body[0].(type) {
	case uint32:
		return time.Duration(v) * time.Millisecond, nil
	case uint64:
		return time.Duration(v) * time.Millisecond, nil
	}
	return 0, fmt.Errorf("GetSessionIdleTime returned %T, want uint32", body[0])
}

// xprintidle prints the XScreenSaver idle time in milliseconds, the value
// Chromium reads on X11.
type xprintidle struct {
	ctx      context.Context
	run      cmdRunner
	xwayland bool
}

func (x xprintidle) Name() string {
	if x.xwayland {
		return "xprintidle (XWayland)"
	}
	return "xprintidle"
}

func (x xprintidle) Idle() (time.Duration, error) {
	out, _, err := x.run(x.ctx, cmdTimeout, nil, "xprintidle")
	if err != nil {
		return 0, err
	}
	return parseXprintidle(out)
}

func parseXprintidle(out string) (time.Duration, error) {
	ms, err := strconv.ParseUint(strings.TrimSpace(out), 10, 63)
	if err != nil {
		return 0, fmt.Errorf("unexpected xprintidle output %q", out)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

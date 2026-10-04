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

// linuxIdle is what the desktop tells about idle time.
type linuxIdle struct {
	// gate decides when to simulate and verifies bursts; nil means a fixed
	// schedule.
	gate IdleSource
	// xwayland is XWayland's counter on a Wayland session, read for
	// information only.
	xwayland IdleSource
	// sources is every source that answered, for Probe and Diagnose.
	sources []IdleSource
}

// idleSources reads the idle counters this desktop offers: Mutter's
// IdleMonitor, xprintidle (X11 or XWayland) and KDE's ScreenSaver on X11.
func idleSources(ctx context.Context, env linuxEnv, sess *dbus.Conn, lookPath func(string) (string, error)) linuxIdle {
	var mutter, xp, kde IdleSource
	if hasOwner(ctx, sess, "org.gnome.Mutter.IdleMonitor") {
		mutter = mutterIdle{ctx, sess}
	}
	if _, err := lookPath("xprintidle"); err == nil && env.display != "" {
		xp = xprintidle{ctx: ctx, run: runCmd, xwayland: env.wayland != ""}
	}
	if env.x11() && env.kde() && hasOwner(ctx, sess, "org.freedesktop.ScreenSaver") {
		kde = kdeIdle{ctx, sess}
	}
	return pickIdle(env, answering(mutter), answering(xp), answering(kde))
}

// answering returns s if it can be read, else nil.
func answering(s IdleSource) IdleSource {
	if s == nil {
		return nil
	}
	if _, err := s.Idle(); err != nil {
		slog.Debug("activity: idle source unusable", "source", s.Name(), "err", err)
		return nil
	}
	return s
}

// pickIdle chooses among the sources that answered (nil when absent). On
// Wayland only Mutter is trusted: XWayland's counter moves only when an X11
// client gets input, so it never gates or verifies, and without Mutter
// (KDE, wlroots) there is no trustworthy source at all. On X11 the order
// is Mutter, xprintidle, KDE.
func pickIdle(env linuxEnv, mutter, xprintidle, kde IdleSource) linuxIdle {
	var li linuxIdle
	for _, s := range []IdleSource{mutter, xprintidle, kde} {
		if s != nil {
			li.sources = append(li.sources, s)
		}
	}
	if env.wayland != "" {
		li.gate, li.xwayland = mutter, xprintidle
		return li
	}
	if len(li.sources) > 0 {
		li.gate = li.sources[0]
	}
	return li
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

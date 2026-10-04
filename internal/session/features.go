package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/proc"
)

// ---- work hours ----

// armSchedule records the next schedule change and arms its timer.
func (l *loop) armSchedule(now time.Time) {
	sched := l.s.cfg.Schedule
	if sched == nil {
		return
	}
	next, _ := sched.Next(now)
	l.snap.NextChange = next
	switch {
	case next.IsZero(): // always on: nothing ever changes
		if l.schedTimer != nil {
			l.schedTimer.Stop()
		}
	case l.schedTimer == nil:
		l.schedTimer = l.clk.NewTimer(next.Sub(now))
	default:
		l.schedTimer.Reset(next.Sub(now))
	}
}

// checkSchedule enters or leaves the work hours when the clock says so. It
// runs on the transition timer and on every heartbeat, so a missed timer
// (sleep, wall-clock changes) corrects itself.
func (l *loop) checkSchedule() {
	sched := l.s.cfg.Schedule
	if sched == nil {
		return
	}
	now := l.clk.Now()
	in := sched.In(now)
	changed := in != l.inWindow
	if changed && in {
		l.enterWindow()
	} else if changed {
		l.leaveWindow()
	}
	l.armSchedule(now)
	if changed {
		msg := l.scheduleMessage(now)
		slog.Info("session: " + msg)
		l.emit(EventSchedule, "", msg)
	}
}

func (l *loop) enterWindow() {
	l.inWindow, l.snap.InWindow = true, true
	hold, err := l.s.deps.Power.Acquire(l.ctx, l.powerOptions())
	if err != nil {
		l.warn(fmt.Sprintf("could not keep the system awake (%v); retrying", err))
		l.powerRetries = 0
		l.schedulePowerRetry()
	} else {
		l.hold = hold
		l.watchPower()
		l.snap.PowerHold = hold.Describe()
	}
	if l.snap.Active {
		l.startActivity()
	}
}

// leaveWindow pauses the session: no simulated activity, no power hold.
func (l *loop) leaveWindow() {
	l.inWindow, l.snap.InWindow = false, false
	l.stopActivity()
	l.snap.Activity = activity.Status{State: activity.StateOff}
	if l.powerRetry != nil {
		l.powerRetry.Stop()
	}
	_ = l.releaseHold()
}

// releaseHold releases the current hold, if any, and reports a failure as a
// warning.
func (l *loop) releaseHold() error {
	l.powerLost = nil
	l.snap.PowerHold = ""
	if l.hold == nil {
		return nil
	}
	err := l.hold.Release()
	l.hold = nil
	if err != nil {
		slog.Warn("session: releasing the power hold failed", "err", err)
		l.emit(EventWarning, "", fmt.Sprintf("could not release the power hold: %v", err))
	}
	return err
}

func (l *loop) scheduleMessage(now time.Time) string {
	next := l.snap.NextChange
	if l.inWindow {
		if next.IsZero() {
			return "work hours started"
		}
		return fmt.Sprintf("work hours started (until %s)", ClockText(now, next))
	}
	return fmt.Sprintf("outside work hours until %s", ClockText(now, next))
}

// ClockText formats t as "16:00" when it falls on now's day, else as
// "Mon 08:00".
func ClockText(now, t time.Time) string {
	t = t.In(now.Location())
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

// ---- process watch ----

// SystemProcesses is the system process list without this process, so
// "--while keepalive" does not watch itself.
func SystemProcesses() proc.Lister { return excludeSelf{proc.System()} }

type excludeSelf struct{ proc.Lister }

func (e excludeSelf) FindByName(name string) ([]proc.Process, error) {
	procs, err := e.Lister.FindByName(name)
	self := os.Getpid()
	return slices.DeleteFunc(slices.Clone(procs), func(p proc.Process) bool { return p.PID == self }), err
}

func (l *loop) startWatch() error {
	cfg := l.s.cfg
	if len(cfg.WatchPIDs) == 0 && cfg.WatchProcess == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(l.ctx)
	w := proc.NewWatcher(l.clk, WatchInterval, l.s.deps.Processes)
	exits, err := w.Watch(ctx, cfg.WatchPIDs, cfg.WatchProcess)
	if err != nil {
		cancel()
		return err
	}
	l.watchExit, l.cancelWatch = exits, cancel
	return nil
}

func (l *loop) exitMessage(ex proc.Exit) string {
	if cfg := l.s.cfg; len(cfg.WatchPIDs) == 0 && cfg.WatchProcess != "" {
		return cfg.WatchProcess + " exited"
	}
	return ex.Reason
}

// watchDescription is Snapshot.Watching: "make", "zoom", "pid 42",
// "zoom, pids 1, 2".
func watchDescription(command string, pids []int, name string) string {
	var parts []string
	if command != "" {
		parts = append(parts, command)
	}
	if name != "" {
		parts = append(parts, name)
	}
	pids = slices.Compact(slices.Sorted(slices.Values(pids)))
	if len(pids) > 0 {
		s := make([]string, len(pids))
		for i, pid := range pids {
			s[i] = strconv.Itoa(pid)
		}
		label := "pid "
		if len(pids) > 1 {
			label = "pids "
		}
		parts = append(parts, label+strings.Join(s, ", "))
	}
	return strings.Join(parts, ", ")
}

// ---- notifications ----

// Notification kinds; each is rate-limited separately.
const (
	notifyStopped  = "stopped"
	notifyActivity = "activity"
	notifyPower    = "power"
)

// notify shows a desktop notification in the background, at most once per
// kind every NotifyInterval. Failures are only logged; Run waits for
// pending notifications before it returns.
func (l *loop) notify(kind, title, body string) {
	n := l.s.deps.Notifier
	if n == nil {
		return
	}
	now := l.clk.Now()
	if last, ok := l.notified[kind]; ok && now.Sub(last) < NotifyInterval {
		return
	}
	l.notified[kind] = now
	l.notifying.Add(1)
	go func() {
		defer l.notifying.Done()
		if err := n.Notify(context.Background(), title, body); err != nil {
			slog.Debug("session: notification failed", "kind", kind, "err", err)
		}
	}()
}

func (l *loop) notifyDegraded(st activity.Status) {
	body := st.Reason
	if st.Hint != "" {
		body += ". " + st.Hint
	}
	l.notify(notifyActivity, "Keep-Alive: activity simulation not working", body)
}

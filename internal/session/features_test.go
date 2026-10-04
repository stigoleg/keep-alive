package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/proc"
	"github.com/stigoleg/keep-alive/v2/internal/schedule"
)

func mustSchedule(t *testing.T, spec string) *schedule.Schedule {
	t.Helper()
	s, err := schedule.Parse(spec)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (h *harness) assertCounts(acquires, releases int32) {
	h.t.Helper()
	if a, r := h.power.acquires.Load(), h.power.releases.Load(); a != acquires || r != releases {
		h.t.Fatalf("power acquires=%d releases=%d, want %d/%d", a, r, acquires, releases)
	}
}

func (h *harness) noSimulator() {
	h.t.Helper()
	select {
	case <-h.sim.started:
		h.t.Fatal("simulator started outside the work hours")
	case <-time.After(20 * time.Millisecond):
	}
}

func exited(t *testing.T, r *simRun) {
	t.Helper()
	select {
	case <-r.exited:
	case <-time.After(waitTimeout):
		t.Fatal("simulator still running")
	}
}

// t0 is Sunday 2026-03-01 10:00 UTC.

func TestScheduleStartsOutsideWindow(t *testing.T) {
	h := newHarness(t, Config{Active: true, KeepDisplay: true, Schedule: mustSchedule(t, "daily 12:00-14:00")})
	h.start()
	snap := h.seen[0].Snapshot
	if !snap.Running || snap.InWindow || snap.PowerHold != "" || snap.Schedule != "daily 12:00-14:00" {
		t.Fatalf("started snapshot = %+v", snap)
	}
	if !snap.NextChange.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("NextChange = %v", snap.NextChange)
	}
	h.assertCounts(0, 0)
	h.noSimulator()

	h.clk.Advance(2 * time.Hour)
	ev := h.waitFor(EventSchedule)
	if ev.Message != "work hours started (until 14:00)" {
		t.Fatalf("enter message = %q", ev.Message)
	}
	if s := ev.Snapshot; !s.InWindow || s.PowerHold != "fake hold" || !s.NextChange.Equal(t0.Add(4*time.Hour)) {
		t.Fatalf("snapshot after enter = %+v", s)
	}
	h.assertCounts(1, 0)
	run := h.sim.next(t)

	h.clk.Advance(2 * time.Hour)
	ev = h.waitFor(EventSchedule)
	if ev.Message != "outside work hours until Mon 12:00" {
		t.Fatalf("leave message = %q", ev.Message)
	}
	if s := ev.Snapshot; s.InWindow || s.PowerHold != "" || s.Activity.State != activity.StateOff || !s.Active {
		t.Fatalf("snapshot after leave = %+v", s)
	}
	exited(t, run)
	h.assertCounts(1, 1)

	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	h.assertCounts(1, 1)
}

func TestScheduleStartsInsideWindow(t *testing.T) {
	h := newHarness(t, Config{Active: true, Schedule: mustSchedule(t, "daily 08:00-11:00")})
	h.start()
	if s := h.seen[0].Snapshot; !s.InWindow || s.PowerHold != "fake hold" || !s.NextChange.Equal(t0.Add(time.Hour)) {
		t.Fatalf("started snapshot = %+v", s)
	}
	run := h.sim.next(t)
	h.clk.Advance(time.Hour)
	if ev := h.waitFor(EventSchedule); ev.Message != "outside work hours until Mon 08:00" {
		t.Fatalf("leave message = %q", ev.Message)
	}
	exited(t, run)

	// Back inside the next morning: power and activity come back.
	h.clk.Advance(21 * time.Hour)
	if ev := h.waitFor(EventSchedule); ev.Message != "work hours started (until 11:00)" {
		t.Fatalf("enter message = %q", ev.Message)
	}
	h.sim.next(t)
	h.assertCounts(2, 1)
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	h.assertCounts(2, 2)
}

func TestScheduleAlwaysOnNeverTransitions(t *testing.T) {
	h := newHarness(t, Config{Schedule: mustSchedule(t, "daily 00:00-24:00")})
	h.start()
	if s := h.seen[0].Snapshot; !s.InWindow || !s.NextChange.IsZero() || s.PowerHold == "" {
		t.Fatalf("started snapshot = %+v", s)
	}
	if n := h.clk.Waiters(); n != 1 {
		t.Fatalf("armed timers = %d, want only the heartbeat", n)
	}
	h.clk.Advance(8 * 24 * time.Hour)
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	for _, ev := range h.seen {
		if ev.Type == EventSchedule {
			t.Fatalf("schedule event from an always-on schedule: %+v", ev)
		}
	}
	h.assertCounts(1, 1)
}

func TestScheduleAcrossDSTChange(t *testing.T) {
	oslo, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	// Saturday before the spring-forward night (02:00 -> 03:00 on Sunday).
	start := time.Date(2026, 3, 28, 17, 0, 0, 0, oslo)
	h := newHarness(t, Config{Schedule: mustSchedule(t, "daily 08:00-16:00")})
	h.clk.Set(start)
	h.start()
	want := time.Date(2026, 3, 29, 8, 0, 0, 0, oslo)
	if s := h.seen[0].Snapshot; s.InWindow || !s.NextChange.Equal(want) {
		t.Fatalf("started snapshot = %+v", s)
	}
	if real := want.Sub(start); real != 14*time.Hour {
		t.Fatalf("the night is %v long, want 14h", real)
	}
	h.clk.Advance(14*time.Hour - time.Second)
	h.assertCounts(0, 0)
	h.clk.Advance(time.Second)
	ev := h.waitFor(EventSchedule)
	if ev.Message != "work hours started (until 16:00)" || !ev.Time.Equal(want) {
		t.Fatalf("enter event at %v: %q", ev.Time, ev.Message)
	}
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
}

func TestScheduleHeartbeatCatchesMissedTransition(t *testing.T) {
	h := newHarness(t, Config{Schedule: mustSchedule(t, "daily 09:00-17:00")})
	h.start()
	if !h.seen[0].Snapshot.InWindow {
		t.Fatal("not in the window at 10:00")
	}
	// The transition timer is lost (as across sleep or a wall-clock jump);
	// the heartbeat notices that the window has ended.
	h.s.do(func(l *loop) { l.schedTimer.Stop() })
	h.clk.Advance(7*time.Hour - time.Second)
	h.assertCounts(1, 0)
	h.clk.Advance(time.Second) // the heartbeat at 17:00
	ev := h.waitFor(EventSchedule)
	if ev.Snapshot.InWindow || !ev.Snapshot.NextChange.Equal(t0.Add(23*time.Hour)) {
		t.Fatalf("after the missed end: %+v", ev.Snapshot)
	}
	if !ev.Time.Equal(t0.Add(7 * time.Hour)) {
		t.Fatalf("noticed at %v", ev.Time)
	}
	h.assertCounts(1, 1)
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
}

func TestSetActiveOutsideWindowWaitsForIt(t *testing.T) {
	h := newHarness(t, Config{Schedule: mustSchedule(t, "daily 11:00-12:00")})
	h.start()
	h.s.SetActive(true)
	if s := h.s.Snapshot(); !s.Active || s.Activity.State != activity.StateOff {
		t.Fatalf("snapshot = %+v", s)
	}
	h.noSimulator()
	h.clk.Advance(time.Hour)
	h.waitFor(EventSchedule)
	h.sim.next(t)
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
}

func TestDurationBoundsAScheduledSession(t *testing.T) {
	h := newHarness(t, Config{Duration: 3 * time.Hour, Schedule: mustSchedule(t, "daily 11:00-12:00")})
	h.start()
	for range 2 { // enter and leave the window
		h.clk.Advance(time.Hour)
		h.waitFor(EventSchedule)
	}
	h.clk.Advance(time.Hour)
	h.finish(ReasonDuration)
	h.assertCounts(1, 1)
}

func TestPowerFailureOnEnterIsRetried(t *testing.T) {
	h := newHarness(t, Config{Schedule: mustSchedule(t, "daily 11:00-12:00")})
	h.start()
	h.power.acquireErr = errors.New("bus down")
	h.clk.Advance(time.Hour)
	ev := h.waitFor(EventWarning)
	if !strings.Contains(ev.Message, "bus down") {
		t.Fatalf("warning = %q", ev.Message)
	}
	h.power.acquireErr = nil
	if !h.clk.WaitForWaiters(3, waitTimeout) { // heartbeat, schedule, retry
		t.Fatal("retry not armed")
	}
	h.clk.Advance(time.Second)
	if ev := h.waitFor(EventWarning); !strings.Contains(ev.Message, "re-acquired") || ev.Snapshot.PowerHold != "fake hold" {
		t.Fatalf("recovery = %q (%q)", ev.Message, ev.Snapshot.PowerHold)
	}
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	h.assertCounts(1, 1)
}

// ---- process watch ----

type fakeLister struct {
	mu    sync.Mutex
	alive map[int]bool
	named map[string][]proc.Process
}

func (f *fakeLister) Alive(pid int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[pid], nil
}

func (f *fakeLister) FindByName(name string) ([]proc.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.named[name], nil
}

func (f *fakeLister) set(fn func(*fakeLister)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func newWatchHarness(t *testing.T, cfg Config, l *fakeLister) *harness {
	t.Helper()
	h := newHarness(t, cfg)
	h.s = New(cfg, Deps{Clock: h.clk, Power: h.power, Activity: h.sim, Processes: l})
	var unsub func()
	h.events, unsub = h.s.Subscribe()
	t.Cleanup(unsub)
	return h
}

func TestWatchedPIDExitStops(t *testing.T) {
	l := &fakeLister{alive: map[int]bool{42: true}}
	h := newWatchHarness(t, Config{WatchPIDs: []int{42}}, l)
	h.start()
	if w := h.seen[0].Snapshot.Watching; w != "pid 42" {
		t.Fatalf("Watching = %q", w)
	}
	l.set(func(f *fakeLister) { f.alive[42] = false })
	if !h.clk.WaitForWaiters(2, waitTimeout) {
		t.Fatal("watch ticker not armed")
	}
	h.clk.Advance(WatchInterval)
	h.finish(ReasonProcessExited)
	if msg := h.seen[len(h.seen)-1].Message; msg != "process 42 exited" {
		t.Fatalf("stop message = %q", msg)
	}
	h.assertReleasedOnce()
}

func TestWatchedNameExitStops(t *testing.T) {
	l := &fakeLister{named: map[string][]proc.Process{"zoom": {{PID: 7, Name: "zoom.us"}}}}
	h := newWatchHarness(t, Config{WatchProcess: "zoom"}, l)
	h.start()
	if w := h.seen[0].Snapshot.Watching; w != "zoom" {
		t.Fatalf("Watching = %q", w)
	}
	l.set(func(f *fakeLister) { f.named["zoom"] = nil })
	if !h.clk.WaitForWaiters(2, waitTimeout) {
		t.Fatal("watch ticker not armed")
	}
	h.clk.Advance(WatchInterval)
	h.finish(ReasonProcessExited)
	if msg := h.seen[len(h.seen)-1].Message; msg != "zoom exited" {
		t.Fatalf("stop message = %q", msg)
	}
}

func TestWatchDescriptions(t *testing.T) {
	for _, tt := range []struct {
		command string
		pids    []int
		name    string
		want    string
	}{
		{"", []int{42}, "", "pid 42"},
		{"", []int{11, 10}, "", "pids 10, 11"},
		{"", nil, "zoom", "zoom"},
		{"", []int{42}, "zoom", "zoom, pid 42"},
		{"make", nil, "", "make"},
	} {
		if got := watchDescription(tt.command, tt.pids, tt.name); got != tt.want {
			t.Errorf("watchDescription(%v, %q) = %q, want %q", tt.pids, tt.name, got, tt.want)
		}
	}
}

func TestWatchStartErrorAcquiresNothing(t *testing.T) {
	h := newWatchHarness(t, Config{WatchProcess: "zoom"}, &fakeLister{})
	h.run()
	r := h.finish(ReasonError)
	if !errors.Is(r.Err, proc.ErrNotRunning) {
		t.Fatalf("Result.Err = %v, want ErrNotRunning", r.Err)
	}
	h.assertCounts(0, 0)
}

func TestSystemListerSkipsOwnProcess(t *testing.T) {
	own := proc.Process{PID: os.Getpid(), Name: "keepalive"}
	other := proc.Process{PID: os.Getpid() + 1, Name: "keepalive"}
	l := excludeSelf{&fakeLister{named: map[string][]proc.Process{"keepalive": {own, other}}}}
	got, err := l.FindByName("keepalive")
	if err != nil || len(got) != 1 || got[0] != other {
		t.Fatalf("FindByName = %v, %v", got, err)
	}
}

func TestCommandExitedMessage(t *testing.T) {
	h := newHarness(t, Config{})
	h.start()
	h.s.Stop(ReasonCommandExited)
	h.finish(ReasonCommandExited)
	if msg := h.seen[len(h.seen)-1].Message; msg != "command exited" {
		t.Fatalf("stop message = %q", msg)
	}
}

// ---- notifications ----

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string
	err  error
}

func (n *fakeNotifier) Notify(_ context.Context, title, body string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, title+" | "+body)
	return n.err
}

func (n *fakeNotifier) list() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.sent...)
}

func (n *fakeNotifier) waitFor(t *testing.T, count int) []string {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for {
		if got := n.list(); len(got) >= count {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("notifications = %v, want %d", n.list(), count)
		}
		time.Sleep(time.Millisecond)
	}
}

func newNotifyHarness(t *testing.T, cfg Config) (*harness, *fakeNotifier) {
	t.Helper()
	h := newHarness(t, cfg)
	n := &fakeNotifier{}
	h.s = New(cfg, Deps{Clock: h.clk, Power: h.power, Activity: h.sim, Battery: h.batt.status, Notifier: n})
	var unsub func()
	h.events, unsub = h.s.Subscribe()
	t.Cleanup(unsub)
	return h, n
}

func TestNotifyOnUnusualStop(t *testing.T) {
	h, n := newNotifyHarness(t, Config{BatteryThreshold: 20})
	h.batt.set(15, nil)
	h.run()
	h.finish(ReasonBattery)
	// Run returns only after the notification was handed over.
	if got := n.list(); len(got) != 1 || got[0] != "Keep-Alive stopped | battery at 15% (threshold 20%)" {
		t.Fatalf("notifications = %v", got)
	}
}

func TestNoNotifyOnNormalStops(t *testing.T) {
	for _, reason := range []Reason{ReasonUser, ReasonSignal, ReasonIPC, ReasonCommandExited} {
		h, n := newNotifyHarness(t, Config{})
		h.start()
		h.s.Stop(reason)
		h.finish(reason)
		if got := n.list(); len(got) != 0 {
			t.Fatalf("%s: notifications = %v", reason, got)
		}
	}
}

func TestNotifyDegradedOncePerInterval(t *testing.T) {
	h, n := newNotifyHarness(t, Config{Active: true})
	n.err = errors.New("no notification server") // never shown to the user
	h.start()
	run := h.sim.next(t)
	degraded := activity.Status{State: activity.StateDegraded, Reason: "Accessibility permission is missing", Hint: "grant it"}
	run.report(degraded)
	h.waitFor(EventActivity)
	got := n.waitFor(t, 1)
	if got[0] != "Keep-Alive: activity simulation not working | Accessibility permission is missing. grant it" {
		t.Fatalf("notification = %q", got[0])
	}

	// Recovers and degrades again within ten minutes: no second one.
	run.report(activity.Status{State: activity.StateSimulating, Method: "fake"})
	h.waitFor(EventActivity)
	h.clk.Advance(5 * time.Minute)
	run.report(degraded)
	h.waitFor(EventActivity)

	// After ten minutes it may notify again.
	run.report(activity.Status{State: activity.StateSimulating, Method: "fake"})
	h.waitFor(EventActivity)
	h.clk.Advance(5 * time.Minute)
	run.report(degraded)
	h.waitFor(EventActivity)
	n.waitFor(t, 2)

	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	if got := n.list(); len(got) != 2 {
		t.Fatalf("notifications = %v", got)
	}
	for _, ev := range h.seen {
		if strings.Contains(ev.Message, "notification server") {
			t.Fatalf("notify error reached the event stream: %+v", ev)
		}
	}
}

func TestNotifyPowerLostAfterFirstRetry(t *testing.T) {
	for _, recoverAtFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("recovered=%v", recoverAtFirst), func(t *testing.T) {
			h, p := newLossHarness(t)
			n := &fakeNotifier{}
			h.s = New(Config{}, Deps{Clock: h.clk, Power: p, Notifier: n})
			var unsub func()
			h.events, unsub = h.s.Subscribe()
			t.Cleanup(unsub)
			h.start()
			p.waitAttempt(t)
			if !recoverAtFirst {
				p.setFailures(1)
			}
			p.hold(0).lost <- errors.New("bus gone")
			h.waitFor(EventWarning)
			advanceToRetry(t, h, time.Second)
			p.waitAttempt(t)
			if recoverAtFirst {
				h.waitFor(EventWarning) // re-acquired
			} else {
				got := n.waitFor(t, 1)
				if !strings.HasPrefix(got[0], "Keep-Alive: cannot keep the system awake | ") || !strings.Contains(got[0], "bus down") {
					t.Fatalf("notification = %q", got[0])
				}
			}
			h.s.Stop(ReasonUser)
			h.finish(ReasonUser)
			want := 1
			if recoverAtFirst {
				want = 0
			}
			if got := n.list(); len(got) != want {
				t.Fatalf("notifications = %v, want %d", got, want)
			}
		})
	}
}

var _ clock.Clock = (*clock.Fake)(nil)

type fakeLimiter struct {
	mu    sync.Mutex
	asked []string
	allow bool
}

func (f *fakeLimiter) Allow(kind string, _ time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, kind)
	return f.allow
}

func TestNotifyLimiterDecides(t *testing.T) {
	for _, allow := range []bool{false, true} {
		h := newHarness(t, Config{BatteryThreshold: 20})
		n, lim := &fakeNotifier{}, &fakeLimiter{allow: allow}
		h.s = New(Config{BatteryThreshold: 20}, Deps{Clock: h.clk, Power: h.power, Battery: h.batt.status, Notifier: n, NotifyLimiter: lim})
		var unsub func()
		h.events, unsub = h.s.Subscribe()
		t.Cleanup(unsub)
		h.batt.set(15, nil)
		h.run()
		h.finish(ReasonBattery)
		if len(lim.asked) != 1 || lim.asked[0] != notifyStopped {
			t.Fatalf("limiter asked %v", lim.asked)
		}
		if got := len(n.list()); got != map[bool]int{false: 0, true: 1}[allow] {
			t.Fatalf("allow=%v: %d notifications", allow, got)
		}
	}
}

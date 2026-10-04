package session

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
)

var t0 = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

const waitTimeout = 3 * time.Second

// ---- fakes ----

type fakePower struct {
	acquires, releases atomic.Int32
	gate               chan struct{} // when set, Acquire blocks until closed
	acquireErr         error
	releaseErr         error
	lastOpts           atomic.Value
}

func (p *fakePower) Name() string { return "fake" }

func (p *fakePower) Acquire(ctx context.Context, o power.Options) (power.Hold, error) {
	p.lastOpts.Store(o)
	if p.gate != nil {
		<-p.gate
	}
	if p.acquireErr != nil {
		return nil, p.acquireErr
	}
	p.acquires.Add(1)
	return &fakeHold{p: p}, nil
}

type fakeHold struct{ p *fakePower }

func (h *fakeHold) Release() error {
	h.p.releases.Add(1)
	return h.p.releaseErr
}

func (h *fakeHold) Describe() string { return "fake hold" }

type simRun struct {
	ctx    context.Context
	cfg    activity.Config
	report func(activity.Status)
	exited chan struct{}
}

type fakeSim struct {
	mu   sync.Mutex
	runs []*simRun
	err  error // returned when Run exits on its own via exitNow
	// exitNow, when set, makes Run return immediately with err.
	exitNow bool
	started chan *simRun
}

func newFakeSim() *fakeSim { return &fakeSim{started: make(chan *simRun, 16)} }

func (f *fakeSim) Run(ctx context.Context, cfg activity.Config, report func(activity.Status)) error {
	r := &simRun{ctx: ctx, cfg: cfg, report: report, exited: make(chan struct{})}
	defer close(r.exited)
	f.mu.Lock()
	f.runs = append(f.runs, r)
	exitNow, err := f.exitNow, f.err
	f.mu.Unlock()
	f.started <- r
	if exitNow {
		return err
	}
	<-ctx.Done()
	return nil
}

func (f *fakeSim) next(t *testing.T) *simRun {
	t.Helper()
	select {
	case r := <-f.started:
		return r
	case <-time.After(waitTimeout):
		t.Fatal("simulator was not started")
		return nil
	}
}

type fakeBattery struct {
	mu   sync.Mutex
	pct  int
	err  error
	read atomic.Int32
}

func (b *fakeBattery) set(pct int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pct, b.err = pct, err
}

func (b *fakeBattery) status() (platform.BatteryStatus, error) {
	b.read.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return platform.BatteryStatus{}, b.err
	}
	return platform.BatteryStatus{Percentage: b.pct, Available: true}, nil
}

// ---- harness ----

type harness struct {
	t      *testing.T
	clk    *clock.Fake
	power  *fakePower
	sim    *fakeSim
	batt   *fakeBattery
	s      *Session
	events <-chan Event
	seen   []Event
	result chan Result
	cancel context.CancelFunc
}

func newHarness(t *testing.T, cfg Config) *harness {
	t.Helper()
	h := &harness{
		t:      t,
		clk:    clock.NewFake(t0),
		power:  &fakePower{},
		sim:    newFakeSim(),
		batt:   &fakeBattery{pct: 80},
		result: make(chan Result, 1),
	}
	h.s = New(cfg, Deps{Clock: h.clk, Power: h.power, Activity: h.sim, Battery: h.batt.status})
	var unsub func()
	h.events, unsub = h.s.Subscribe()
	t.Cleanup(unsub)
	return h
}

func (h *harness) run() {
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.t.Cleanup(cancel)
	go func() { h.result <- h.s.Run(ctx) }()
}

func (h *harness) start() {
	h.t.Helper()
	h.run()
	h.waitFor(EventStarted)
}

// waitFor returns the next event of type typ, recording everything it skips.
func (h *harness) waitFor(typ EventType) Event {
	h.t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case ev, ok := <-h.events:
			if !ok {
				h.t.Fatalf("event stream closed while waiting for %s (seen %v)", typ, h.types())
			}
			h.seen = append(h.seen, ev)
			if ev.Type == typ {
				return ev
			}
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s (seen %v)", typ, h.types())
		}
	}
}

func (h *harness) waitResult() Result {
	h.t.Helper()
	select {
	case r := <-h.result:
		return r
	case <-time.After(waitTimeout):
		h.t.Fatal("Run did not return")
		return Result{}
	}
}

// drain reads the rest of the stream until it closes.
func (h *harness) drain() {
	h.t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case ev, ok := <-h.events:
			if !ok {
				return
			}
			h.seen = append(h.seen, ev)
		case <-deadline:
			h.t.Fatal("event stream not closed")
		}
	}
}

func (h *harness) types() []EventType {
	out := make([]EventType, len(h.seen))
	for i, ev := range h.seen {
		out[i] = ev.Type
	}
	return out
}

func (h *harness) finish(want Reason) Result {
	h.t.Helper()
	r := h.waitResult()
	h.drain()
	if r.Reason != want {
		h.t.Fatalf("Result.Reason = %q, want %q (err %v)", r.Reason, want, r.Err)
	}
	if n := len(h.seen); n == 0 || h.seen[n-1].Type != EventStopped {
		h.t.Fatalf("last event is not stopped: %v", h.types())
	}
	if got := h.seen[len(h.seen)-1].Reason; got != want {
		h.t.Fatalf("stopped event reason = %q, want %q", got, want)
	}
	return r
}

func (h *harness) assertReleasedOnce() {
	h.t.Helper()
	if a, r := h.power.acquires.Load(), h.power.releases.Load(); a != 1 || r != 1 {
		h.t.Fatalf("power acquires=%d releases=%d, want 1/1", a, r)
	}
}

// ---- stop reasons ----

func TestDurationEndsSession(t *testing.T) {
	h := newHarness(t, Config{Duration: 2 * time.Minute, KeepDisplay: true})
	h.start()

	snap := h.seen[len(h.seen)-1].Snapshot
	if !snap.Running || snap.Mode != ModeDuration || !snap.EndsAt.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("started snapshot = %+v", snap)
	}
	if snap.Remaining != 2*time.Minute || snap.PowerHold != "fake hold" || !snap.KeepDisplay {
		t.Fatalf("started snapshot = %+v", snap)
	}
	if o := h.power.lastOpts.Load().(power.Options); !o.KeepDisplay {
		t.Fatal("KeepDisplay not passed to the inhibitor")
	}

	h.clk.Advance(119 * time.Second)
	select {
	case r := <-h.result:
		t.Fatalf("stopped early: %+v", r)
	case <-time.After(20 * time.Millisecond):
	}
	h.clk.Advance(time.Second)
	r := h.finish(ReasonDuration)
	h.assertReleasedOnce()
	if !r.StartedAt.Equal(t0) || !r.EndedAt.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("result times = %v..%v", r.StartedAt, r.EndedAt)
	}
	stopping := false
	for _, ev := range h.seen {
		if ev.Type == EventStopping {
			stopping = true
		}
	}
	if !stopping {
		t.Fatalf("no stopping event: %v", h.types())
	}
	if final := h.seen[len(h.seen)-1]; final.Snapshot.Running || final.Message == "" {
		t.Fatalf("stopped event = %+v", final)
	}
}

func TestUntilEndsSession(t *testing.T) {
	until := t0.Add(10 * time.Minute)
	h := newHarness(t, Config{Until: until})
	h.start()
	if snap := h.seen[0].Snapshot; snap.Mode != ModeUntil || !snap.EndsAt.Equal(until) {
		t.Fatalf("snapshot = %+v", snap)
	}
	h.clk.Advance(10 * time.Minute)
	h.finish(ReasonUntil)
	h.assertReleasedOnce()
}

func TestUntilInThePastStopsImmediately(t *testing.T) {
	h := newHarness(t, Config{Until: t0.Add(-time.Minute)})
	h.run()
	h.finish(ReasonUntil)
	h.assertReleasedOnce()
}

func TestBatteryCheckedImmediately(t *testing.T) {
	h := newHarness(t, Config{BatteryThreshold: 20})
	h.batt.set(15, nil)
	h.run()
	h.waitFor(EventStarted)
	ev := h.waitFor(EventBattery)
	if ev.Snapshot.Battery != (Battery{Percent: 15, Available: true, Threshold: 20}) {
		t.Fatalf("battery = %+v", ev.Snapshot.Battery)
	}
	h.finish(ReasonBattery)
	h.assertReleasedOnce()
}

func TestBatteryPolledUntilThreshold(t *testing.T) {
	h := newHarness(t, Config{BatteryThreshold: 20})
	h.start()
	if ev := h.waitFor(EventBattery); ev.Snapshot.Battery.Percent != 80 {
		t.Fatalf("battery = %+v", ev.Snapshot.Battery)
	}
	h.batt.set(20, nil)
	h.clk.Advance(BatteryPollInterval)
	h.finish(ReasonBattery)
	h.assertReleasedOnce()
	if n := h.batt.read.Load(); n != 2 {
		t.Fatalf("battery reads = %d, want 2", n)
	}
}

func TestBatteryErrorWarnsAndKeepsRunning(t *testing.T) {
	h := newHarness(t, Config{BatteryThreshold: 20})
	h.batt.set(0, errors.New("no battery"))
	h.start()
	if ev := h.waitFor(EventBattery); ev.Snapshot.Battery.Available {
		t.Fatalf("battery = %+v", ev.Snapshot.Battery)
	}
	h.waitFor(EventWarning)
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	h.assertReleasedOnce()
}

func TestNoBatteryPollingWithoutThreshold(t *testing.T) {
	h := newHarness(t, Config{})
	h.start()
	h.clk.Advance(5 * time.Minute)
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	if n := h.batt.read.Load(); n != 0 {
		t.Fatalf("battery reads = %d, want 0", n)
	}
}

func TestContextCancelIsSignal(t *testing.T) {
	h := newHarness(t, Config{})
	h.start()
	h.cancel()
	h.finish(ReasonSignal)
	h.assertReleasedOnce()
}

func TestUserStopAndDoubleStop(t *testing.T) {
	h := newHarness(t, Config{Active: true})
	h.start()
	run := h.sim.next(t)
	h.s.Stop(ReasonUser)
	h.s.Stop(ReasonBattery)
	h.finish(ReasonUser)
	h.assertReleasedOnce()
	select {
	case <-run.exited:
	default:
		t.Fatal("simulator still running after stop")
	}
	h.s.Stop(ReasonUser) // after Run returned: must not block or panic
}

func TestStopBeforeRun(t *testing.T) {
	h := newHarness(t, Config{})
	h.s.Stop(ReasonUser)
	h.run()
	h.finish(ReasonUser)
	if a := h.power.acquires.Load(); a != 0 {
		t.Fatalf("acquired power %d times for a session stopped before Run", a)
	}
}

func TestStopDuringStart(t *testing.T) {
	h := newHarness(t, Config{})
	h.power.gate = make(chan struct{})
	h.run()
	// Wait until Run is inside Acquire, then stop.
	deadline := time.Now().Add(waitTimeout)
	for h.power.lastOpts.Load() == nil {
		if time.Now().After(deadline) {
			t.Fatal("Acquire not called")
		}
		time.Sleep(time.Millisecond)
	}
	h.s.Stop(ReasonUser)
	close(h.power.gate)
	h.finish(ReasonUser)
	h.assertReleasedOnce()
}

func TestAcquireErrorEndsWithError(t *testing.T) {
	h := newHarness(t, Config{})
	h.power.acquireErr = errors.New("denied")
	h.run()
	r := h.finish(ReasonError)
	if r.Err == nil {
		t.Fatal("Result.Err is nil")
	}
	if a, rel := h.power.acquires.Load(), h.power.releases.Load(); a != 0 || rel != 0 {
		t.Fatalf("acquires=%d releases=%d, want 0/0", a, rel)
	}
}

func TestReleaseErrorIsReported(t *testing.T) {
	h := newHarness(t, Config{})
	h.power.releaseErr = errors.New("stuck")
	h.start()
	h.s.Stop(ReasonUser)
	r := h.finish(ReasonUser)
	if r.Err == nil {
		t.Fatal("release error not reported in Result.Err")
	}
	h.assertReleasedOnce()
}

func TestInvalidConfig(t *testing.T) {
	h := newHarness(t, Config{Duration: time.Minute, Until: t0.Add(time.Hour)})
	h.run()
	if r := h.finish(ReasonError); r.Err == nil {
		t.Fatal("Result.Err is nil")
	}
	if a := h.power.acquires.Load(); a != 0 {
		t.Fatal("acquired power for an invalid config")
	}
}

func TestRunTwice(t *testing.T) {
	h := newHarness(t, Config{})
	h.start()
	if r := h.s.Run(context.Background()); r.Reason != ReasonError || r.Err == nil {
		t.Fatalf("second Run = %+v, want error", r)
	}
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	h.assertReleasedOnce()
}

// ---- runtime controls ----

func TestExtend(t *testing.T) {
	h := newHarness(t, Config{Duration: 10 * time.Minute})
	h.start()

	h.s.Extend(5 * time.Minute)
	if snap := h.s.Snapshot(); !snap.EndsAt.Equal(t0.Add(15 * time.Minute)) {
		t.Fatalf("EndsAt after extend = %v", snap.EndsAt)
	}
	h.waitFor(EventSnapshot)
	h.clk.Advance(10 * time.Minute)
	if snap := h.s.Snapshot(); !snap.Running || snap.Remaining != 5*time.Minute {
		t.Fatalf("snapshot = %+v", snap)
	}
	h.clk.Advance(5 * time.Minute)
	h.finish(ReasonDuration)
	h.assertReleasedOnce()
}

func TestExtendNegativeClampsToOneMinute(t *testing.T) {
	h := newHarness(t, Config{Duration: 10 * time.Minute})
	h.start()
	h.s.Extend(-30 * time.Minute)
	if snap := h.s.Snapshot(); !snap.EndsAt.Equal(t0.Add(MinRemainingAfterShorten)) {
		t.Fatalf("EndsAt = %v, want now+1m", snap.EndsAt)
	}

	// With under a minute left, shortening must not lengthen the session.
	h.clk.Advance(30 * time.Second)
	h.s.Extend(-time.Minute)
	if snap := h.s.Snapshot(); !snap.EndsAt.Equal(t0.Add(MinRemainingAfterShorten)) {
		t.Fatalf("EndsAt = %v, want unchanged", snap.EndsAt)
	}
	h.clk.Advance(30 * time.Second)
	h.finish(ReasonDuration)
}

func TestExtendIndefiniteWarns(t *testing.T) {
	h := newHarness(t, Config{})
	h.start()
	h.s.Extend(time.Hour)
	h.waitFor(EventWarning)
	if snap := h.s.Snapshot(); !snap.EndsAt.IsZero() {
		t.Fatalf("EndsAt = %v, want zero", snap.EndsAt)
	}
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
}

func TestSetActiveToggling(t *testing.T) {
	h := newHarness(t, Config{Activity: activity.Config{IdleThreshold: time.Minute, Interval: 10 * time.Second}})
	h.start()
	if h.seen[0].Snapshot.Active {
		t.Fatal("started active")
	}

	h.s.SetActive(true)
	run1 := h.sim.next(t)
	if run1.cfg.IdleThreshold != time.Minute || run1.cfg.Interval != 10*time.Second {
		t.Fatalf("simulator config = %+v", run1.cfg)
	}
	run1.report(activity.Status{State: activity.StateWaitingIdle, Method: "fake"})
	ev := h.waitFor(EventActivity)
	if !ev.Snapshot.Active || ev.Snapshot.Activity.State != activity.StateWaitingIdle {
		t.Fatalf("activity event snapshot = %+v", ev.Snapshot)
	}

	// Only the idle reading changed: no new event.
	run1.report(activity.Status{State: activity.StateWaitingIdle, Method: "fake", Idle: time.Second})
	run1.report(activity.Status{State: activity.StateSimulating, Method: "fake", LastBurst: t0})
	if ev := h.waitFor(EventActivity); ev.Snapshot.Activity.State != activity.StateSimulating {
		t.Fatalf("expected simulating, got %+v", ev.Snapshot.Activity)
	}

	h.s.SetActive(false)
	select {
	case <-run1.exited:
	case <-time.After(waitTimeout):
		t.Fatal("simulator not cancelled by SetActive(false)")
	}
	if ev := h.waitFor(EventActivity); ev.Snapshot.Active || ev.Snapshot.Activity.State != activity.StateOff {
		t.Fatalf("after off: %+v", ev.Snapshot)
	}

	h.s.SetActive(true)
	run2 := h.sim.next(t)
	run1.report(activity.Status{State: activity.StateDegraded}) // stale run: ignored
	run2.report(activity.Status{State: activity.StatePausedUser, Method: "fake"})
	if ev := h.waitFor(EventActivity); ev.Snapshot.Activity.State != activity.StatePausedUser {
		t.Fatalf("expected paused_user from run 2, got %+v", ev.Snapshot.Activity)
	}

	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	h.assertReleasedOnce()
	select {
	case <-run2.exited:
	default:
		t.Fatal("second simulator still running after stop")
	}
}

func TestSimulatorFailureDegrades(t *testing.T) {
	h := newHarness(t, Config{Active: true})
	h.sim.exitNow = true
	h.sim.err = errors.New("no input backend")
	h.start()
	h.waitFor(EventWarning)
	if snap := h.s.Snapshot(); snap.Activity.State != activity.StateDegraded {
		t.Fatalf("activity = %+v", snap.Activity)
	}
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	h.assertReleasedOnce()
}

func TestActiveWithoutSimulatorWarns(t *testing.T) {
	clk := clock.NewFake(t0)
	p := &fakePower{}
	s := New(Config{Active: true}, Deps{Clock: clk, Power: p})
	events, unsub := s.Subscribe()
	defer unsub()
	done := make(chan Result, 1)
	go func() { done <- s.Run(context.Background()) }()
	for ev := range events {
		if ev.Type == EventWarning {
			s.Stop(ReasonUser)
		}
	}
	if r := <-done; r.Reason != ReasonUser {
		t.Fatalf("reason = %s", r.Reason)
	}
}

func TestHeartbeat(t *testing.T) {
	h := newHarness(t, Config{Duration: time.Hour})
	h.start()
	h.clk.Advance(HeartbeatInterval)
	ev := h.waitFor(EventSnapshot)
	if ev.Snapshot.Remaining != time.Hour-HeartbeatInterval {
		t.Fatalf("heartbeat remaining = %v", ev.Snapshot.Remaining)
	}
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
}

func TestSnapshotOutsideRun(t *testing.T) {
	h := newHarness(t, Config{Duration: time.Hour, BatteryThreshold: 10, KeepDisplay: true})
	snap := h.s.Snapshot()
	if snap.Running || snap.Mode != ModeDuration || snap.Battery.Threshold != 10 || !snap.KeepDisplay {
		t.Fatalf("pre-run snapshot = %+v", snap)
	}
	h.start()
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	if snap := h.s.Snapshot(); snap.Running {
		t.Fatalf("post-run snapshot still running: %+v", snap)
	}
	h.s.SetActive(true) // no-op after the session ended
	h.s.Extend(time.Minute)
}

func TestSlowSubscriberNeverBlocksLoop(t *testing.T) {
	h := newHarness(t, Config{})
	slow, unsub := h.s.Subscribe()
	defer unsub()
	h.start()
	for i := 0; i < subscriberBuffer*3; i++ {
		h.clk.Advance(HeartbeatInterval)
		h.waitFor(EventSnapshot)
	}
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)

	var last Event
	n := 0
	for ev := range slow {
		last = ev
		n++
	}
	if n > subscriberBuffer {
		t.Fatalf("slow subscriber got %d events, buffer is %d", n, subscriberBuffer)
	}
	if last.Type != EventStopped {
		t.Fatalf("slow subscriber's last event = %s, want stopped", last.Type)
	}
}

func TestSubscribeAfterEndIsClosed(t *testing.T) {
	h := newHarness(t, Config{})
	h.start()
	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	ch, unsub := h.s.Subscribe()
	defer unsub()
	if _, ok := <-ch; ok {
		t.Fatal("subscription after end delivered an event")
	}
}

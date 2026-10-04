package activity

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/clock"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

// machine fakes the OS: an idle counter driven by the fake clock, a lock
// flag and an injector whose input resets the counter when effective.
type machine struct {
	clk *clock.Fake

	mu        sync.Mutex
	lastInput time.Time
	idleErr   error
	locked    bool
	ignored   bool // injected input does not reset the idle counter
	playErr   error
	bursts    []time.Time
	taps      int
	closed    int
	opens     int
	openErr   error
	idleReads int
	// xwLast is the last input a second counter (XWayland) saw; xwFollows
	// makes injected input reach it.
	xwLast    time.Time
	xwFollows bool
}

func newMachine() *machine {
	clk := clock.NewFake(time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC))
	return &machine{clk: clk, lastInput: clk.Now(), xwLast: clk.Now()}
}

func (m *machine) userInput() {
	m.mu.Lock()
	m.lastInput = m.clk.Now()
	m.mu.Unlock()
}

func (m *machine) set(f func(m *machine)) {
	m.mu.Lock()
	f(m)
	m.mu.Unlock()
}

func (m *machine) burstCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bursts)
}

type fakeIdle struct{ m *machine }

func (fakeIdle) Name() string { return "fake idle" }
func (f fakeIdle) Idle() (time.Duration, error) {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	f.m.idleReads++
	if f.m.idleErr != nil {
		return 0, f.m.idleErr
	}
	return max(f.m.clk.Now().Sub(f.m.lastInput), 0), nil
}

// fakeXWayland is a second idle counter that only sees injected input when
// xwFollows is set.
type fakeXWayland struct{ m *machine }

func (fakeXWayland) Name() string { return "xprintidle (XWayland)" }
func (f fakeXWayland) Idle() (time.Duration, error) {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	return max(f.m.clk.Now().Sub(f.m.xwLast), 0), nil
}

type fakeLock struct{ m *machine }

func (f fakeLock) Locked() (bool, error) {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	return f.m.locked, nil
}

// fakeInjector is an AbsoluteInjector that replays whole bursts.
type fakeInjector struct{ m *machine }

func (fakeInjector) Name() string                       { return "Fake" }
func (fakeInjector) Available() error                   { return nil }
func (fakeInjector) Diagnose() (string, string)         { return "input ignored", "grant it" }
func (fakeInjector) MoveTo(x, y float64) error          { return nil }
func (fakeInjector) Position() (float64, float64, bool) { return 500, 500, true }
func (fakeInjector) Bounds() (Rect, bool)               { return Rect{0, 0, 1000, 1000}, true }
func (f fakeInjector) Tap() error {
	f.m.mu.Lock()
	f.m.taps++
	f.m.mu.Unlock()
	return nil
}
func (f fakeInjector) Close() error {
	f.m.mu.Lock()
	f.m.closed++
	f.m.mu.Unlock()
	return nil
}
func (f fakeInjector) Play(ctx context.Context, ox, oy float64, p Path) error {
	f.m.mu.Lock()
	defer f.m.mu.Unlock()
	if f.m.playErr != nil {
		return f.m.playErr
	}
	f.m.bursts = append(f.m.bursts, f.m.clk.Now())
	if !f.m.ignored {
		// The OS accounts for injected events a little after they are
		// posted, so the idle counter lags our own clock.
		f.m.lastInput = f.m.clk.Now().Add(100 * time.Millisecond)
		if f.m.xwFollows {
			f.m.xwLast = f.m.lastInput
		}
	}
	return nil
}

type harness struct {
	t   *testing.T
	m   *machine
	c   *controller
	ctx context.Context

	mu       sync.Mutex
	statuses []Status
}

func newHarness(t *testing.T, cfg Config, tweak func(m *machine, d *controllerDeps)) *harness {
	t.Helper()
	m := newMachine()
	h := &harness{t: t, m: m, ctx: context.Background()}
	d := controllerDeps{
		clock: m.clk,
		idle:  fakeIdle{m},
		lock:  fakeLock{m},
		open: func() (Injector, error) {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.opens++
			if m.openErr != nil {
				return nil, m.openErr
			}
			return fakeInjector{m}, nil
		},
		rnd:        seeded(1),
		sleep:      func(ctx context.Context, d time.Duration) error { return ctx.Err() },
		noIdleHint: "install an idle source",
	}
	if tweak != nil {
		tweak(m, &d)
	}
	h.c = newController(cfg, d, func(st Status) {
		h.mu.Lock()
		h.statuses = append(h.statuses, st)
		h.mu.Unlock()
	})
	h.c.init()
	return h
}

// runFor steps the controller, advancing the clock by whatever delay each
// step asks for, until d has passed.
func (h *harness) runFor(d time.Duration) {
	h.t.Helper()
	end := h.m.clk.Now().Add(d)
	for h.m.clk.Now().Before(end) {
		next := h.c.step(h.ctx)
		if next <= 0 {
			h.t.Fatalf("step asked for a non-positive delay %v", next)
		}
		if next > tickInterval {
			h.t.Fatalf("step asked for %v, more than the %v tick", next, tickInterval)
		}
		h.m.clk.Advance(min(next, end.Sub(h.m.clk.Now())))
	}
}

// userActiveFor has the user touch the mouse every two seconds.
func (h *harness) userActiveFor(d time.Duration) {
	h.t.Helper()
	for end := h.m.clk.Now().Add(d); h.m.clk.Now().Before(end); {
		h.m.userInput()
		h.runFor(2 * time.Second)
	}
}

func (h *harness) last() Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.statuses) == 0 {
		h.t.Fatal("no status reported")
	}
	return h.statuses[len(h.statuses)-1]
}

// states lists reported states with consecutive duplicates removed.
func (h *harness) states() []State {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []State
	for _, st := range h.statuses {
		if len(out) == 0 || out[len(out)-1] != st.State {
			out = append(out, st.State)
		}
	}
	return out
}

var testCfg = Config{IdleThreshold: 2 * time.Minute, Interval: 30 * time.Second}

func TestControllerWaitsForIdleThreshold(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.runFor(119 * time.Second)
	if n := h.m.burstCount(); n != 0 {
		t.Fatalf("%d bursts before the idle threshold", n)
	}
	st := h.last()
	if st.State != StateWaitingIdle {
		t.Fatalf("state = %s, want waiting_idle", st.State)
	}
	if st.Idle < 110*time.Second {
		t.Fatalf("idle progress = %v, want ~2m", st.Idle)
	}
	h.runFor(4 * time.Second)
	if n := h.m.burstCount(); n != 1 {
		t.Fatalf("%d bursts after reaching the threshold, want 1", n)
	}
	st = h.last()
	if st.State != StateSimulating || st.Method != "Fake" || st.LastBurst.IsZero() {
		t.Fatalf("after first burst: %+v", st)
	}
}

func TestControllerCadenceIsJitteredInterval(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.runFor(2*time.Minute + 2*time.Second)
	h.runFor(60 * time.Minute)
	b := h.m.bursts
	if len(b) < 80 {
		t.Fatalf("only %d bursts in an hour at a 30s interval", len(b))
	}
	gaps := map[time.Duration]bool{}
	for i := 1; i < len(b); i++ {
		gap := b[i].Sub(b[i-1])
		if gap < 19500*time.Millisecond || gap > 40500*time.Millisecond {
			t.Fatalf("gap %d = %v, want 30s ±35%%", i, gap)
		}
		gaps[gap.Round(time.Second)] = true
	}
	if len(gaps) < 5 {
		t.Fatalf("gaps are not randomised: %v", gaps)
	}
	for _, s := range h.states() {
		if s == StatePausedUser {
			t.Fatal("our own bursts were mistaken for the user returning")
		}
	}
}

func TestControllerPausesWhenUserReturns(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.runFor(3 * time.Minute)
	if h.last().State != StateSimulating {
		t.Fatalf("state = %s, want simulating", h.last().State)
	}
	h.userActiveFor(6 * time.Second)
	if h.last().State != StatePausedUser {
		t.Fatalf("state = %s after the user moved the mouse, want paused_user", h.last().State)
	}
	n := h.m.burstCount()
	// The user keeps working for five minutes.
	h.userActiveFor(5 * time.Minute)
	if got := h.m.burstCount(); got != n {
		t.Fatalf("%d bursts while the user was active", got-n)
	}
	if h.last().State != StatePausedUser {
		t.Fatalf("state = %s while the user is active, want paused_user", h.last().State)
	}
	// Away again: re-arms only after a full idle threshold.
	h.runFor(110 * time.Second)
	if got := h.m.burstCount(); got != n {
		t.Fatal("re-armed before the idle threshold")
	}
	h.runFor(12 * time.Second)
	if h.m.burstCount() != n+1 || h.last().State != StateSimulating {
		t.Fatalf("not simulating again after the idle threshold: %+v", h.last())
	}
}

func TestControllerUserReturnBetweenBurstsNeedsTolerance(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.runFor(2*time.Minute + 2*time.Second)
	// Right after a burst the idle counter is tiny; that is our own input.
	h.runFor(2 * time.Second)
	if h.last().State != StateSimulating {
		t.Fatalf("state = %s just after our burst, want simulating", h.last().State)
	}
}

func TestControllerPausesWhileLocked(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.runFor(3 * time.Minute)
	n := h.m.burstCount()
	h.m.set(func(m *machine) { m.locked = true })
	h.runFor(10 * time.Minute)
	if got := h.m.burstCount(); got != n {
		t.Fatalf("%d bursts while locked", got-n)
	}
	if st := h.last(); st.State != StatePausedLocked || st.Reason == "" {
		t.Fatalf("locked status = %+v", st)
	}
	// Unlocking means the user typed a password: wait for idle again.
	h.m.set(func(m *machine) { m.locked = false })
	h.m.userInput()
	h.runFor(2 * time.Second)
	if st := h.last(); st.State != StateWaitingIdle {
		t.Fatalf("state after unlock = %s, want waiting_idle", st.State)
	}
	h.runFor(2*time.Minute + 2*time.Second)
	if h.last().State != StateSimulating {
		t.Fatalf("state = %s, want simulating after idle", h.last().State)
	}
}

func TestControllerDegradesAfterTwoIneffectiveBurstsAndRecovers(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.m.set(func(m *machine) { m.ignored = true })
	h.runFor(2*time.Minute + 2*time.Second)
	if n := h.m.burstCount(); n != 1 {
		t.Fatalf("bursts = %d, want 1", n)
	}
	if st := h.last(); st.State != StateSimulating {
		t.Fatalf("after one ineffective burst state = %s, want still simulating", st.State)
	}
	h.runFor(41 * time.Second)
	if n := h.m.burstCount(); n != 2 {
		t.Fatalf("bursts = %d, want 2", n)
	}
	st := h.last()
	if st.State != StateDegraded || st.Reason != "input ignored" || st.Hint != "grant it" || st.Method != "Fake" {
		t.Fatalf("after two ineffective bursts: %+v", st)
	}
	// Keeps trying while degraded.
	h.runFor(3 * time.Minute)
	if n := h.m.burstCount(); n < 5 {
		t.Fatalf("stopped bursting while degraded (%d bursts)", n)
	}
	if st := h.last(); st.State != StateDegraded {
		t.Fatalf("state = %s, want degraded", st.State)
	}
	// Permission granted: the next burst works.
	h.m.set(func(m *machine) { m.ignored = false })
	h.runFor(41 * time.Second)
	if st := h.last(); st.State != StateSimulating || st.Reason != "" || st.Hint != "" {
		t.Fatalf("after an effective burst: %+v", st)
	}
}

func TestControllerBurstErrorsCountAsIneffective(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.m.set(func(m *machine) { m.playErr = errors.New("boom") })
	h.runFor(2*time.Minute + 2*time.Second + 41*time.Second)
	if st := h.last(); st.State != StateDegraded {
		t.Fatalf("state = %s after failing bursts, want degraded", st.State)
	}
}

func TestControllerFixedCadenceWithoutIdleSource(t *testing.T) {
	for name, tweak := range map[string]func(m *machine, d *controllerDeps){
		"nil source":     func(m *machine, d *controllerDeps) { d.idle = nil },
		"failing source": func(m *machine, d *controllerDeps) { m.idleErr = errors.New("NotSupported") },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, testCfg, tweak)
			h.runFor(time.Second)
			// Working as designed: no degraded state (and so no "not
			// working" notification), only a hint about its limit.
			st := h.last()
			if st.State != StateWaitingIdle || st.Method != "Fake on a fixed schedule" ||
				st.Hint != fixedHint || st.Reason != "first burst after 2m0s" {
				t.Fatalf("status = %+v", st)
			}
			// The user just started keepalive: the first burst waits one idle threshold.
			h.runFor(2*time.Minute - 2*time.Second)
			if n := h.m.burstCount(); n != 0 {
				t.Fatalf("%d bursts before the first threshold", n)
			}
			if st := h.last(); st.State != StateWaitingIdle || st.Idle < 2*time.Minute-3*time.Second {
				t.Fatalf("waiting status shows no progress: %+v", st)
			}
			h.runFor(2 * time.Second)
			h.runFor(10 * time.Minute)
			b := h.m.bursts
			if len(b) != 21 {
				t.Fatalf("bursts = %d, want 21 (one every 30s)", len(b))
			}
			for i := 1; i < len(b); i++ {
				if gap := b[i].Sub(b[i-1]); gap != 30*time.Second {
					t.Fatalf("gap %d = %v, want exactly 30s", i, gap)
				}
			}
			if st := h.last(); st.State != StateSimulating || st.LastBurst.IsZero() || st.Method != "Fake on a fixed schedule" ||
				st.Hint != fixedHint || st.Reason != "" {
				t.Fatalf("status = %+v", st)
			}
			for _, st := range h.statuses {
				if st.State == StateDegraded {
					t.Fatalf("degraded on the fixed schedule: %+v", st)
				}
			}
		})
	}
}

const fixedHint = "no idle source on this desktop, so simulated activity cannot pause while you use the computer; install an idle source"

func TestControllerReprobesUnavailableInjector(t *testing.T) {
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		m.openErr = &Unavailable{Reason: "no backend", Hint: "install xdotool"}
	})
	h.runFor(time.Second)
	st := h.last()
	if st.State != StateDegraded || st.Reason != "no backend" || st.Hint != "install xdotool" {
		t.Fatalf("status = %+v", st)
	}
	h.runFor(58 * time.Second)
	if h.m.opens != 1 {
		t.Fatalf("opened %d times within a minute, want 1", h.m.opens)
	}
	h.runFor(3 * time.Minute)
	if n := h.m.burstCount(); n != 0 {
		t.Fatalf("%d bursts without an injector", n)
	}
	if h.m.opens != 4 {
		t.Fatalf("opened %d times in 4 minutes, want 4", h.m.opens)
	}
	h.m.set(func(m *machine) { m.openErr = nil })
	h.runFor(62 * time.Second)
	if st := h.last(); st.State == StateDegraded {
		t.Fatalf("still degraded after the injector became available: %+v", st)
	}
	if h.m.burstCount() == 0 {
		t.Fatal("no burst after the injector became available although the user is idle")
	}
}

func TestControllerTapsKeysOnlyWhenEnabled(t *testing.T) {
	for _, keys := range []bool{false, true} {
		cfg := testCfg
		cfg.Keys = keys
		h := newHarness(t, cfg, nil)
		h.runFor(5 * time.Minute)
		n := h.m.burstCount()
		if n == 0 {
			t.Fatal("no bursts")
		}
		want := 0
		if keys {
			want = n
		}
		if h.m.taps != want {
			t.Fatalf("keys=%v: %d taps for %d bursts, want %d", keys, h.m.taps, n, want)
		}
	}
}

func TestControllerReportsEveryTransition(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.runFor(3 * time.Minute)
	h.userActiveFor(6 * time.Second)
	h.runFor(2*time.Minute + 2*time.Second)
	h.m.set(func(m *machine) { m.locked = true })
	h.runFor(4 * time.Second)
	h.m.set(func(m *machine) { m.locked = false })
	h.m.userInput()
	h.runFor(4 * time.Second)
	want := []State{StateWaitingIdle, StateSimulating, StatePausedUser, StateSimulating, StatePausedLocked, StateWaitingIdle}
	got := h.states()
	if len(got) != len(want) {
		t.Fatalf("states = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("states = %v, want %v", got, want)
		}
	}
	// Every burst while simulating is reported with its time.
	var lastBursts int
	var prev time.Time
	h.mu.Lock()
	for _, st := range h.statuses {
		if st.State == StateSimulating && !st.LastBurst.Equal(prev) {
			lastBursts++
			prev = st.LastBurst
		}
	}
	h.mu.Unlock()
	if lastBursts != h.m.burstCount() {
		t.Fatalf("%d burst reports for %d bursts", lastBursts, h.m.burstCount())
	}
}

func TestControllerRunBlocksUntilCancelledAndCloses(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		h := newHarness(t, testCfg, nil)
		if unavailable {
			h.m.set(func(m *machine) { m.openErr = &Unavailable{Reason: "none"} })
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- h.c.run(ctx) }()
		if !h.m.clk.WaitForWaiters(1, 2*time.Second) {
			t.Fatal("run never armed its timer")
		}
		h.m.clk.Advance(5 * time.Minute)
		select {
		case err := <-done:
			t.Fatalf("run returned on its own: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("run = %v, want nil", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("run did not return after cancel")
		}
		if !unavailable && h.m.closed != 1 {
			t.Fatalf("injector closed %d times, want 1", h.m.closed)
		}
	}
}

func TestControllerFixedCadenceFailureReasonIsStable(t *testing.T) {
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		d.idle = nil
		m.playErr = errors.New("xdotool: exit status 1")
	})
	h.runFor(2*time.Minute + 31*time.Second) // two failed bursts
	h.mu.Lock()
	h.statuses = nil
	h.mu.Unlock()
	h.runFor(2 * time.Minute)
	reasons := map[string]bool{}
	for _, st := range h.statuses {
		reasons[st.Reason] = true
	}
	if len(reasons) != 1 || !reasons["input ignored: xdotool: exit status 1"] {
		t.Fatalf("reasons while failing = %v", reasons)
	}
}

func TestControllerRunReturnsAtOnceWhenCancelled(t *testing.T) {
	h := newHarness(t, testCfg, nil)
	h.m.set(func(m *machine) { m.idleReads, m.opens = 0, 0 })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.c.run(ctx); err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if h.m.idleReads != 0 || h.m.opens != 0 || len(h.statuses) != 0 {
		t.Fatalf("cancelled run read idle %d times, opened %d injectors, reported %d statuses", h.m.idleReads, h.m.opens, len(h.statuses))
	}
}

func TestControllerNeverBurstsAfterCancel(t *testing.T) {
	for name, tweak := range map[string]func(m *machine, d *controllerDeps){
		"idle-gated":     nil,
		"fixed schedule": func(m *machine, d *controllerDeps) { d.idle = nil },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, testCfg, tweak)
			h.runFor(2*time.Minute - 2*time.Second)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			h.ctx = ctx
			h.runFor(time.Minute)
			if n := h.m.burstCount(); n != 0 {
				t.Fatalf("%d bursts after the context was cancelled", n)
			}
		})
	}
}

const testXWaylandNote = "Apps running under XWayland may not see this activity"

func TestControllerWarnsOnceWhenXWaylandMissesTwoBursts(t *testing.T) {
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		d.secondary = fakeXWayland{m}
		d.secondaryHint = testXWaylandNote
	})
	h.runFor(2*time.Minute + 2*time.Second)
	if st := h.last(); st.State != StateSimulating || st.Hint != "" {
		t.Fatalf("after one burst XWayland missed: %+v", st)
	}
	h.runFor(41 * time.Second)
	if n := h.m.burstCount(); n != 2 {
		t.Fatalf("bursts = %d, want 2", n)
	}
	if st := h.last(); st.State != StateSimulating || st.Hint != testXWaylandNote {
		t.Fatalf("after two bursts XWayland missed: %+v, want simulating with the XWayland hint", st)
	}
	// The hint stays, so the change is reported (and printed) once.
	h.m.set(func(m *machine) { m.xwFollows = true })
	h.runFor(3 * time.Minute)
	for _, st := range h.statuses[len(h.statuses)-5:] {
		if st.State != StateSimulating || st.Hint != testXWaylandNote {
			t.Fatalf("hint dropped later: %+v", st)
		}
	}
}

func TestControllerNoXWaylandHintWhenItFollows(t *testing.T) {
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		m.xwFollows = true
		d.secondary = fakeXWayland{m}
		d.secondaryHint = testXWaylandNote
	})
	h.runFor(10 * time.Minute)
	if h.m.burstCount() < 10 {
		t.Fatalf("only %d bursts", h.m.burstCount())
	}
	for _, st := range h.statuses {
		if st.Hint != "" {
			t.Fatalf("hint %q although XWayland saw every burst", st.Hint)
		}
	}
}

// namedInjector is a fakeInjector with its own name whose bursts fail
// while broken is set.
type namedInjector struct {
	fakeInjector
	name   string
	broken *bool
	tries  *int
}

func (n namedInjector) Name() string { return n.name }
func (n namedInjector) Play(ctx context.Context, ox, oy float64, p Path) error {
	n.m.mu.Lock()
	*n.tries++
	broken := *n.broken
	n.m.mu.Unlock()
	if broken {
		return errors.New(n.name + " broke")
	}
	return n.fakeInjector.Play(ctx, ox, oy, p)
}

// methods is an ordered list of input methods for deps.next.
type methods struct {
	m      *machine
	names  []string
	broken map[string]*bool
	tries  map[string]*int
	absent map[string]bool
}

func newMethods(m *machine, names ...string) *methods {
	ms := &methods{m: m, names: names, broken: map[string]*bool{}, tries: map[string]*int{}, absent: map[string]bool{}}
	for _, n := range names {
		ms.broken[n], ms.tries[n] = new(bool), new(int)
	}
	return ms
}

func (ms *methods) next(skip func(string) bool) (Injector, error) {
	ms.m.mu.Lock()
	defer ms.m.mu.Unlock()
	for _, n := range ms.names {
		if ms.absent[n] || (skip != nil && skip(n)) {
			continue
		}
		ms.m.opens++
		return namedInjector{fakeInjector{ms.m}, n, ms.broken[n], ms.tries[n]}, nil
	}
	return nil, &Unavailable{Reason: "no input method works"}
}

func (ms *methods) set(f func()) {
	ms.m.mu.Lock()
	f()
	ms.m.mu.Unlock()
}

func withMethods(ms **methods, names ...string) func(m *machine, d *controllerDeps) {
	return func(m *machine, d *controllerDeps) {
		*ms = newMethods(m, names...)
		d.next = (*ms).next
		d.open = func() (Injector, error) { return (*ms).next(nil) }
	}
}

func TestControllerFallsThroughAfterTwoFailures(t *testing.T) {
	var ms *methods
	h := newHarness(t, testCfg, withMethods(&ms, "uinput", "ydotool"))
	ms.set(func() { *ms.broken["uinput"] = true })
	h.runFor(2*time.Minute + 2*time.Second)
	if st := h.last(); st.State != StateSimulating || st.Method != "uinput" {
		t.Fatalf("after one failure: %+v", st)
	}
	h.runFor(41 * time.Second)
	if *ms.tries["uinput"] != 2 || *ms.tries["ydotool"] != 1 {
		t.Fatalf("tries uinput %d, ydotool %d; want 2 then the same burst on ydotool", *ms.tries["uinput"], *ms.tries["ydotool"])
	}
	if st := h.last(); st.State != StateSimulating || st.Method != "ydotool" || st.LastBurst.IsZero() {
		t.Fatalf("after falling through: %+v", st)
	}
	for _, s := range h.states() {
		if s == StateDegraded {
			t.Fatal("reported degraded although the next method worked")
		}
	}
	if h.m.closed != 1 {
		t.Fatalf("closed %d injectors, want the failed one", h.m.closed)
	}

	// A minute later the failed method is tried again: one failure sends
	// the same burst back to ydotool at once.
	before := *ms.tries["uinput"]
	h.runFor(3 * time.Minute)
	retries := *ms.tries["uinput"] - before
	if retries < 2 || retries > 4 {
		t.Fatalf("uinput retried %d times in 3 minutes, want about one a minute", retries)
	}
	if st := h.last(); st.Method != "ydotool" || st.State != StateSimulating {
		t.Fatalf("after retries: %+v", st)
	}
	for _, s := range h.states() {
		if s == StateDegraded {
			t.Fatal("a retry of the failed method reported degraded")
		}
	}

	// Fixed: the next retry keeps it.
	ms.set(func() { *ms.broken["uinput"] = false })
	h.runFor(2 * time.Minute)
	if st := h.last(); st.Method != "uinput" || st.State != StateSimulating {
		t.Fatalf("not back on uinput after it was fixed: %+v", st)
	}
	n := *ms.tries["ydotool"]
	h.runFor(3 * time.Minute)
	if *ms.tries["ydotool"] != n || h.last().Method != "uinput" {
		t.Fatal("left uinput again although it works")
	}
}

func TestControllerStaysDegradedWithoutAnotherMethod(t *testing.T) {
	var ms *methods
	h := newHarness(t, testCfg, withMethods(&ms, "uinput", "ydotool"))
	ms.set(func() {
		*ms.broken["uinput"] = true
		ms.absent["ydotool"] = true
	})
	h.runFor(2*time.Minute + 2*time.Second + 41*time.Second)
	st := h.last()
	if st.State != StateDegraded || st.Method != "uinput" {
		t.Fatalf("status = %+v, want degraded on uinput", st)
	}
	h.runFor(5 * time.Minute)
	if *ms.tries["uinput"] < 10 {
		t.Fatalf("stopped trying uinput (%d bursts) although nothing else works", *ms.tries["uinput"])
	}
	// ydotool shows up (daemon started): the next failure falls through.
	ms.set(func() { ms.absent["ydotool"] = false })
	h.runFor(41 * time.Second)
	if st := h.last(); st.State != StateSimulating || st.Method != "ydotool" {
		t.Fatalf("status = %+v, want simulating via ydotool", st)
	}
}

func TestControllerPausesWhenTheUserTakesThePointerMidBurst(t *testing.T) {
	var ms *methods
	h := newHarness(t, testCfg, withMethods(&ms, "uinput", "ydotool"))
	h.runFor(2*time.Minute + 2*time.Second)
	if h.last().State != StateSimulating {
		t.Fatalf("state = %s, want simulating", h.last().State)
	}
	// The user grabs the mouse during the next two bursts.
	h.m.set(func(m *machine) { m.playErr = errUserMoved })
	h.runFor(41 * time.Second)
	st := h.last()
	if st.State != StatePausedUser || st.Reason != userReason {
		t.Fatalf("status = %+v, want paused_user", st)
	}
	h.m.set(func(m *machine) { m.playErr = nil })
	h.m.userInput()
	h.userActiveFor(10 * time.Second)
	if h.last().State != StatePausedUser {
		t.Fatalf("state = %s while the user works, want paused_user", h.last().State)
	}
	h.runFor(2*time.Minute + 4*time.Second)
	if st := h.last(); st.State != StateSimulating || st.Method != "uinput" {
		t.Fatalf("after the user left again: %+v", st)
	}
	for _, s := range h.states() {
		if s == StateDegraded {
			t.Fatal("the user's hand counted as a failure")
		}
	}
	if *ms.tries["ydotool"] != 0 {
		t.Fatal("fell through to ydotool because the user moved the mouse")
	}
}

func TestControllerFixedScheduleBacksOffWhenTheUserTakesThePointer(t *testing.T) {
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) { d.idle = nil })
	h.runFor(2*time.Minute + 2*time.Second)
	n := h.m.burstCount()
	h.m.set(func(m *machine) { m.playErr = errUserMoved })
	h.runFor(31 * time.Second)
	h.m.set(func(m *machine) { m.playErr = nil })
	if st := h.last(); st.State != StatePausedUser {
		t.Fatalf("status = %+v, want paused_user", st)
	}
	h.runFor(110 * time.Second)
	if st := h.last(); st.State != StatePausedUser || h.m.burstCount() != n {
		t.Fatalf("burst again %v after the user took the pointer, status %+v", 110*time.Second, st)
	}
	h.runFor(12 * time.Second)
	if h.m.burstCount() != n+1 || h.last().State != StateSimulating {
		t.Fatalf("not back on the fixed schedule one idle threshold later: %+v", h.last())
	}
}

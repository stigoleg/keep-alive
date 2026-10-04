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
}

func newMachine() *machine {
	clk := clock.NewFake(time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC))
	return &machine{clk: clk, lastInput: clk.Now()}
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
	if f.m.idleErr != nil {
		return 0, f.m.idleErr
	}
	return max(f.m.clk.Now().Sub(f.m.lastInput), 0), nil
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
			st := h.last()
			if st.State != StateDegraded || st.Reason != noIdleReason || st.Hint != "install an idle source" {
				t.Fatalf("status = %+v", st)
			}
			// The user just started keepalive: the first burst waits one idle threshold.
			h.runFor(2*time.Minute - 2*time.Second)
			if n := h.m.burstCount(); n != 0 {
				t.Fatalf("%d bursts before the first threshold", n)
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
			if st := h.last(); st.State != StateDegraded || st.LastBurst.IsZero() {
				t.Fatalf("status = %+v", st)
			}
		})
	}
}

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

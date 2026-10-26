package activity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultWaitsThreeMinutesAndHonorsOverrides(t *testing.T) {
	for _, threshold := range []time.Duration{DefaultIdleThreshold, 90 * time.Second} {
		if DefaultIdleThreshold != 3*time.Minute {
			t.Fatal("default is not three minutes")
		}
		h := newHarness(t, Config{IdleThreshold: threshold, Interval: DefaultInterval}, nil)
		h.runFor(threshold - time.Second)
		if h.m.burstCount() != 0 {
			t.Fatal("burst before the configured idle threshold")
		}
		h.runFor(3 * time.Second)
		if h.m.burstCount() != 1 {
			t.Fatal("no burst after the configured idle threshold")
		}
	}
}

func TestControllerDetectsInputImmediatelyAfterBurst(t *testing.T) {
	h := newHarness(t, Config{IdleThreshold: DefaultIdleThreshold, Interval: DefaultInterval}, nil)
	h.runFor(DefaultIdleThreshold + time.Millisecond)
	n := h.m.burstCount()
	h.m.userInput() // inside the old three-second blind window
	h.runFor(time.Millisecond)
	if h.last().State != StatePausedUser {
		t.Fatalf("state = %+v", h.last())
	}
	h.runFor(DefaultIdleThreshold - time.Second)
	if h.m.burstCount() != n {
		t.Fatal("rearmed before a full idle period")
	}
	h.runFor(3 * time.Second)
	if h.m.burstCount() != n+1 {
		t.Fatal("did not resume after a full idle period")
	}
}

func TestHeldInputPreventsSimulationUntilReleasedAndIdle(t *testing.T) {
	for _, key := range []uint64{42, 0x110} {
		h := newHarness(t, testCfg, nil)
		h.m.input.input(key, true)
		h.runFor(10 * time.Minute)
		if h.m.burstCount() != 0 {
			t.Fatal("simulated with a held key or button")
		}
		h.m.input.input(key, false)
		h.runFor(testCfg.IdleThreshold - time.Second)
		if h.m.burstCount() != 0 {
			t.Fatal("did not restart the idle wait after release")
		}
		h.runFor(3 * time.Second)
		if h.m.burstCount() != 1 {
			t.Fatal("did not resume after release and idle")
		}
	}
}

func TestPlaybackYieldsWithoutRestoringOrFinishing(t *testing.T) {
	for _, failure := range []bool{false, true} {
		for _, relative := range []bool{false, true} {
			state := newInputState(time.Now)
			monitor := fakeMonitor{state}
			ctx, stop := watchInput(context.Background(), monitor, 0)
			var injector Injector
			abs := &finishingRecorder{absRecorder: absRecorder{ox: 700, oy: 400}}
			rel := &relRecorder{}
			injector = abs
			if relative {
				injector = rel
			}
			n := 0
			sleep := func(context.Context, time.Duration) error {
				n++
				if n == 10 {
					if failure {
						state.fail(errors.New("observer failed"))
					} else {
						state.input(42, true)
					}
				}
				return nil // check synchronously, even before the watcher runs
			}
			err := playBurst(ctx, injector, NewPath(seeded(7)), sleep)
			stop()
			if failure && !isMonitorFailure(err) || !failure && !errors.Is(err, errUserMoved) {
				t.Fatalf("interruption = %v", err)
			}
			if relative {
				if len(rel.moves) != 9 {
					t.Fatalf("relative commands after takeover: %d", len(rel.moves))
				}
			} else if len(abs.moves) != 9 || len(abs.finished) != 0 {
				t.Fatalf("absolute commands after takeover: %d, finishes %d", len(abs.moves), len(abs.finished))
			}
		}
	}
}

type blockingPath struct {
	fakeInjector
	started chan struct{}
}

func (b blockingPath) Play(ctx context.Context, _, _ float64, _ Path) error {
	close(b.started)
	<-ctx.Done()
	return context.Cause(ctx)
}

func TestInputWakesBlockedPlaybackWithin100Milliseconds(t *testing.T) {
	for _, kind := range []string{"typing", "click", "scroll", "drag", "monitor failure"} {
		t.Run(kind, func(t *testing.T) {
			m := newMachine()
			monitor := fakeMonitor{m.input}
			ctx, stop := watchInput(context.Background(), monitor, 0)
			defer stop()
			started, done := make(chan struct{}), make(chan error, 1)
			go func() { done <- playBurst(ctx, blockingPath{fakeInjector{m}, started}, NewPath(seeded(1)), realSleep) }()
			<-started
			if kind == "monitor failure" {
				m.input.fail(errors.New("lost coverage"))
			} else {
				m.input.input(0, false)
			}
			select {
			case err := <-done:
				if !errors.Is(err, errUserMoved) && !isMonitorFailure(err) {
					t.Fatalf("playback = %v", err)
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("input did not cancel blocked playback within 100ms")
			}
		})
	}
}

type inputAtEnd struct{ fakeInjector }

func (i inputAtEnd) Play(ctx context.Context, x, y float64, p Path) error {
	if err := i.fakeInjector.Play(ctx, x, y, p); err != nil {
		return err
	}
	i.m.userInput()
	return nil
}

func TestInputAtEndOfPathSkipsShiftAndFailover(t *testing.T) {
	cfg := testCfg
	cfg.Keys = true
	var fallback bool
	h := newHarness(t, cfg, func(m *machine, d *controllerDeps) {
		d.open = func() (Injector, error) { return inputAtEnd{fakeInjector{m}}, nil }
		d.next = func(func(string) bool) (Injector, error) { fallback = true; return fakeInjector{m}, nil }
	})
	h.runFor(3 * time.Minute)
	if h.m.taps != 0 || fallback || h.last().State != StatePausedUser {
		t.Fatalf("takeover: taps=%d fallback=%v status=%+v", h.m.taps, fallback, h.last())
	}
}

func TestMonitorRecoveryRequiresFreshIdleAndRetriesEveryMinute(t *testing.T) {
	attempts := 0
	var h *harness
	h = newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		d.monitor = func(context.Context) (inputMonitor, error) {
			attempts++
			if attempts > 1 {
				m.input = newInputState(m.clk.Now)
			}
			return fakeMonitor{m.input}, nil
		}
	})
	h.runFor(3 * time.Minute)
	n := h.m.burstCount()
	h.m.input.fail(errors.New("lost coverage"))
	h.runFor(59 * time.Second)
	if attempts != 1 || h.m.burstCount() != n || h.last().State != StateDegraded {
		t.Fatalf("lost coverage: attempts=%d status=%+v", attempts, h.last())
	}
	h.runFor(3 * time.Second)
	if attempts != 2 {
		t.Fatalf("retry attempts = %d", attempts)
	}
	h.runFor(testCfg.IdleThreshold - 4*time.Second)
	if h.m.burstCount() != n {
		t.Fatal("recovery reused stale OS inactivity")
	}
	h.runFor(6 * time.Second)
	if h.m.burstCount() != n+1 {
		t.Fatal("did not resume after a full observed idle period")
	}
}

func TestSeedCannotOverwriteInputOrHeldState(t *testing.T) {
	state := newInputState(time.Now)
	before := state.Snapshot()
	state.input(10, true)
	select {
	case <-before.Changed:
	default:
		t.Fatal("input did not notify subscribers")
	}
	state.SeedIdle(time.Hour, before.Generation)
	if snap := state.Snapshot(); snap.Idle > time.Second || !snap.Held {
		t.Fatalf("stale seed hid real input: %+v", snap)
	}
	state.input(10, false)
	state.SeedIdle(time.Hour, state.Snapshot().Generation)
	if state.Snapshot().Idle < time.Hour {
		t.Fatal("could not seed initial idle")
	}
}

func TestDiagnosticsRequiresMonitoringEvenWithWorkingInjector(t *testing.T) {
	m := newMachine()
	b := probeBackend(m)
	b.monitor = nil
	d := diagnose(b)
	if d.Usable() {
		t.Fatal("diagnostics ignored missing monitoring")
	}
	if reason, _ := d.Problem(); reason == "" {
		t.Fatal("no explanation for paused simulation")
	}
	res := probe(context.Background(), b, true, seeded(1), noSleep)
	if res.Reason == "" || m.burstCount() != 0 || m.taps != 0 {
		t.Fatalf("unsafe probe: %+v", res)
	}
}

func TestMonitorOutlivesSessionCancellationUntilCleanup(t *testing.T) {
	var monitorContext context.Context
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		d.monitor = func(ctx context.Context) (inputMonitor, error) {
			monitorContext = ctx
			return fakeMonitor{m.input}, nil
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	if !h.c.ensureMonitor(ctx, h.m.clk.Now()) {
		t.Fatal("monitor did not start")
	}
	cancel()
	if monitorContext.Err() != nil {
		t.Fatal("shutdown cleanup lost user-input monitoring")
	}
	h.c.closeMonitor()
	if monitorContext.Err() == nil {
		t.Fatal("monitor context leaked after cleanup")
	}
}

func TestStartupFailureDoesNotSeedRecoveryFromStaleIdle(t *testing.T) {
	attempts := 0
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		d.monitor = func(context.Context) (inputMonitor, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("initial monitoring failed")
			}
			m.input = newInputState(m.clk.Now)
			return fakeMonitor{m.input}, nil
		}
	})
	h.runFor(123 * time.Second)
	if h.m.burstCount() != 0 || attempts != 2 {
		t.Fatalf("recovery reused old idle: bursts=%d attempts=%d", h.m.burstCount(), attempts)
	}
	h.runFor(time.Minute)
	if h.m.burstCount() != 1 {
		t.Fatal("did not resume after fresh observed idle")
	}
}

type preparedTestInjector struct {
	fakeInjector
	prepared *bool
}

func (i preparedTestInjector) Prepare(context.Context) error { *i.prepared = true; return nil }

func TestVirtualDeviceIsPreparedBeforeMonitoring(t *testing.T) {
	prepared := false
	h := newHarness(t, testCfg, func(m *machine, d *controllerDeps) {
		d.open = func() (Injector, error) { return preparedTestInjector{fakeInjector{m}, &prepared}, nil }
		d.monitor = func(context.Context) (inputMonitor, error) {
			if !prepared {
				t.Fatal("observation started before virtual-device setup")
			}
			return fakeMonitor{m.input}, nil
		}
	})
	h.runFor(time.Second)
	if !prepared || h.m.burstCount() != 0 {
		t.Fatal("virtual-device setup emitted input")
	}
}

package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/power"
)

// lossyPower hands out holds that implement power.Watcher.
type lossyPower struct {
	clk       *clock.Fake
	attempted chan struct{}

	mu       sync.Mutex
	failures int // the next N Acquire calls fail
	attempts []time.Time
	holds    []*lossyHold
}

func newLossyPower(clk *clock.Fake) *lossyPower {
	return &lossyPower{clk: clk, attempted: make(chan struct{}, 16)}
}

func (p *lossyPower) Name() string { return "lossy" }

func (p *lossyPower) Acquire(ctx context.Context, o power.Options) (power.Hold, error) {
	p.mu.Lock()
	defer func() {
		p.mu.Unlock()
		p.attempted <- struct{}{}
	}()
	p.attempts = append(p.attempts, p.clk.Now())
	if p.failures > 0 {
		p.failures--
		return nil, errors.New("bus down")
	}
	h := &lossyHold{n: len(p.holds) + 1, lost: make(chan error, 1)}
	p.holds = append(p.holds, h)
	return h, nil
}

func (p *lossyPower) setFailures(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failures = n
}

func (p *lossyPower) hold(i int) *lossyHold {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.holds[i]
}

func (p *lossyPower) attemptOffsets(from time.Time) []time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []time.Duration
	for _, at := range p.attempts {
		if at.After(from) {
			out = append(out, at.Sub(from))
		}
	}
	return out
}

func (p *lossyPower) waitAttempt(t *testing.T) {
	t.Helper()
	select {
	case <-p.attempted:
	case <-time.After(waitTimeout):
		t.Fatal("Acquire was not called")
	}
}

type lossyHold struct {
	n        int
	lost     chan error
	releases atomic.Int32
}

func (h *lossyHold) Release() error     { h.releases.Add(1); return nil }
func (h *lossyHold) Describe() string   { return fmt.Sprintf("hold %d", h.n) }
func (h *lossyHold) Lost() <-chan error { return h.lost }

// lossHarness reuses the harness with a lossy inhibitor.
func newLossHarness(t *testing.T) (*harness, *lossyPower) {
	t.Helper()
	h := newHarness(t, Config{})
	p := newLossyPower(h.clk)
	h.s = New(Config{}, Deps{Clock: h.clk, Power: p})
	var unsub func()
	h.events, unsub = h.s.Subscribe()
	t.Cleanup(unsub)
	return h, p
}

// advanceToRetry waits for the retry timer to be armed (heartbeat + retry)
// and moves the clock by d.
func advanceToRetry(t *testing.T, h *harness, d time.Duration) {
	t.Helper()
	if !h.clk.WaitForWaiters(2, waitTimeout) {
		t.Fatal("re-acquire timer not armed")
	}
	h.clk.Advance(d)
}

func TestLostPowerHoldIsReacquiredWithBackoff(t *testing.T) {
	h, p := newLossHarness(t)
	h.start()
	p.waitAttempt(t)
	first := p.hold(0)

	p.setFailures(4)
	lostAt := h.clk.Now()
	first.lost <- errors.New("session bus connection lost")
	ev := h.waitFor(EventWarning)
	if !strings.Contains(ev.Message, "power hold lost") || !strings.Contains(ev.Message, "session bus connection lost") {
		t.Fatalf("loss warning = %q", ev.Message)
	}
	if ev.Snapshot.PowerHold != "" {
		t.Fatalf("PowerHold while lost = %q, want empty", ev.Snapshot.PowerHold)
	}

	for _, d := range []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 30 * time.Second, 30 * time.Second} {
		advanceToRetry(t, h, d)
		p.waitAttempt(t)
	}
	ev = h.waitFor(EventWarning)
	if !strings.Contains(ev.Message, "re-acquired") || ev.Snapshot.PowerHold != "hold 2" {
		t.Fatalf("recovery event = %q, PowerHold %q", ev.Message, ev.Snapshot.PowerHold)
	}
	want := []time.Duration{time.Second, 3 * time.Second, 8 * time.Second, 38 * time.Second, 68 * time.Second}
	if got := p.attemptOffsets(lostAt); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("re-acquire attempts at %v after the loss, want %v", got, want)
	}
	if r := first.releases.Load(); r != 1 {
		t.Fatalf("lost hold released %d times, want 1", r)
	}

	// A second loss starts the backoff again from 1s.
	second := p.hold(1)
	lostAt = h.clk.Now()
	second.lost <- errors.New("gone again")
	h.waitFor(EventWarning)
	advanceToRetry(t, h, time.Second)
	p.waitAttempt(t)
	if ev := h.waitFor(EventWarning); ev.Snapshot.PowerHold != "hold 3" {
		t.Fatalf("PowerHold after second recovery = %q", ev.Snapshot.PowerHold)
	}
	if got := p.attemptOffsets(lostAt); fmt.Sprint(got) != fmt.Sprint([]time.Duration{time.Second}) {
		t.Fatalf("second recovery attempts at %v", got)
	}

	h.s.Stop(ReasonUser)
	h.finish(ReasonUser)
	for i, want := range []int32{1, 1, 1} {
		if r := p.hold(i).releases.Load(); r != want {
			t.Fatalf("hold %d released %d times, want %d", i+1, r, want)
		}
	}
}

func TestStopWhilePowerHoldLost(t *testing.T) {
	h, p := newLossHarness(t)
	h.start()
	p.waitAttempt(t)
	p.setFailures(1)
	p.hold(0).lost <- errors.New("bus gone")
	h.waitFor(EventWarning)
	advanceToRetry(t, h, time.Second)
	p.waitAttempt(t) // fails; the next retry is armed

	h.s.Stop(ReasonUser)
	r := h.finish(ReasonUser)
	if r.Err != nil {
		t.Fatalf("Result.Err = %v", r.Err)
	}
	if n := p.hold(0).releases.Load(); n != 1 {
		t.Fatalf("lost hold released %d times, want 1", n)
	}
	h.clk.Advance(time.Hour)
	select {
	case <-p.attempted:
		t.Fatal("re-acquired after the session stopped")
	case <-time.After(50 * time.Millisecond):
	}
}

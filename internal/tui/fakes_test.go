package tui

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

var errDenied = errors.New("denied")

type fakePower struct {
	acquires, releases atomic.Int32
	err                error
}

func (p *fakePower) Name() string { return "fake" }

func (p *fakePower) Acquire(context.Context, power.Options) (power.Hold, error) {
	if p.err != nil {
		return nil, p.err
	}
	p.acquires.Add(1)
	return fakeHold{p}, nil
}

type fakeHold struct{ p *fakePower }

func (h fakeHold) Release() error   { h.p.releases.Add(1); return nil }
func (h fakeHold) Describe() string { return "fake" }

type fakeSim struct{}

func (fakeSim) Run(ctx context.Context, _ activity.Config, report func(activity.Status)) error {
	report(activity.Status{State: activity.StateWaitingIdle})
	<-ctx.Done()
	return nil
}

type testDeps struct{ power *fakePower }

// startTestSession builds a model wired to fake session dependencies.
func startTestSession(t *testing.T, o Options) (Model, *testDeps) {
	t.Helper()
	d := &testDeps{power: &fakePower{}}
	o.Deps = session.Deps{
		Power:    d.power,
		Activity: fakeSim{},
		Battery: func() (platform.BatteryStatus, error) {
			return platform.BatteryStatus{Percentage: 80, Available: true}, nil
		},
	}
	m := New(o)
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, d
}

func stopTestSession(t *testing.T, m Model) {
	t.Helper()
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
}

// nextEvent waits for the next event of the model's running session.
func nextEvent(t *testing.T, m Model) sessionEventMsg {
	t.Helper()
	r := m.sessions.current()
	if r == nil {
		t.Fatal("no running session")
	}
	ch := make(chan any, 1)
	go func() { ch <- waitForEvent(r)() }()
	select {
	case msg := <-ch:
		ev, ok := msg.(sessionEventMsg)
		if !ok {
			t.Fatalf("event stream ended (%T)", msg)
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("no session event")
		return sessionEventMsg{}
	}
}

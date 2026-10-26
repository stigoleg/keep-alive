package activity

import (
	"context"
	"errors"
	"sync"
	"time"
)

// inputMonitor observes real input independently of the idle counter that our
// own events reset. Changed is closed on input or loss of monitoring coverage.
// A snapshot and its notification channel are read atomically.
type inputMonitor interface {
	Name() string
	Snapshot() inputSnapshot
	SeedIdle(time.Duration, uint64)
	Close() error
}

type inputSnapshot struct {
	Idle       time.Duration
	Generation uint64
	Held       bool
	Err        error
	Changed    <-chan struct{}
}

type inputState struct {
	mu         sync.Mutex
	now        func() time.Time
	last       time.Time
	generation uint64
	held       map[uint64]bool
	err        error
	changed    chan struct{}
}

func newInputState(now func() time.Time) *inputState {
	return &inputState{now: now, last: now(), held: make(map[uint64]bool), changed: make(chan struct{})}
}

func (s *inputState) Snapshot() inputSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return inputSnapshot{Idle: max(s.now().Sub(s.last), 0), Generation: s.generation,
		Held: len(s.held) > 0, Err: s.err, Changed: s.changed}
}

func (s *inputState) signal() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// key identifies transient held state only. Motion uses key=0; no input values
// are retained in logs, diagnostics, or session output.
func (s *inputState) input(key uint64, down bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != 0 {
		if down {
			s.held[key] = true
		} else {
			delete(s.held, key)
		}
	}
	s.last = s.now()
	s.generation++
	s.signal()
}

func (s *inputState) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
		s.signal()
	}
}

func (s *inputState) SeedIdle(idle time.Duration, generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == generation && len(s.held) == 0 && s.err == nil {
		s.last = s.now().Add(-max(idle, 0))
	}
}

// monitorFailure is never an injector failure and must not trigger failover.
type monitorFailure struct{ error }

type inputGuard struct {
	monitor    inputMonitor
	generation uint64
}

type inputGuardKey struct{}

func (g inputGuard) check() error {
	s := g.monitor.Snapshot()
	if s.Err != nil {
		return monitorFailure{s.Err}
	}
	if s.Generation != g.generation || s.Held {
		return errUserMoved
	}
	return nil
}

// watchInput wakes blocked sleeps and helper processes immediately. The same
// guard is also checked synchronously before each injection, closing the gap
// between a notification being delivered and its watcher being scheduled.
func watchInput(parent context.Context, monitor inputMonitor, generation uint64) (context.Context, func()) {
	guard := inputGuard{monitor, generation}
	ctx, cancel := context.WithCancelCause(context.WithValue(parent, inputGuardKey{}, guard))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			s := monitor.Snapshot()
			if err := guard.check(); err != nil {
				cancel(err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-s.Changed:
			}
		}
	}()
	return ctx, func() { cancel(context.Canceled); <-done }
}

func inputError(ctx context.Context) error {
	if g, ok := ctx.Value(inputGuardKey{}).(inputGuard); ok {
		if err := g.check(); err != nil {
			return err
		}
	}
	return context.Cause(ctx)
}

// restore only cleans up an ordinary error/cancellation while the user still
// leaves the input alone. Its separate watcher also cancels a cleanup helper.
func restore(ctx context.Context, fn func(context.Context)) {
	if err := inputError(ctx); errors.Is(err, errUserMoved) {
		return
	} else if isMonitorFailure(err) {
		return
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 250*time.Millisecond)
	defer cancel()
	if g, ok := ctx.Value(inputGuardKey{}).(inputGuard); ok {
		var stop func()
		cleanup, stop = watchInput(cleanup, g.monitor, g.generation)
		defer stop()
	}
	if inputError(cleanup) == nil {
		fn(cleanup)
	}
}

func isMonitorFailure(err error) bool {
	var failure monitorFailure
	return errors.As(err, &failure)
}

// SeedHeld installs a startup snapshot only if no observed input could have
// made it stale while the native query was in flight.
func (s *inputState) SeedHeld(keys []uint64, generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation != generation || s.err != nil {
		return false
	}
	for _, key := range keys {
		s.held[key] = true
	}
	return true
}

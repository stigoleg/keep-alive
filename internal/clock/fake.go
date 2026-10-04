package clock

import (
	"sort"
	"sync"
	"time"
)

// Fake is a manually advanced Clock for tests. Timers and tickers fire only
// when Advance or Set moves the fake time past their deadline. Like the time
// package, every channel has a buffer of one and a tick that finds the buffer
// full is dropped.
type Fake struct {
	mu      sync.Mutex
	now     time.Time
	waiters []*fakeWaiter
	changed chan struct{}
}

type fakeWaiter struct {
	clock    *Fake
	ch       chan time.Time
	deadline time.Time
	period   time.Duration // 0 for timers
	active   bool
}

// NewFake returns a fake clock set to now.
func NewFake(now time.Time) *Fake {
	return &Fake{now: now, changed: make(chan struct{})}
}

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// NewTimer returns a timer that fires once d after the current fake time.
func (f *Fake) NewTimer(d time.Duration) Timer {
	return &fakeTimer{f.add(d, 0)}
}

// NewTicker returns a ticker with period d. It panics on d <= 0 like
// time.NewTicker.
func (f *Fake) NewTicker(d time.Duration) Ticker {
	if d <= 0 {
		panic("clock: non-positive interval for NewTicker")
	}
	return &fakeTicker{f.add(d, d)}
}

// After is NewTimer(d).C().
func (f *Fake) After(d time.Duration) <-chan time.Time {
	return f.NewTimer(d).C()
}

// Advance moves the fake time forward by d, firing everything that falls due.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	target := f.now.Add(d)
	f.mu.Unlock()
	f.Set(target)
}

// Set moves the fake time to t, firing every timer and ticker whose deadline
// is at or before t in deadline order. Moving backwards fires nothing.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for {
		next := f.nextDueLocked(t)
		if next == nil {
			break
		}
		f.now = next.deadline
		select {
		case next.ch <- next.deadline:
		default:
		}
		if next.period > 0 {
			next.deadline = next.deadline.Add(next.period)
		} else {
			next.active = false
		}
	}
	f.now = t
	f.pruneLocked()
}

// Waiters reports how many timers and tickers are currently armed. Tests use
// it with WaitForWaiters to know that the code under test has armed its
// timers before advancing the clock.
func (f *Fake) Waiters() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.countLocked()
}

// WaitForWaiters blocks until at least n timers and tickers are armed or the
// real-time timeout passes; it reports whether the count was reached.
func (f *Fake) WaitForWaiters(n int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		f.mu.Lock()
		if f.countLocked() >= n {
			f.mu.Unlock()
			return true
		}
		changed := f.changed
		f.mu.Unlock()
		select {
		case <-changed:
		case <-deadline:
			return false
		}
	}
}

func (f *Fake) add(d time.Duration, period time.Duration) *fakeWaiter {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := &fakeWaiter{clock: f, ch: make(chan time.Time, 1), deadline: f.now.Add(d), period: period, active: true}
	f.waiters = append(f.waiters, w)
	f.notifyLocked()
	return w
}

func (f *Fake) nextDueLocked(t time.Time) *fakeWaiter {
	var due []*fakeWaiter
	for _, w := range f.waiters {
		if w.active && !w.deadline.After(t) {
			due = append(due, w)
		}
	}
	if len(due) == 0 {
		return nil
	}
	sort.SliceStable(due, func(i, j int) bool { return due[i].deadline.Before(due[j].deadline) })
	return due[0]
}

func (f *Fake) countLocked() int {
	n := 0
	for _, w := range f.waiters {
		if w.active {
			n++
		}
	}
	return n
}

func (f *Fake) pruneLocked() {
	kept := f.waiters[:0]
	for _, w := range f.waiters {
		if w.active {
			kept = append(kept, w)
		}
	}
	for i := len(kept); i < len(f.waiters); i++ {
		f.waiters[i] = nil
	}
	f.waiters = kept
}

// notifyLocked wakes every WaitForWaiters call.
func (f *Fake) notifyLocked() {
	close(f.changed)
	f.changed = make(chan struct{})
}

func (w *fakeWaiter) stop() bool {
	f := w.clock
	f.mu.Lock()
	defer f.mu.Unlock()
	was := w.active
	w.active = false
	w.drain()
	f.pruneLocked()
	f.notifyLocked()
	return was
}

func (w *fakeWaiter) reset(d time.Duration) bool {
	f := w.clock
	f.mu.Lock()
	defer f.mu.Unlock()
	was := w.active
	w.drain()
	w.deadline = f.now.Add(d)
	if w.period > 0 {
		w.period = d
	}
	if !w.active {
		w.active = true
		f.waiters = append(f.waiters, w)
	}
	f.notifyLocked()
	return was
}

// drain discards an undelivered tick so Stop and Reset never leave a stale
// value behind, matching time.Timer since Go 1.23.
func (w *fakeWaiter) drain() {
	select {
	case <-w.ch:
	default:
	}
}

type fakeTimer struct{ w *fakeWaiter }

func (t *fakeTimer) C() <-chan time.Time        { return t.w.ch }
func (t *fakeTimer) Stop() bool                 { return t.w.stop() }
func (t *fakeTimer) Reset(d time.Duration) bool { return t.w.reset(d) }

type fakeTicker struct{ w *fakeWaiter }

func (t *fakeTicker) C() <-chan time.Time { return t.w.ch }
func (t *fakeTicker) Stop()               { t.w.stop() }
func (t *fakeTicker) Reset(d time.Duration) {
	if d <= 0 {
		panic("clock: non-positive interval for Ticker.Reset")
	}
	t.w.reset(d)
}

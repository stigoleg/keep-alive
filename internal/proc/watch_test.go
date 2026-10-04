package proc

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/clock"
)

const interval = 2 * time.Second

var epoch = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

// fakeLister is an in-memory process table.
type fakeLister struct {
	mu      sync.Mutex
	alive   map[int]bool
	names   map[string]int // name -> number of matching processes
	failing bool           // every lookup returns an error
}

func newFakeLister() *fakeLister {
	return &fakeLister{alive: map[int]bool{}, names: map[string]int{}}
}

func (f *fakeLister) Alive(pid int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failing {
		return false, errors.New("lookup failed")
	}
	return f.alive[pid], nil
}

func (f *fakeLister) FindByName(name string) ([]Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failing {
		return nil, errors.New("lookup failed")
	}
	procs := make([]Process, f.names[name])
	for i := range procs {
		procs[i] = Process{PID: 1000 + i, Name: name}
	}
	return procs, nil
}

func (f *fakeLister) set(pid int, alive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[pid] = alive
}

func (f *fakeLister) setName(name string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names[name] = n
}

func (f *fakeLister) setFailing(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = on
}

// harness runs a Watcher on a fake clock and lets the test step it one poll
// at a time.
type harness struct {
	t      *testing.T
	clk    *clock.Fake
	lister *fakeLister
	polled chan struct{}
	exits  <-chan Exit
}

func newHarness(t *testing.T) *harness {
	return &harness{t: t, clk: clock.NewFake(epoch), lister: newFakeLister(), polled: make(chan struct{}, 1)}
}

func (h *harness) watcher() *Watcher {
	w := NewWatcher(h.clk, interval, h.lister)
	w.polled = func() { h.polled <- struct{}{} }
	return w
}

func (h *harness) start(ctx context.Context, pids []int, name string) {
	h.t.Helper()
	exits, err := h.watcher().Watch(ctx, pids, name)
	if err != nil {
		h.t.Fatalf("Watch(%v, %q): %v", pids, name, err)
	}
	h.exits = exits
}

// poll advances the clock by one interval and waits for the poll to finish.
func (h *harness) poll() {
	h.t.Helper()
	h.clk.Advance(interval)
	select {
	case <-h.polled:
	case <-time.After(5 * time.Second):
		h.t.Fatal("the watcher did not poll after the interval passed")
	}
}

func (h *harness) expectNoExit() {
	h.t.Helper()
	select {
	case ex, ok := <-h.exits:
		h.t.Fatalf("unexpected exit %+v (open %v)", ex, ok)
	default:
	}
}

func (h *harness) expectExit(reason string) {
	h.t.Helper()
	select {
	case ex, ok := <-h.exits:
		if !ok {
			h.t.Fatal("exit channel closed without an Exit")
		}
		if ex.Reason != reason {
			h.t.Fatalf("Exit.Reason = %q, want %q", ex.Reason, reason)
		}
	case <-time.After(5 * time.Second):
		h.t.Fatal("no Exit")
	}
	h.expectClosed()
}

func (h *harness) expectClosed() {
	h.t.Helper()
	select {
	case ex, ok := <-h.exits:
		if ok {
			h.t.Fatalf("second value on the exit channel: %+v", ex)
		}
	case <-time.After(5 * time.Second):
		h.t.Fatal("exit channel not closed")
	}
}

func TestWatchPIDExit(t *testing.T) {
	h := newHarness(t)
	h.lister.set(4242, true)
	h.start(context.Background(), []int{4242}, "")
	if n := h.clk.Waiters(); n != 1 {
		t.Fatalf("Watch armed %d timers, want its ticker armed before it returns", n)
	}

	h.poll()
	h.expectNoExit()

	h.lister.set(4242, false)
	h.poll()
	h.expectExit("process 4242 exited")
	if n := h.clk.Waiters(); n != 0 {
		t.Fatalf("%d timers still armed after the watch ended", n)
	}
}

func TestWatchAnyPID(t *testing.T) {
	h := newHarness(t)
	h.lister.set(10, true)
	h.lister.set(11, true)
	h.lister.set(12, false) // already gone: ignored
	h.start(context.Background(), []int{10, 11, 12, 10}, "")

	h.lister.set(10, false)
	h.poll()
	h.expectNoExit()

	// A pid that died stays dead even if the number is reused.
	h.lister.set(10, true)
	h.lister.set(11, false)
	h.poll()
	h.expectExit("process 11 exited")
}

func TestWatchPIDsExitTogether(t *testing.T) {
	h := newHarness(t)
	h.lister.set(10, true)
	h.lister.set(11, true)
	h.start(context.Background(), []int{10, 11}, "")
	h.lister.set(10, false)
	h.lister.set(11, false)
	h.poll()
	h.expectExit("processes 10, 11 exited")
}

func TestWatchNameDisappears(t *testing.T) {
	h := newHarness(t)
	h.lister.setName("zoom", 2)
	h.start(context.Background(), nil, "zoom")

	h.lister.setName("zoom", 1)
	h.poll()
	h.expectNoExit()

	h.lister.setName("zoom", 0)
	h.poll()
	h.expectExit(`no process named "zoom" is running anymore`)
}

func TestWatchPIDAndName(t *testing.T) {
	h := newHarness(t)
	h.lister.set(4242, true)
	h.lister.setName("zoom", 1)
	h.start(context.Background(), []int{4242}, "zoom")

	h.lister.setName("zoom", 0) // the pid keeps it awake
	h.poll()
	h.expectNoExit()

	h.lister.setName("zoom", 1) // a new zoom counts again
	h.lister.set(4242, false)
	h.poll()
	h.expectNoExit()

	h.lister.setName("zoom", 0)
	h.poll()
	h.expectExit(`no process named "zoom" is running anymore`)
}

func TestWatchPIDAndNameTogether(t *testing.T) {
	h := newHarness(t)
	h.lister.set(4242, true)
	h.lister.setName("zoom", 1)
	h.start(context.Background(), []int{4242}, "zoom")
	h.lister.set(4242, false)
	h.lister.setName("zoom", 0)
	h.poll()
	h.expectExit(`process 4242 exited and no process named "zoom" is running anymore`)
}

func TestWatchLookupErrorsKeepWatching(t *testing.T) {
	h := newHarness(t)
	h.lister.set(4242, true)
	h.lister.setName("zoom", 1)
	h.start(context.Background(), []int{4242}, "zoom")

	h.lister.set(4242, false)
	h.lister.setName("zoom", 0)
	h.lister.setFailing(true)
	h.poll()
	h.expectNoExit()

	h.lister.setFailing(false)
	h.poll()
	h.expectExit(`process 4242 exited and no process named "zoom" is running anymore`)
}

func TestWatchCancel(t *testing.T) {
	h := newHarness(t)
	h.lister.set(4242, true)
	ctx, cancel := context.WithCancel(context.Background())
	h.start(ctx, []int{4242}, "")
	cancel()
	h.expectClosed()
	if !h.clk.WaitForWaiters(0, time.Second) || h.clk.Waiters() != 0 {
		t.Fatal("ticker still armed after cancel")
	}
}

func TestWatchImmediateErrors(t *testing.T) {
	h := newHarness(t)
	h.lister.set(1, true)
	h.lister.setName("zoom", 1)
	tests := []struct {
		pids    []int
		name    string
		want    string
		running bool // errors.Is(err, ErrNotRunning)
	}{
		{[]int{4242}, "", "process 4242 is not running", true},
		{[]int{4242, 4243}, "", "none of the processes 4242, 4243 are running", true},
		{nil, "slack", `no process named "slack" is running`, true},
		{[]int{1}, "slack", `no process named "slack" is running`, true},
		{[]int{4242}, "zoom", "process 4242 is not running", true},
		{nil, "", "proc: nothing to watch: give a process ID or a process name", false},
		{[]int{0}, "", "proc: invalid process ID 0", false},
		{[]int{-5}, "zoom", "proc: invalid process ID -5", false},
	}
	for _, tt := range tests {
		exits, err := h.watcher().Watch(context.Background(), tt.pids, tt.name)
		if err == nil {
			t.Fatalf("Watch(%v, %q) = %v, want error %q", tt.pids, tt.name, exits, tt.want)
		}
		if err.Error() != tt.want || errors.Is(err, ErrNotRunning) != tt.running {
			t.Errorf("Watch(%v, %q) error = %q (ErrNotRunning %v), want %q (%v)",
				tt.pids, tt.name, err, errors.Is(err, ErrNotRunning), tt.want, tt.running)
		}
	}
	if n := h.clk.Waiters(); n != 0 {
		t.Fatalf("a failed Watch left %d timers armed", n)
	}

	h.lister.setFailing(true)
	if _, err := h.watcher().Watch(context.Background(), []int{1}, ""); err == nil || errors.Is(err, ErrNotRunning) {
		t.Fatalf("Watch with a failing lookup = %v, want the lookup error", err)
	}

	if _, err := NewWatcher(h.clk, 0, h.lister).Watch(context.Background(), []int{1}, ""); err == nil {
		t.Fatal("Watch with a zero interval gave no error")
	}
}

// TestWatchRealProcess runs the real lister and clock against a child process.
func TestWatchRealProcess(t *testing.T) {
	cmd := helper(t, "sleep")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exits, err := Watch(ctx, clock.Real(), 20*time.Millisecond, []int{cmd.Process.Pid}, "")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ex := <-exits:
		t.Fatalf("exit while the child runs: %+v", ex)
	case <-time.After(100 * time.Millisecond):
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case ex := <-exits:
		if want := "process " + strconv.Itoa(cmd.Process.Pid) + " exited"; ex.Reason != want {
			t.Fatalf("Exit.Reason = %q, want %q", ex.Reason, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no Exit after the child was killed")
	}
}

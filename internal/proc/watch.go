package proc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/clock"
)

// ErrNotRunning is matched (errors.Is) by the error Watch returns when none
// of the watched processes is running at the start. Its text is meant for
// the user, e.g. `no process named "zoom" is running`.
var ErrNotRunning = errors.New("process not running")

type notRunningError struct{ msg string }

func (e *notRunningError) Error() string        { return e.msg }
func (e *notRunningError) Is(target error) bool { return target == ErrNotRunning }

// Exit tells why a watch ended, e.g. "process 4242 exited" or
// `no process named "zoom" is running anymore`.
type Exit struct{ Reason string }

// Watcher polls a Lister on a clock.
type Watcher struct {
	clk      clock.Clock
	interval time.Duration
	lister   Lister
	polled   func() // test hook, called after every poll
}

// NewWatcher returns a Watcher that polls l every interval on clk.
func NewWatcher(clk clock.Clock, interval time.Duration, l Lister) *Watcher {
	return &Watcher{clk: clk, interval: interval, lister: l}
}

// Watch watches the real processes; see Watcher.Watch.
func Watch(ctx context.Context, clk clock.Clock, interval time.Duration, pids []int, name string) (<-chan Exit, error) {
	return NewWatcher(clk, interval, System()).Watch(ctx, pids, name)
}

// Watch watches until none of the processes is left: every pid in pids has
// exited and, when name is set, no process matching name is running. The
// returned channel then delivers exactly one Exit and is closed; when ctx is
// cancelled first it is closed without an Exit.
//
// Watch fails at once, with an error matching ErrNotRunning, when pids are
// given and none of them is alive, or when name is given and nothing matches
// it. A pid that is seen dead is never checked again, so a reused pid does
// not count. A name is checked on every poll, so a process that starts again
// under that name keeps the watch going while a pid is still alive. A failed
// lookup during polling counts as still running.
func (w *Watcher) Watch(ctx context.Context, pids []int, name string) (<-chan Exit, error) {
	if w.interval <= 0 {
		return nil, fmt.Errorf("proc: watch interval must be positive, got %v", w.interval)
	}
	if len(pids) == 0 && name == "" {
		return nil, errors.New("proc: nothing to watch: give a process ID or a process name")
	}
	for _, pid := range pids {
		if pid <= 0 {
			return nil, fmt.Errorf("proc: invalid process ID %d", pid)
		}
	}
	pids = slices.Compact(slices.Sorted(slices.Values(pids)))

	var live []int
	for _, pid := range pids {
		ok, err := w.lister.Alive(pid)
		if err != nil {
			return nil, err
		}
		if ok {
			live = append(live, pid)
		}
	}
	if len(pids) > 0 && len(live) == 0 {
		if len(pids) == 1 {
			return nil, &notRunningError{fmt.Sprintf("process %d is not running", pids[0])}
		}
		return nil, &notRunningError{fmt.Sprintf("none of the processes %s are running", joinPIDs(pids))}
	}
	present := false
	if name != "" {
		procs, err := w.lister.FindByName(name)
		if err != nil {
			return nil, err
		}
		if len(procs) == 0 {
			return nil, &notRunningError{fmt.Sprintf("no process named %q is running", name)}
		}
		present = true
	}

	exits := make(chan Exit, 1)
	ticker := w.clk.NewTicker(w.interval)
	go w.loop(ctx, ticker, exits, live, name, present)
	return exits, nil
}

func (w *Watcher) loop(ctx context.Context, ticker clock.Ticker, exits chan<- Exit, live []int, name string, present bool) {
	defer close(exits)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
		}

		var gone []int
		kept := live[:0]
		for _, pid := range live {
			ok, err := w.lister.Alive(pid)
			if err != nil {
				slog.Debug("process watch: lookup failed", "pid", pid, "err", err)
				ok = true
			}
			if ok {
				kept = append(kept, pid)
			} else {
				gone = append(gone, pid)
			}
		}
		live = kept

		nameGone := false
		if name != "" {
			procs, err := w.lister.FindByName(name)
			now := len(procs) > 0
			if err != nil {
				slog.Debug("process watch: lookup failed", "name", name, "err", err)
				now = present
			}
			nameGone = present && !now
			present = now
		}

		done := len(live) == 0 && !present && ctx.Err() == nil
		if done {
			exits <- Exit{Reason: exitReason(gone, name, nameGone)}
		}
		if w.polled != nil {
			w.polled()
		}
		if done {
			return
		}
	}
}

func exitReason(gone []int, name string, nameGone bool) string {
	var parts []string
	switch len(gone) {
	case 0:
	case 1:
		parts = append(parts, fmt.Sprintf("process %d exited", gone[0]))
	default:
		parts = append(parts, fmt.Sprintf("processes %s exited", joinPIDs(gone)))
	}
	if nameGone {
		parts = append(parts, fmt.Sprintf("no process named %q is running anymore", name))
	}
	return strings.Join(parts, " and ")
}

func joinPIDs(pids []int) string {
	s := make([]string, len(pids))
	for i, pid := range pids {
		s[i] = strconv.Itoa(pid)
	}
	return strings.Join(s, ", ")
}

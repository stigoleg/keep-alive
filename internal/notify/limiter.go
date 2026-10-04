package notify

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileLimiter shows each kind of notification at most once per Interval and
// remembers when in a small JSON file, so a process that is started again
// and again (a failing login service) does not repeat it on every start.
type FileLimiter struct {
	Path     string
	Interval time.Duration

	mu sync.Mutex
}

// Allow reports whether a notification of kind may be shown at now and, if
// so, records it. A state file that cannot be read or written never holds a
// notification back; one dated in the future (the clock was set back) is
// ignored.
func (l *FileLimiter) Allow(kind string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	last := map[string]time.Time{}
	if data, err := os.ReadFile(l.Path); err == nil {
		if err := json.Unmarshal(data, &last); err != nil {
			slog.Debug("notify: ignoring unreadable rate-limit state", "path", l.Path, "err", err)
			last = map[string]time.Time{}
		}
	}
	if t, ok := last[kind]; ok {
		if d := now.Sub(t); d >= 0 && d < l.Interval {
			return false
		}
	}
	last[kind] = now.Round(0)
	if err := l.write(last); err != nil {
		slog.Debug("notify: cannot save rate-limit state", "path", l.Path, "err", err)
	}
	return true
}

// write replaces the state file atomically.
func (l *FileLimiter) write(last map[string]time.Time) error {
	data, err := json.Marshal(last)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.Path), filepath.Base(l.Path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), l.Path)
}

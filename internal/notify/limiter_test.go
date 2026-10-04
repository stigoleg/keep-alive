package notify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestFileLimiterPersistsAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "notified.json")
	t0 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	limiter := func() *FileLimiter { return &FileLimiter{Path: path, Interval: 10 * time.Minute} }

	if !limiter().Allow("stopped", t0) {
		t.Fatal("first notification held back")
	}
	// A new process (a restarted service) within the interval.
	if limiter().Allow("stopped", t0.Add(9*time.Minute)) {
		t.Fatal("repeated within 10 minutes after a restart")
	}
	if !limiter().Allow("power", t0.Add(time.Minute)) {
		t.Fatal("another kind held back")
	}
	if !limiter().Allow("stopped", t0.Add(10*time.Minute)) {
		t.Fatal("held back after the interval")
	}
	if fi, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Fatalf("state file: %v, %v", fi, err)
	}
	// The clock was set back: never silence notifications for good.
	if !limiter().Allow("stopped", t0.Add(-time.Hour)) {
		t.Fatal("held back by a record from the future")
	}
}

func TestFileLimiterNeverBlocksOnBadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notified.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := &FileLimiter{Path: path, Interval: time.Hour}
	now := time.Now()
	if !l.Allow("stopped", now) {
		t.Fatal("corrupt state held a notification back")
	}
	if l.Allow("stopped", now) {
		t.Fatal("corrupt state was not replaced")
	}
	unwritable := &FileLimiter{Path: filepath.Join(path, "below-a-file"), Interval: time.Hour}
	if !unwritable.Allow("stopped", now) || !unwritable.Allow("stopped", now) {
		t.Fatal("an unwritable state file held a notification back")
	}
}

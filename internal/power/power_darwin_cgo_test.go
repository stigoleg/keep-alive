//go:build darwin && cgo

package power

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIOPMHoldVisibleInPmset(t *testing.T) {
	if testing.Short() {
		t.Skip("holds real power assertions")
	}
	pid := os.Getpid()
	if got := assertionsOf(t, pid); len(got) != 0 {
		t.Fatalf("test process already holds %v", got)
	}
	for _, display := range []bool{true, false} {
		h, err := New().Acquire(context.Background(), Options{KeepDisplay: display, Reason: "test"})
		if err != nil {
			t.Fatalf("Acquire(display=%v): %v", display, err)
		}
		checkHoldTypes(t, pid, display)
		if d := h.Describe(); !strings.HasPrefix(d, "IOPMAssertion(PreventUserIdleSystemSleep") || strings.Contains(d, "Display") != display {
			t.Errorf("Describe() = %q (display=%v)", d, display)
		}
		if err := h.Release(); err != nil {
			t.Fatalf("Release: %v", err)
		}
		if err := h.Release(); err != nil {
			t.Fatalf("second Release: %v", err)
		}
		if got := waitAssertions(t, pid, 2*time.Second, isEmpty); len(got) != 0 {
			t.Fatalf("still listed after Release (display=%v): %v", display, got)
		}
	}
}

func TestIOPMReleasedWhenKilled(t *testing.T) { testKilledOwnerReleases(t, "iopm") }

func TestAcquireHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Acquire(ctx, Options{}); err == nil {
		t.Fatal("Acquire with a cancelled context succeeded")
	}
}

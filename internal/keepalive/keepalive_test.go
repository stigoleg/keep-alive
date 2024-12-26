package keepalive

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestKeepAlive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Add test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Kill any existing caffeinate processes
	if runtime.GOOS == "darwin" {
		exec.Command("pkill", "-9", "caffeinate").Run()
	}

	// Cleanup after test
	t.Cleanup(func() {
		if runtime.GOOS == "darwin" {
			exec.Command("pkill", "-9", "caffeinate").Run()
		}
	})

	t.Run("Basic Operations", func(t *testing.T) {
		k := &Keeper{}
		defer k.Stop() // Ensure cleanup even if test fails

		if k.IsRunning() {
			t.Fatal("expected not running at start")
		}

		// Start indefinite
		err := k.StartIndefinite()
		if err != nil && err.Error() == "unsupported platform" {
			t.Skip("Skipping on unsupported platform")
		}
		if err != nil {
			t.Fatalf("StartIndefinite failed: %v", err)
		}
		if !k.IsRunning() {
			t.Fatal("expected running after StartIndefinite")
		}

		// Wait for context or short timeout
		select {
		case <-ctx.Done():
			t.Fatal("test timeout")
		case <-time.After(200 * time.Millisecond):
		}

		// Stop
		err = k.Stop()
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
		if k.IsRunning() {
			t.Fatal("expected not running after Stop")
		}
	})

	t.Run("Timed Operation", func(t *testing.T) {
		k := &Keeper{}
		defer k.Stop() // Ensure cleanup even if test fails

		// Start timed
		err := k.StartTimed(2 * time.Second)
		if err != nil && err.Error() == "unsupported platform" {
			t.Skip("Skipping on unsupported platform")
		}
		if err != nil {
			t.Fatalf("StartTimed failed: %v", err)
		}
		if !k.IsRunning() {
			t.Fatal("expected running after StartTimed")
		}

		// Wait for context or short timeout
		select {
		case <-ctx.Done():
			t.Fatal("test timeout")
		case <-time.After(200 * time.Millisecond):
		}

		// Stop
		err = k.Stop()
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
		if k.IsRunning() {
			t.Fatal("expected not running after Stop")
		}
	})
}

package keepalive

import (
	"testing"
	"time"
)

func TestKeepAlive(t *testing.T) {
	k := &Keeper{}
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

	// Stop
	err = k.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if k.IsRunning() {
		t.Fatal("expected not running after Stop")
	}

	// Start timed
	err = k.StartTimed(1)
	if err != nil && err.Error() == "unsupported platform" {
		t.Skip("Skipping on unsupported platform")
	}
	if err != nil {
		t.Fatalf("StartTimed failed: %v", err)
	}
	if !k.IsRunning() {
		t.Fatal("expected running after StartTimed")
	}
	time.Sleep(100 * time.Millisecond)
	err = k.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if k.IsRunning() {
		t.Fatal("expected not running after Stop")
	}
}

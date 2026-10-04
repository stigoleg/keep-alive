package power

import (
	"context"
	"testing"
)

func TestLegacyHoldReleasesOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("acquires a real power assertion")
	}
	hold, err := New().Acquire(context.Background(), Options{KeepDisplay: true, Reason: "test"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if hold.Describe() == "" {
		t.Error("Describe() is empty")
	}
	if err := hold.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := hold.Release(); err == nil {
		t.Fatal("second Release succeeded")
	}
}

func TestAcquireHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Acquire(ctx, Options{}); err == nil {
		t.Fatal("Acquire with a cancelled context succeeded")
	}
}

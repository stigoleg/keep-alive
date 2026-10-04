package legacy

import (
	"context"
	"testing"

	"github.com/stigoleg/keep-alive/v2/internal/platform"
)

type fakeKeepAlive struct {
	starts, stops int
	simulate      []bool
}

func (f *fakeKeepAlive) Start(context.Context) error { f.starts++; return nil }
func (f *fakeKeepAlive) Stop() error                 { f.stops++; return nil }
func (f *fakeKeepAlive) SetSimulateActivity(on bool) { f.simulate = append(f.simulate, on) }
func (f *fakeKeepAlive) lastSimulate() (bool, bool) {
	if len(f.simulate) == 0 {
		return false, false
	}
	return f.simulate[len(f.simulate)-1], true
}

func useFake(t *testing.T) *fakeKeepAlive {
	t.Helper()
	fake := &fakeKeepAlive{}
	orig := newKeepAlive
	newKeepAlive = func() (platform.KeepAlive, error) { return fake, nil }
	t.Cleanup(func() {
		newKeepAlive = orig
		shared = object{}
	})
	return fake
}

func TestSharedObjectIsRefCounted(t *testing.T) {
	fake := useFake(t)

	if err := Acquire(); err != nil {
		t.Fatal(err)
	}
	if err := Acquire(); err != nil {
		t.Fatal(err)
	}
	if fake.starts != 1 {
		t.Fatalf("starts = %d, want 1", fake.starts)
	}
	if err := Release(); err != nil {
		t.Fatal(err)
	}
	if fake.stops != 0 {
		t.Fatalf("stopped while still referenced")
	}
	if err := Release(); err != nil {
		t.Fatal(err)
	}
	if fake.stops != 1 {
		t.Fatalf("stops = %d, want 1", fake.stops)
	}
	if err := Release(); err == nil {
		t.Fatal("Release without Acquire succeeded")
	}
}

func TestActivityUsersToggleSimulationOnce(t *testing.T) {
	fake := useFake(t)

	if err := Acquire(); err != nil {
		t.Fatal(err)
	}
	SetActivity(true)
	SetActivity(true) // overlapping simulator during a quick off/on toggle
	SetActivity(false)
	if on, _ := fake.lastSimulate(); !on {
		t.Fatal("simulation disabled while a user remains")
	}
	SetActivity(false)
	if on, ok := fake.lastSimulate(); !ok || on {
		t.Fatal("simulation not disabled after last user left")
	}
	if err := Release(); err != nil {
		t.Fatal(err)
	}
}

func TestStartAppliesPendingActivity(t *testing.T) {
	fake := useFake(t)

	SetActivity(true)
	if err := Acquire(); err != nil {
		t.Fatal(err)
	}
	if on, _ := fake.lastSimulate(); !on {
		t.Fatal("activity requested before start was not applied")
	}
	SetActivity(false)
	if err := Release(); err != nil {
		t.Fatal(err)
	}
}

package platform

import (
	"errors"
	"testing"
	"time"
)

func TestActivityObserverSeesControllerTransitions(t *testing.T) {
	var kinds []ActivityEventKind
	SetActivityObserver(func(ev ActivityEvent) { kinds = append(kinds, ev.Kind) })
	defer SetActivityObserver(nil)

	ac := NewActivityController("test", NewMousePatternGenerator(newCryptoSeededRand()))
	noop := func([]MousePoint, time.Duration) {}

	ac.MaybeJitter(func() (time.Duration, error) { return 0, errors.New("boom") }, noop)
	ac.MaybeJitter(func() (time.Duration, error) { return time.Second, nil }, noop)

	// Pretend the user went idle long ago so the next call jitters.
	ac.lastUserActiveNS = time.Now().Add(-time.Hour).UnixNano()
	if !ac.MaybeJitter(func() (time.Duration, error) { return time.Hour, nil }, noop) {
		t.Fatal("expected a jitter burst")
	}

	want := []ActivityEventKind{ActivityIdleUnknown, ActivityWaitingIdle, ActivityBurst}
	if len(kinds) != len(want) {
		t.Fatalf("events = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("events = %v, want %v", kinds, want)
		}
	}
}

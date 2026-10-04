package activity

import (
	"errors"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/platform"
)

func TestLegacyStatusMapsControllerEvents(t *testing.T) {
	base := Status{Method: "test"}
	at := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		ev   platform.ActivityEvent
		want State
	}{
		{platform.ActivityEvent{Kind: platform.ActivityIdleUnknown, Err: errors.New("x")}, StateDegraded},
		{platform.ActivityEvent{Kind: platform.ActivityWaitingIdle}, StateWaitingIdle},
		{platform.ActivityEvent{Kind: platform.ActivityUserReturned}, StatePausedUser},
		{platform.ActivityEvent{Kind: platform.ActivityBurst, At: at}, StateSimulating},
	}
	for _, tt := range tests {
		got := legacyStatus(base, tt.ev)
		if got.State != tt.want || got.Method != "test" {
			t.Errorf("legacyStatus(%v) = %+v, want state %s", tt.ev.Kind, got, tt.want)
		}
	}
	if got := legacyStatus(base, platform.ActivityEvent{Kind: platform.ActivityBurst, At: at}); !got.LastBurst.Equal(at) {
		t.Errorf("LastBurst = %v, want %v", got.LastBurst, at)
	}
}

func TestLegacyHintOnlyForCustomSettings(t *testing.T) {
	def := Config{IdleThreshold: DefaultIdleThreshold, Interval: DefaultInterval}
	if h := legacyHint(def); h != "" {
		t.Fatalf("hint for defaults = %q, want empty", h)
	}
	def.Keys = true
	if h := legacyHint(def); h == "" {
		t.Fatal("expected a hint for --active-keys")
	}
}

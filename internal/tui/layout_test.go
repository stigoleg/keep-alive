package tui

import (
	"testing"
	"time"
)

func TestSpan(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                                 "0s",
		12 * time.Second:                  "12s",
		70 * time.Second:                  "1m 10s",
		2 * time.Minute:                   "2m",
		65 * time.Second:                  "1m 05s",
		72 * time.Minute:                  "1h 12m",
		125 * time.Minute:                 "2h 05m",
		2*time.Hour + 30*time.Second:      "2h 00m",
		-time.Second:                      "0s",
		26*time.Hour + 5*time.Minute + 59: "26h 05m",
	} {
		if got := span(d); got != want {
			t.Errorf("span(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestShortDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		2 * time.Hour:    "2h",
		90 * time.Minute: "1h30m",
		45 * time.Minute: "45m",
		90 * time.Second: "1m30s",
		30 * time.Second: "30s",
		10 * time.Hour:   "10h",
	} {
		if got := shortDuration(d); got != want {
			t.Errorf("shortDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

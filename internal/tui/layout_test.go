package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
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

func TestCanvasWidths(t *testing.T) {
	st := NewStyles(nil, Look{})
	for _, tc := range []struct {
		term          int
		frame, framed bool
		total, width  int
	}{
		{0, true, true, 64, 60},
		{100, true, true, 64, 60},
		{64, true, true, 64, 60},
		{48, true, true, 48, 44},
		{47, true, false, 47, 45},
		{40, true, false, 40, 38},
		{64, false, false, 64, 61},
	} {
		c := newCanvas(st, tc.term, fitFull, tc.frame)
		if c.framed != tc.framed || c.total != tc.total || c.width != tc.width {
			t.Errorf("terminal %d frame %v: framed %v total %d width %d", tc.term, tc.frame, c.framed, c.total, c.width)
		}
		c.add(strings.Repeat("x", 200))
		c.keyHints(keyHint{"a", strings.Repeat("y", 200), ""})
		for _, l := range strings.Split(c.String(), "\n") {
			if lipgloss.Width(l) > c.total {
				t.Errorf("terminal %d: %q is %d wide", tc.term, l, lipgloss.Width(l))
			}
		}
		if got := strings.Count(c.String(), "\n") + 1; got != c.height() {
			t.Errorf("terminal %d: %d lines, height says %d", tc.term, got, c.height())
		}
	}
	c := newCanvas(st, 64, fitNoSpacers, true)
	c.spacer()
	if len(c.lines) != 0 {
		t.Fatal("a spacer on a short terminal")
	}
}

package tui

import (
	"strings"
	"testing"
)

func TestFitDropsInOrderUntilItFits(t *testing.T) {
	// A screen of 10 lines that loses one line per level.
	build := func(level int) string { return strings.Repeat("x\n", 9-level) + "x" }
	for _, tc := range []struct{ height, level int }{
		{0, fitFull}, {30, fitFull}, {10, fitFull}, {9, fitNoSpacers}, {8, fitNoHolding},
		{7, fitNoSparkline}, {6, fitNoNote}, {3, fitNoNote},
	} {
		m := Model{height: tc.height}
		if got := strings.Count(m.fit(build), "\n") + 1; got != 10-tc.level {
			t.Errorf("height %d: %d lines, want %d", tc.height, got, 10-tc.level)
		}
	}
}

package tui

import "strings"

// What a terminal too short for a screen leaves out, in order; each level
// also leaves out what the levels before it do.
const (
	fitFull        = iota
	fitNoSpacers   // blank spacer lines
	fitNoHolding   // the HOLDING row
	fitNoSparkline // the sparkline (its state text stays)
	fitNoNote      // a callout's muted line
	fitNoDigits    // the big digits (for one line of text) and callout boxes
	fitLevels
)

// fit draws a screen with build at the first level whose lines fit the
// terminal's height, or at the last level when none does. An unknown
// height fits everything.
func (m Model) fit(build func(level int) string) string {
	var out string
	for level := range fitLevels {
		out = build(level)
		if m.height <= 0 || strings.Count(out, "\n")+1 <= m.height {
			return out
		}
	}
	return out
}

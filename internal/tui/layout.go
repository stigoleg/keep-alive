package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// wrapLines wraps text to width at spaces; continuation lines are indent
// shorter. A hyphen never ends a line ("08:00-16:00" stays whole) unless
// a word is wider than the line.
func wrapLines(text string, width int, indent string) []string {
	width = max(width, 8)
	wrap := func(s string, w int) []string {
		s = strings.ReplaceAll(s, "-", nbHyphen)
		return strings.Split(strings.ReplaceAll(ansi.Wrap(s, w, ""), nbHyphen, "-"), "\n")
	}
	first := wrap(text, width)
	if len(first) <= 1 || indent == "" {
		return first
	}
	rest := strings.Join(first[1:], " ")
	return append(first[:1], wrap(rest, max(width-lipgloss.Width(indent), 8))...)
}

// nbHyphen stands in for "-" while wrapping: ansi.Wrap breaks after every
// hyphen.
const nbHyphen = "\u2011"

// span formats d for people: "12s", "1m 05s", "2h 05m". Longer spans drop
// the seconds.
func span(d time.Duration) string {
	d = max(d.Round(time.Second), 0)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	case m > 0 && s == 0:
		return fmt.Sprintf("%dm", m)
	case m > 0:
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// shortDuration formats d the way it is typed: "2h", "1h30m", "45m".
func shortDuration(d time.Duration) string {
	s := d.Round(time.Second).String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// clockDigits formats d for the big digits: H:MM, or M:SS under an hour
// when seconds is set. Seconds round up, so a countdown shows 0:00 only at
// the end.
func clockDigits(d time.Duration, seconds bool) string {
	d = max(d, 0)
	if r := d % time.Second; r > 0 {
		d += time.Second - r
	}
	if seconds && d < time.Hour {
		return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

// scheduleText is a normalized schedule as people read it:
// "Mon–Fri 08:00–11:30, 12:00–16:00" (ASCII terminals keep the hyphens).
func (st Styles) scheduleText(s string) string {
	if st.g.dash != "–" {
		return s
	}
	return strings.NewReplacer("-", "–", ",", ", ").Replace(s)
}

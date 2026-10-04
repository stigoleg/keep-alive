package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Layout widths. The UI is designed for targetWidth columns and stays
// correct down to minWidth by wrapping and truncating; it never draws past
// the terminal's width.
const (
	targetWidth = 64
	minWidth    = 40
	margin      = " " // left margin of every line
)

// page collects the lines of one screen. width is the usable width after
// the margin.
type page struct {
	st    Styles
	width int
	lines []string
}

func newPage(st Styles, termWidth int) *page {
	w := termWidth
	if w <= 0 || w > targetWidth {
		w = targetWidth
	}
	// One column of slack on the right keeps terminals from wrapping a
	// line that ends exactly at the edge.
	return &page{st: st, width: max(w-len(margin)-1, 1)}
}

// add appends lines; each is truncated to the width.
func (p *page) add(lines ...string) {
	for _, l := range lines {
		p.lines = append(p.lines, margin+ansi.Truncate(l, p.width, "…"))
	}
}

func (p *page) blank() { p.lines = append(p.lines, "") }

// wrapped adds text wrapped to the width in style; continuation lines get
// indent.
func (p *page) wrapped(text, indent string, style lipgloss.Style) {
	for i, l := range wrapLines(text, p.width, indent) {
		if i > 0 {
			l = indent + l
		}
		p.add(style.Render(l))
	}
}

// header puts left and right on one line, right-aligned, or on two lines
// when they do not fit.
func (p *page) header(left, right string) {
	gap := p.width - lipgloss.Width(left) - lipgloss.Width(right)
	if right == "" {
		p.add(left)
		return
	}
	if gap < 2 {
		p.add(left, right)
		return
	}
	p.add(left + strings.Repeat(" ", gap) + right)
}

// footer joins items with " · " and breaks the line between items when it
// would be too wide.
func (p *page) footer(items ...string) {
	sep := " · "
	line := ""
	for _, it := range items {
		switch {
		case line == "":
			line = it
		case lipgloss.Width(line)+len(sep)+lipgloss.Width(it) <= p.width:
			line += sep + it
		default:
			p.add(p.st.Muted.Render(line + " ·"))
			line = it
		}
	}
	if line != "" {
		p.add(p.st.Muted.Render(line))
	}
}

// notice adds a "! text" or "✗ text" block with an optional "fix:" line,
// wrapped with a hanging indent.
func (p *page) notice(mark string, style lipgloss.Style, text, fix string) {
	for i, l := range wrapLines(text, p.width-2, "") {
		if i == 0 {
			p.add(style.Render(mark) + " " + l)
		} else {
			p.add("  " + l)
		}
	}
	if fix != "" {
		for i, l := range wrapLines(fix, p.width-7, "") {
			if i == 0 {
				p.add("  " + p.st.Muted.Render("fix:") + " " + l)
			} else {
				p.add("       " + l)
			}
		}
	}
}

func (p *page) String() string { return strings.Join(p.lines, "\n") }

// wrapLines wraps text to width; continuation lines are indent shorter.
func wrapLines(text string, width int, indent string) []string {
	width = max(width, 8)
	first := strings.Split(ansi.Wrap(text, width, ""), "\n")
	if len(first) <= 1 || indent == "" {
		return first
	}
	rest := strings.Join(first[1:], " ")
	return append(first[:1], strings.Split(ansi.Wrap(rest, max(width-lipgloss.Width(indent), 8), ""), "\n")...)
}

// row lays out label, value and an optional key: the label padded to
// labelW, the value truncated to what is left, the key at the right edge.
func row(width int, label string, labelW int, value, key string) string {
	l := label + strings.Repeat(" ", max(labelW-lipgloss.Width(label), 0))
	room := width - lipgloss.Width(l)
	if key != "" {
		room -= lipgloss.Width(key) + 2
	}
	v := ""
	if value != "" && room >= 6 {
		v = ansi.Truncate(value, room, "…")
	}
	line := l + v
	if key != "" {
		line += strings.Repeat(" ", max(width-lipgloss.Width(line)-lipgloss.Width(key), 1)) + key
	}
	return strings.TrimRight(line, " ")
}

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

// progressBar draws done (0..1) as a bar of width cells.
func progressBar(st Styles, done float64, width int) string {
	done = min(max(done, 0), 1)
	full := int(done*float64(width) + 0.5)
	return st.Accent.Render(strings.Repeat("█", full)) + st.Muted.Render(strings.Repeat("░", width-full))
}

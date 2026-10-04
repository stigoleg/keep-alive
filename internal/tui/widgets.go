package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The building blocks of every screen. Each returns finished, styled text
// of a known width; the screens only arrange them.

type pillKind int

const (
	pillOK pillKind = iota
	pillWarn
	pillBad
	pillMuted
	pillCount
)

// pill is a status badge: " ● AWAKE " on a coloured background, or
// "[AWAKE]" without colours.
func (st Styles) pill(k pillKind, symbol, text string) string {
	if !st.color {
		return st.pills[k].Render("[" + text + "]")
	}
	return st.pills[k].Render(" " + symbol + " " + text + " ")
}

// keycap is a key as a chip: " a " on the keycap colour, or "[a]".
func (st Styles) keycap(key string) string {
	key = st.text(key)
	if !st.color {
		return "[" + key + "]"
	}
	return st.Key.Render(" " + key + " ")
}

// keyHint is a key and what it does; short is the label for narrow
// terminals ("" keeps label).
type keyHint struct{ key, label, short string }

// keyLines lays out keys as keycaps with muted labels, two spaces apart, in
// lines of at most width cells: on one line with the full labels when they
// fit, else with the short ones, else over as many lines as needed.
func (st Styles) keyLines(width int, keys []keyHint) []string {
	const gap = "  "
	render := func(short bool) []string {
		items := make([]string, len(keys))
		for i, k := range keys {
			label := k.label
			if short && k.short != "" {
				label = k.short
			}
			items[i] = st.keycap(k.key) + " " + st.Muted.Render(st.text(label))
		}
		return items
	}
	fits := func(items []string) bool {
		return lipgloss.Width(strings.Join(items, gap)) <= width
	}
	items := render(false)
	if fits(items) {
		return []string{strings.Join(items, gap)}
	}
	items = render(true)
	if fits(items) {
		return []string{strings.Join(items, gap)}
	}
	var lines []string
	line := ""
	for _, it := range items {
		it = ansi.Truncate(it, width, "")
		switch {
		case line == "":
			line = it
		case lipgloss.Width(line)+len(gap)+lipgloss.Width(it) <= width:
			line += gap + it
		default:
			lines = append(lines, line)
			line = it
		}
	}
	return append(lines, line)
}

// box frames lines in a rounded border width cells wide, drawn in tone,
// with one column of padding: each line may be width-4 cells wide and is
// truncated beyond that.
func (st Styles) box(tone lipgloss.Style, width int, lines []string) []string {
	g := st.g
	width = max(width, 5)
	inner := width - 4
	out := make([]string, 0, len(lines)+2)
	out = append(out, tone.Render(g.tl+strings.Repeat(g.h, width-2)+g.tr))
	side := tone.Render(g.v)
	for _, l := range lines {
		l = ansi.Truncate(l, inner, st.g.ellipsis)
		out = append(out, side+" "+l+strings.Repeat(" ", inner-lipgloss.Width(l))+" "+side)
	}
	return append(out, tone.Render(g.bl+strings.Repeat(g.h, width-2)+g.br))
}

// calloutText is what a callout box says: text, then an optional fix and
// a muted note.
type calloutText struct {
	text, fix, note string
}

// callout is a box in tone, width cells wide: the text, a blank line, "Fix"
// and the fix with a hanging indent, then the note in muted. spacer false
// leaves out the blank line and note false the note (short terminals).
func (st Styles) callout(tone lipgloss.Style, width int, c calloutText, spacer, note bool) []string {
	inner := max(width-4, 8)
	var lines []string
	lines = append(lines, wrapLines(st.text(c.text), inner, "")...)
	if c.fix != "" {
		if spacer {
			lines = append(lines, "")
		}
		const label = "Fix  "
		for i, l := range wrapLines(st.text(c.fix), inner-len(label), "") {
			if i == 0 {
				lines = append(lines, st.Bold.Render("Fix")+"  "+l)
			} else {
				lines = append(lines, strings.Repeat(" ", len(label))+l)
			}
		}
	}
	if c.note != "" && note {
		for _, l := range wrapLines(st.text(c.note), inner, "") {
			lines = append(lines, st.Muted.Render(l))
		}
	}
	return st.box(tone, width, lines)
}

// seg is a piece of text in a style from Styles (never a bare
// lipgloss.Style, which would query the terminal), for rows drawn on a
// background.
type seg struct {
	text  string
	style lipgloss.Style
}

// band draws a row on the selection band: left and right segments spread
// over width cells on the band colour. Without colours the row is drawn
// as it is.
func (st Styles) band(width int, left, right []seg) string {
	draw := func(ss []seg) string {
		var b strings.Builder
		for _, s := range ss {
			style := s.style
			if st.color {
				style = style.Inherit(st.Band)
			}
			b.WriteString(style.Render(st.text(s.text)))
		}
		return b.String()
	}
	l, r := draw(left), draw(right)
	gap := max(width-lipgloss.Width(l)-lipgloss.Width(r), 1)
	return l + st.Band.Render(strings.Repeat(" ", gap)) + r
}

// spread puts left and right on one line of width cells, right-aligned;
// left is truncated when both do not fit.
func (st Styles) spread(width int, left, right string) string {
	rw := lipgloss.Width(right)
	if rw == 0 {
		return ansi.Truncate(left, width, st.g.ellipsis)
	}
	if lipgloss.Width(left)+1+rw > width {
		left = ansi.Truncate(left, max(width-rw-1, 0), st.g.ellipsis)
	}
	return left + strings.Repeat(" ", max(width-lipgloss.Width(left)-rw, 1)) + right
}

// filled is how many of width cells done (0..1) fills.
func filled(done float64, width int) int {
	done = min(max(done, 0), 1)
	return min(int(done*float64(width)+0.5), width)
}

// cells is s repeated n times, as a slice of cells.
func cells(s string, n int) []string {
	out := make([]string, max(n, 0))
	for i := range out {
		out[i] = s
	}
	return out
}

// blockBar is a progress bar of width cells: the done part █ in the
// gradient, the rest █ in the track colour. Without colours: █ and ░.
func (st Styles) blockBar(done float64, width int) string {
	n := filled(done, width)
	bar := st.paint(cells(st.g.block, n), st.gradientStops(gradDark, gradLight), false)
	if !st.color {
		return bar + st.Track.Render(strings.Repeat(st.g.shade, width-n))
	}
	return bar + st.Track.Render(strings.Repeat(st.g.block, width-n))
}

// lineBar is a thin progress line of width cells: the done part ━ in the
// gradient, the rest ─ in the frame colour.
func (st Styles) lineBar(done float64, width int) string {
	n := filled(done, width)
	return st.paint(cells(st.g.thick, n), st.gradientStops(gradDark, gradLight), false) +
		st.Frame.Render(strings.Repeat(st.g.thin, width-n))
}

// meter is a level of width cells, such as the battery: the done part ━
// in tone, the rest ━ in the track colour. Without colours the rest is ─.
func (st Styles) meter(done float64, width int, tone lipgloss.Style) string {
	n := filled(done, width)
	rest := st.g.thick
	if !st.color {
		rest = st.g.thin
	}
	return tone.Render(strings.Repeat(st.g.thick, n)) + st.Track.Render(strings.Repeat(rest, width-n))
}

// sparkline draws counts, oldest first, one cell each: ▁ in the dim colour
// for none, else ▂ to █ scaled to the largest count (at least 2, so a
// lone burst stands half high), coloured teal to green.
func (st Styles) sparkline(counts []int) string {
	top := 2
	for _, c := range counts {
		top = max(top, c)
	}
	cols := gradient(len(counts), st.gradientStops(sparkDark, sparkLight))
	var b strings.Builder
	for i, c := range counts {
		if c <= 0 {
			b.WriteString(st.Dim.Render(st.g.spark[0]))
			continue
		}
		level := min(max((c*7+top-1)/top, 1), 7)
		cell := st.g.spark[level]
		if st.color {
			cell = st.r.NewStyle().Foreground(lipgloss.Color(cols[i])).Render(cell)
		}
		b.WriteString(cell)
	}
	return b.String()
}

// bigFont is a 3-row block font for countdowns: 'F' is a full block, 'U'
// an upper half block. Digits are 3 cells wide, ':' one.
var bigFont = map[rune][3]string{
	'0': {"FUF", "F F", "UUU"}, '1': {"UF ", " F ", "UUU"}, '2': {"UUF", "FUU", "UUU"},
	'3': {"UUF", "UUF", "UUU"}, '4': {"F F", "UUF", "  U"}, '5': {"FUU", "UUF", "UUU"},
	'6': {"FUU", "FUF", "UUU"}, '7': {"UUF", "  F", "  U"}, '8': {"FUF", "FUF", "UUU"},
	'9': {"FUF", "UUF", "UUU"}, ':': {" ", "U", "U"},
}

// bigText draws text (digits and ':') in the block font, characters one
// cell apart; other characters are left out.
func (st Styles) bigText(text string) [3]string {
	var rows [3]string
	repl := strings.NewReplacer("F", st.g.full, "U", st.g.upper)
	first := true
	for _, r := range text {
		glyph, ok := bigFont[r]
		if !ok {
			continue
		}
		for i := range rows {
			if !first {
				rows[i] += " "
			}
			rows[i] += repl.Replace(glyph[i])
		}
		first = false
	}
	return rows
}

// wordmark is "keepalive" in bold, in the gradient.
func (st Styles) wordmark() string {
	return st.paint(strings.Split("keepalive", ""), st.gradientStops(gradDark, gradLight), true)
}

// versionText is "v2.0.0" for a release, the version as it is otherwise.
func versionText(v string) string {
	if v != "" && v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}

// label is an UPPERCASE section label padded to width cells.
func (st Styles) label(text string, width int) string {
	t := strings.ToUpper(text)
	return st.Label.Render(t) + strings.Repeat(" ", max(width-lipgloss.Width(t), 0))
}

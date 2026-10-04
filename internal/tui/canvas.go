package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Screen widths: screens are drawn canvasWidth wide (or as wide as the
// terminal when it is narrower); below frameMinWidth there is no frame.
const (
	canvasWidth   = 64
	frameMinWidth = 48
)

// canvas collects the lines of one screen. With a frame, the content sits
// in a rounded border with one column of padding; without, it gets a
// margin. Key hints go below, outside the frame, indented by one column.
// level says what a short terminal leaves out (see fit).
type canvas struct {
	st     Styles
	level  int
	framed bool
	total  int // the width of the whole screen
	width  int // the content width
	margin int // columns left of the content without a frame
	lines  []string
	keys   []string
}

// newCanvas starts a screen for a terminal termWidth wide (0: unknown).
// frame asks for the frame; a terminal narrower than frameMinWidth gets
// none.
func newCanvas(st Styles, termWidth, level int, frame bool) *canvas {
	w := termWidth
	if w <= 0 || w > canvasWidth {
		w = canvasWidth
	}
	c := &canvas{st: st, level: level, total: w}
	switch {
	case frame && w >= frameMinWidth:
		c.framed, c.width = true, w-4
	case w >= frameMinWidth:
		// One column of slack on the right keeps a terminal from wrapping
		// a line that ends exactly at its edge.
		c.margin, c.width = 2, w-3
	default:
		c.margin, c.width = 1, max(w-2, 1)
	}
	return c
}

// add appends lines, each truncated to the content width.
func (c *canvas) add(lines ...string) {
	for _, l := range lines {
		c.lines = append(c.lines, ansi.Truncate(l, c.width, c.st.g.ellipsis))
	}
}

// spacer adds a blank line, unless the terminal is too short for it.
func (c *canvas) spacer() {
	if c.level < fitNoSpacers {
		c.lines = append(c.lines, "")
	}
}

// spread adds left and right on one line, right aligned.
func (c *canvas) spread(left, right string) { c.add(c.st.spread(c.width, left, right)) }

// wrapped adds text wrapped to the content width; continuation lines get
// indent.
func (c *canvas) wrapped(text, indent string, style lipgloss.Style) {
	for i, l := range wrapLines(c.st.text(text), c.width, indent) {
		if i > 0 {
			l = indent + l
		}
		c.add(style.Render(l))
	}
}

// keyHints sets the key footer.
func (c *canvas) keyHints(keys ...keyHint) {
	c.keys = c.st.keyLines(c.total-c.keyIndent()-1, keys)
}

// keyIndent is how far the key footer is indented: one column under a
// frame, the margin otherwise.
func (c *canvas) keyIndent() int {
	if c.framed {
		return 1
	}
	return c.margin
}

// height is the number of lines the screen takes.
func (c *canvas) height() int { return strings.Count(c.String(), "\n") + 1 }

func (c *canvas) String() string {
	var out []string
	if c.framed {
		out = c.st.box(c.st.Frame, c.total, c.lines)
	} else {
		pad := strings.Repeat(" ", c.margin)
		for _, l := range c.lines {
			if l == "" {
				out = append(out, "")
			} else {
				out = append(out, pad+l)
			}
		}
	}
	if !c.framed && len(c.keys) > 0 && len(c.lines) > 0 && c.level < fitNoSpacers {
		out = append(out, "") // a frame separates the keys; without one, a blank line does
	}
	for _, k := range c.keys {
		out = append(out, strings.Repeat(" ", c.keyIndent())+k)
	}
	return strings.Join(out, "\n")
}

// labelWidth is the width of the dashboard's label column.
const labelWidth = 10

// header adds the wordmark on the left and right on the right; right is
// cut short when both do not fit (a long development version).
func (c *canvas) header(right string) {
	mark := c.st.wordmark()
	if room := c.width - lipgloss.Width(mark) - 2; lipgloss.Width(right) > room {
		right = ansi.Truncate(right, max(room, 0), c.st.g.ellipsis)
	}
	c.spread(mark, right)
}

// fits reports whether a labelled row of value and right takes one line.
func (c *canvas) fits(value, right string) bool {
	return labelWidth+lipgloss.Width(value)+gapFor(right)+lipgloss.Width(right) <= c.width
}

// pick returns the first of values that fits on one labelled row with
// right, or the last.
func (c *canvas) pick(right string, values ...string) string {
	for _, v := range values {
		if c.fits(v, right) {
			return v
		}
	}
	return values[len(values)-1]
}

// labelRow adds an UPPERCASE label, its value and an optional right-aligned
// note. When they do not fit on one line, the value wraps under itself and
// the note follows on a line of its own.
func (c *canvas) labelRow(label, value, right string) {
	lab := c.st.label(label, labelWidth)
	if c.fits(value, right) {
		c.spread(lab+value, right)
		return
	}
	indent := strings.Repeat(" ", labelWidth)
	for i, l := range wrapLines(value, c.width-labelWidth, "") {
		if i == 0 {
			c.add(lab + l)
		} else {
			c.add(indent + l)
		}
	}
	if right != "" {
		c.spread(indent, right)
	}
}

// gapFor is the space needed before right: one column, or none without it.
func gapFor(right string) int {
	if right == "" {
		return 0
	}
	return 1
}

// rule adds a line across the content in the frame colour.
func (c *canvas) rule() { c.add(c.st.Frame.Render(strings.Repeat(c.st.g.thin, c.width))) }

// footerText puts text where the key hints go.
func (c *canvas) footerText(text string, style lipgloss.Style) {
	c.keys = nil
	for _, l := range wrapLines(c.st.text(text), c.total-c.keyIndent()-1, "") {
		c.keys = append(c.keys, style.Render(l))
	}
}

// callout adds a callout box across the content; at the last fit level it
// loses its box.
func (c *canvas) callout(tone lipgloss.Style, t calloutText) {
	if c.level >= fitNoDigits {
		c.add(c.st.calloutLines(tone, c.width, t, false, false)...)
		return
	}
	c.lines = append(c.lines, c.st.callout(tone, c.width, t, c.level < fitNoSpacers, c.level < fitNoNote)...)
}

// mark adds text after a mark in tone ("✗ invalid …"), wrapped with a
// hanging indent, in style.
func (c *canvas) mark(tone lipgloss.Style, mark, text string, style lipgloss.Style) {
	for i, l := range wrapLines(c.st.text(text), c.width-2, "") {
		if i == 0 {
			c.add(tone.Render(mark) + " " + style.Render(l))
		} else {
			c.add("  " + style.Render(l))
		}
	}
}

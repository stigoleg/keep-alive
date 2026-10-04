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
func (c *canvas) height() int {
	n := len(c.lines) + len(c.keys)
	if c.framed {
		n += 2
	}
	return n
}

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
	for _, k := range c.keys {
		out = append(out, strings.Repeat(" ", c.keyIndent())+k)
	}
	return strings.Join(out, "\n")
}

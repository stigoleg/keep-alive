package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// gradientStop is a colour at a position from 0 to 1.
type gradientStop struct {
	hex string
	at  float64
}

// gradient spreads n colours evenly over stops (sorted by position, the
// first at 0 and the last at 1), blending in OkLab so the steps look even.
// The first and last colours are exactly the end stops; n = 1 gives the
// first stop.
func gradient(n int, stops []gradientStop) []string {
	if n <= 0 || len(stops) == 0 {
		return nil
	}
	cols := make([]colorful.Color, len(stops))
	for i, s := range stops {
		c, err := colorful.Hex(s.hex)
		if err != nil {
			panic("tui: bad gradient colour " + s.hex)
		}
		cols[i] = c
	}
	out := make([]string, n)
	for i := range n {
		t := 0.0
		if n > 1 {
			t = float64(i) / float64(n-1)
		}
		out[i] = blendAt(t, stops, cols)
	}
	return out
}

func blendAt(t float64, stops []gradientStop, cols []colorful.Color) string {
	last := len(stops) - 1
	switch {
	case t <= stops[0].at:
		return strings.ToLower(stops[0].hex)
	case t >= stops[last].at:
		return strings.ToLower(stops[last].hex)
	}
	for i := 1; i <= last; i++ {
		if t > stops[i].at {
			continue
		}
		a, b := stops[i-1], stops[i]
		if b.at <= a.at {
			return strings.ToLower(b.hex)
		}
		return cols[i-1].BlendOkLab(cols[i], (t-a.at)/(b.at-a.at)).Clamped().Hex()
	}
	return strings.ToLower(stops[last].hex)
}

// gradientStops returns the dark or light stops for the terminal.
func (st Styles) gradientStops(dark, light []gradientStop) []gradientStop {
	if st.dark {
		return dark
	}
	return light
}

// paint renders each cell of cells in the matching colour of a gradient
// over len(cells) colours (spaces stay plain); bold makes the text bold
// too. Without colours it
// renders the cells as they are (bold when asked).
func (st Styles) paint(cells []string, stops []gradientStop, bold bool) string {
	base := st.r.NewStyle().Bold(bold)
	if !st.color {
		return base.Render(strings.Join(cells, ""))
	}
	var b strings.Builder
	for i, hex := range gradient(len(cells), stops) {
		if cells[i] == " " { // nothing to colour
			b.WriteString(" ")
			continue
		}
		b.WriteString(base.Foreground(lipgloss.Color(hex)).Render(cells[i]))
	}
	return b.String()
}

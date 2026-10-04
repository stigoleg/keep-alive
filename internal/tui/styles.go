package tui

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The palette. Adaptive colours pick the light or dark variant from the
// renderer's background setting; limited terminals get the nearest colour.
var (
	colorAccent  = lipgloss.AdaptiveColor{Light: "#6639BA", Dark: "#A78BFA"}
	colorOK      = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	colorWarn    = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
	colorProblem = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#6E7781", Dark: "#8B949E"}
)

// Styles is every style the UI uses, built from one renderer. Nothing in
// this package styles text at init; New builds the Styles.
type Styles struct {
	Title    lipgloss.Style // the app name
	Heading  lipgloss.Style // section headings
	Selected lipgloss.Style // the chosen menu item
	Accent   lipgloss.Style
	OK       lipgloss.Style
	Warn     lipgloss.Style
	Problem  lipgloss.Style
	Muted    lipgloss.Style
	// Cursor marks the text cursor in an input; plain is set when the
	// renderer has no colours, so the cursor is drawn as a block instead.
	Cursor lipgloss.Style
	plain  bool
}

// NewStyles builds the styles from r; nil means plain text.
func NewStyles(r *lipgloss.Renderer) Styles {
	if r == nil {
		r = NewRenderer(io.Discard, false)
	}
	s := r.NewStyle()
	return Styles{
		Title:    s.Bold(true),
		Heading:  s.Bold(true),
		Selected: s.Bold(true).Foreground(colorAccent),
		Accent:   s.Foreground(colorAccent),
		OK:       s.Foreground(colorOK),
		Warn:     s.Foreground(colorWarn),
		Problem:  s.Foreground(colorProblem),
		Muted:    s.Foreground(colorMuted),
		Cursor:   s.Reverse(true),
		plain:    r.ColorProfile() == termenv.Ascii,
	}
}

// NewRenderer returns a renderer for w that never queries the terminal.
// Bubble Tea's package init already asked the terminal for its background
// colour (and lipgloss cached the answer); asking again once Bubble Tea owns
// the terminal would race its input reader and can stall for seconds. color
// false (NO_COLOR) renders plain text; otherwise the colour profile comes
// from the environment (TERM, COLORTERM, NO_COLOR).
func NewRenderer(w io.Writer, color bool) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w)
	r.SetHasDarkBackground(lipgloss.HasDarkBackground())
	if !color {
		r.SetColorProfile(termenv.Ascii)
	}
	return r
}

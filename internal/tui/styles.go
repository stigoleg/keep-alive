package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The palette ("Aurora"). Adaptive colours pick the light or dark variant
// from the renderer's background setting; terminals with fewer colours get
// the nearest one. Plain text keeps the terminal's own foreground.
var (
	colorFG     = lipgloss.AdaptiveColor{Light: "#1F2330", Dark: "#E4E6EE"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#687085", Dark: "#8A90A2"}
	colorDim    = lipgloss.AdaptiveColor{Light: "#9AA0B1", Dark: "#5B6172"}
	colorFrame  = lipgloss.AdaptiveColor{Light: "#C8CCD8", Dark: "#3A3F50"}
	colorSel    = lipgloss.AdaptiveColor{Light: "#ECEEF6", Dark: "#1E2232"}
	colorTrack  = lipgloss.AdaptiveColor{Light: "#E1E4EC", Dark: "#2A2F3E"}
	colorAccent = lipgloss.AdaptiveColor{Light: "#6D3FD9", Dark: "#A78BFA"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	colorBad    = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#F87171"}
	colorKeyBG  = lipgloss.AdaptiveColor{Light: "#E6E8F0", Dark: "#272C3B"}
	colorKeyFG  = lipgloss.AdaptiveColor{Light: "#3A4159", Dark: "#CFD3E1"}
	// Text on an ok, warn or muted pill, and on a bad one.
	colorPillFG    = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#062012"}
	colorPillBadFG = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#2B0808"}
)

// Gradient stops, dark and light: violet → pink (at 55 %) → amber for the
// wordmark and progress, teal → green for the sparkline.
var (
	gradDark   = []gradientStop{{"#A78BFA", 0}, {"#F472B6", 0.55}, {"#FBBF24", 1}}
	gradLight  = []gradientStop{{"#6D3FD9", 0}, {"#DB2777", 0.55}, {"#D97706", 1}}
	sparkDark  = []gradientStop{{"#2DD4BF", 0}, {"#4ADE80", 1}}
	sparkLight = []gradientStop{{"#0F766E", 0}, {"#15803D", 1}}
)

// Look is how the UI may draw. The zero value draws in colour with Unicode
// symbols.
type Look struct {
	// NoColor drops every colour (NO_COLOR); bold and reverse video stay
	// when the renderer can draw them.
	NoColor bool
	// ASCII draws with ASCII characters only, for a terminal that is not
	// UTF-8.
	ASCII bool
}

// Styles is every style the UI uses, built from one renderer. Nothing in
// this package styles text at init; New builds the Styles.
type Styles struct {
	Plain    lipgloss.Style // no style, from this renderer
	Title    lipgloss.Style // the app name
	Heading  lipgloss.Style // section headings
	Selected lipgloss.Style // the chosen menu item
	Bold     lipgloss.Style
	Accent   lipgloss.Style
	OK       lipgloss.Style
	Warn     lipgloss.Style
	Problem  lipgloss.Style
	Muted    lipgloss.Style
	Dim      lipgloss.Style
	Frame    lipgloss.Style // borders
	Track    lipgloss.Style // the empty part of a bar
	Label    lipgloss.Style // UPPERCASE section labels
	Key      lipgloss.Style // a keycap
	Band     lipgloss.Style // the selection band behind the chosen row
	// Cursor marks the text cursor in an input; plain is set when the
	// renderer draws no escape sequences at all, so the cursor is drawn as
	// a block instead.
	Cursor lipgloss.Style

	pills [pillCount]lipgloss.Style

	r     *lipgloss.Renderer
	g     glyphs
	color bool // colours are drawn
	plain bool // nothing but text is drawn
	dark  bool // the terminal background is dark
}

// NewStyles builds the styles from r with look; nil means plain text.
func NewStyles(r *lipgloss.Renderer, look Look) Styles {
	if r == nil {
		r = NewRenderer(io.Discard, false)
	}
	plain := r.ColorProfile() == termenv.Ascii
	color := !plain && !look.NoColor
	s := r.NewStyle()
	fg := func(c lipgloss.TerminalColor) lipgloss.Style {
		if !color {
			return s
		}
		return s.Foreground(c)
	}
	st := Styles{
		Plain:    s,
		Title:    s.Bold(true),
		Heading:  s.Bold(true),
		Selected: fg(colorAccent).Bold(true),
		Bold:     s.Bold(true),
		Accent:   fg(colorAccent),
		OK:       fg(colorOK),
		Warn:     fg(colorWarn),
		Problem:  fg(colorBad),
		Muted:    fg(colorMuted),
		Dim:      fg(colorDim),
		Frame:    fg(colorFrame),
		Track:    fg(colorTrack),
		Label:    fg(colorMuted).Bold(true),
		Key:      s,
		Band:     s,
		Cursor:   s.Reverse(true),
		r:        r,
		g:        unicodeGlyphs,
		color:    color,
		plain:    plain,
		dark:     r.HasDarkBackground(),
	}
	if look.ASCII {
		st.g = asciiGlyphs
	}
	for k := range st.pills {
		st.pills[k] = s.Bold(true)
	}
	if color {
		st.Key = s.Background(colorKeyBG).Foreground(colorKeyFG)
		st.Band = s.Background(colorSel).Foreground(colorFG)
		pill := func(bg, fg lipgloss.TerminalColor) lipgloss.Style { return s.Bold(true).Background(bg).Foreground(fg) }
		st.pills = [pillCount]lipgloss.Style{
			pillOK:    pill(colorOK, colorPillFG),
			pillWarn:  pill(colorWarn, colorPillFG),
			pillBad:   pill(colorBad, colorPillBadFG),
			pillMuted: pill(colorMuted, colorPillFG),
		}
	}
	return st
}

// NewRenderer returns a renderer for w that never queries the terminal.
// Bubble Tea's package init already asked the terminal for its background
// colour (and lipgloss cached the answer); asking again once Bubble Tea owns
// the terminal would race its input reader and can stall for seconds. color
// false (NO_COLOR) keeps bold and reverse video on a terminal, and draws
// plain text elsewhere; pair it with Look.NoColor. Otherwise the colour
// profile comes from the environment (TERM, COLORTERM, NO_COLOR).
func NewRenderer(w io.Writer, color bool) *lipgloss.Renderer {
	r := lipgloss.NewRenderer(w)
	r.SetHasDarkBackground(lipgloss.HasDarkBackground())
	if !color {
		p := termenv.NewOutput(w).ColorProfile() // ignores NO_COLOR
		if p != termenv.Ascii {
			p = termenv.ANSI
		}
		r.SetColorProfile(p)
	}
	return r
}

// UnicodeTerminal reports whether the terminal shows UTF-8: the locale
// (LC_ALL, then LC_CTYPE, then LANG) names UTF-8. Without a locale, macOS
// terminals are UTF-8, and on Windows Windows Terminal (WT_SESSION) is.
func UnicodeTerminal(lookup func(string) (string, bool), goos string) bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v, ok := lookup(k); ok && v != "" {
			v = strings.ToLower(v)
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	switch goos {
	case "darwin":
		return true
	case "windows":
		v, ok := lookup("WT_SESSION")
		return ok && v != ""
	}
	return false
}

// glyphs are the symbols the UI draws, in Unicode or ASCII.
type glyphs struct {
	// The rounded frame.
	tl, tr, bl, br, h, v string
	// States: on (◉), off (○), awake (●), paused (◐), problem (▲), stopped
	// (■), the selection marker (❯), input prompt (›), valid (✓), invalid (✗).
	on, off, awake, paused, problem, stopped, sel, prompt, valid, invalid string
	// Bars: block (█), shade (░), thick line (━), thin line (─); the
	// sparkline's levels, lowest first; the big digits' full and upper half
	// blocks.
	block, shade, thick, thin string
	spark                     [8]string
	full, upper               string
	// Text.
	ellipsis, dash, sep string
}

var (
	unicodeGlyphs = glyphs{
		tl: "╭", tr: "╮", bl: "╰", br: "╯", h: "─", v: "│",
		on: "◉", off: "○", awake: "●", paused: "◐", problem: "▲", stopped: "■", sel: "❯", prompt: "›", valid: "✓", invalid: "✗",
		block: "█", shade: "░", thick: "━", thin: "─",
		spark: [8]string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"},
		full:  "█", upper: "▀",
		ellipsis: "…", dash: "–", sep: "·",
	}
	asciiGlyphs = glyphs{
		tl: "+", tr: "+", bl: "+", br: "+", h: "-", v: "|",
		on: "*", off: "o", awake: "*", paused: "~", problem: "!", stopped: "#", sel: ">", prompt: ">", valid: "+", invalid: "x",
		block: "#", shade: "-", thick: "=", thin: "-",
		spark: [8]string{"_", ".", ".", "-", "-", "-", "=", "="},
		full:  "#", upper: "\"",
		ellipsis: "...", dash: "-", sep: "-",
	}
)

// asciiText maps the typographic characters UI text uses to ASCII, one
// cell for one where it can.
var asciiText = strings.NewReplacer(
	"…", "...", "–", "-", "—", "-", "·", "-", "→", "->", "×", "x",
	"‘", "'", "’", "'", "“", `"`, "”", `"`, "✓", "+", "✗", "x",
	"↑", "^", "↓", "v", "⏎", "enter",
)

// text prepares UI text for the terminal: ASCII for a terminal that is not
// UTF-8, unchanged otherwise. Call it before measuring.
func (st Styles) text(s string) string {
	if st.g.ellipsis == "…" {
		return s
	}
	return asciiText.Replace(s)
}

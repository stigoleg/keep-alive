package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpKey struct{ key, text string }

var (
	homeHelp = []helpKey{
		{"↑↓", "choose how long"},
		{"⏎", "start"},
		{"?", "this help"},
		{"q", "quit"},
		{"a", "simulate activity"},
		{"d", "keep the display on"},
		{"b", "stop at a battery level"},
		{"k", "tap Shift with activity"},
	}
	dashHelp = []helpKey{
		{"a", "activity on or off"},
		{"+/-", "15 min more or less"},
		{"?", "this help"},
		{"s/esc", "stop, back to Home"},
		{"q", "stop and quit"},
		{"ctrl+c", "stop and quit"},
	}
	attachedHelp = []helpKey{
		{"a", "activity on or off"},
		{"+/-", "15 min more or less"},
		{"?", "this help"},
		{"s/esc", "stop it (asks first)"},
		{"q", "quit, keep it running"},
		{"ctrl+c", "quit, keep it running"},
	}
)

func (m Model) helpView() string {
	return m.fit(func(level int) string { return m.helpPage(level).String() })
}

func (m Model) helpPage(level int) *canvas {
	st := m.st
	c := newCanvas(st, m.width, level, true)
	c.header(st.Muted.Render(versionText(m.version)))
	c.spacer()
	c.add(st.label("Keys", 0))

	keys := homeHelp
	if m.screen == screenDashboard {
		keys = dashHelp
		if m.dash.ctrl.Attached() != nil {
			keys = attachedHelp
		}
	}
	capW, textW := 0, 0
	for _, k := range keys {
		capW, textW = max(capW, lipgloss.Width(st.keycap(k.key))), max(textW, lipgloss.Width(k.text))
	}
	cell := func(k helpKey) string {
		kc := st.keycap(k.key)
		return kc + strings.Repeat(" ", capW-lipgloss.Width(kc)+1) + k.text
	}
	// Two pairs per line when they fit, else one.
	colW := capW + 1 + textW
	if c.width >= 2*colW+3 {
		half := (len(keys) + 1) / 2
		for i := range half {
			line := cell(keys[i])
			if j := i + half; j < len(keys) {
				line += strings.Repeat(" ", colW+3-lipgloss.Width(line)) + cell(keys[j])
			}
			c.add(line)
		}
	} else {
		for _, k := range keys {
			c.add(cell(k))
		}
	}
	c.spacer()

	c.wrapped("Simulate activity makes small pointer arcs (up to 130 px) once you are idle, so Teams and Slack show you as Active.", "", st.Muted)
	c.wrapped(`It pauses while you use the computer or the screen is locked; "keepalive doctor" checks that it works.`, "", st.Muted)
	path := homeRelative(m.logPath)
	logs := "Logs: " + path
	if !m.logOn {
		logs = "Logs: off; start with --log to write " + path
	}
	c.wrapped(logs, "", st.Muted)
	c.keyHints(keyHint{"esc", "close", ""}, keyHint{"ctrl+c", "quit", ""})
	return c
}

// homeRelative shortens a path under the home directory to "~/…", so the
// log path fits on one line of the help.
func homeRelative(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return path
	}
	return "~" + string(filepath.Separator) + rel
}

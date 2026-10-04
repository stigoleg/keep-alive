package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpKey struct{ key, text string }

var (
	homeHelp = []helpKey{
		{"↑ ↓", "choose how long"},
		{"enter", "start"},
		{"?", "this help"},
		{"q", "quit"},
		{"a", "simulate activity"},
		{"d", "keep the display on"},
		{"b", "stop at a battery level"},
		{"k", "tap Shift with activity"},
	}
	dashHelp = []helpKey{
		{"a", "activity on or off"},
		{"+ -", "15 min more or less"},
		{"?", "this help"},
		{"s esc", "stop, back to Home"},
		{"q", "stop and quit"},
		{"ctrl+c", "stop and quit"},
	}
	attachedHelp = []helpKey{
		{"a", "activity on or off"},
		{"+ -", "15 min more or less"},
		{"?", "this help"},
		{"s esc", "stop it (asks first)"},
		{"q", "quit, keep it running"},
		{"ctrl+c", "quit, keep it running"},
	}
)

func (m Model) helpView() string {
	st := m.st
	p := newPage(st, m.width)
	p.add(st.Title.Render("keepalive "+m.version) + st.Muted.Render(" · help"))
	p.blank()

	keys := homeHelp
	if m.screen == screenDashboard {
		keys = dashHelp
		if m.dash.ctrl.Attached() != nil {
			keys = attachedHelp
		}
	}
	keyW, textW := 0, 0
	for _, k := range keys {
		keyW, textW = max(keyW, lipgloss.Width(k.key)), max(textW, lipgloss.Width(k.text))
	}
	cell := func(k helpKey) string {
		return st.Accent.Render(k.key) + strings.Repeat(" ", keyW-lipgloss.Width(k.key)+2) + k.text
	}
	// Two pairs per line when they fit, else one.
	colW := keyW + 2 + textW
	if p.width >= 2*colW+2 {
		half := (len(keys) + 1) / 2
		for i := range half {
			line := cell(keys[i])
			if j := i + half; j < len(keys) {
				line += strings.Repeat(" ", colW+2-lipgloss.Width(line)) + cell(keys[j])
			}
			p.add(line)
		}
	} else {
		for _, k := range keys {
			p.add(cell(k))
		}
	}
	p.blank()

	p.wrapped("Simulate activity moves the pointer in small arcs (up to about 130 px) and back to where it was, once you have been idle for a while, so Teams and Slack keep showing you as Active.", "", st.Muted)
	p.wrapped(`It pauses while you use the computer or the screen is locked; "keepalive doctor" checks that it works here.`, "", st.Muted)
	logs := "Logs: " + m.logPath
	if !m.logOn {
		logs = "Logs: off; start with --log to write " + m.logPath
	}
	p.wrapped(logs, "", st.Muted)
	p.blank()
	p.footer("? or esc close")
	return p.String()
}

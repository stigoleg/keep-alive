package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// Home's ways to keep the computer awake, in menu order.
const (
	modeIndefinite = iota
	modeDuration
	modeUntil
	modeSchedule
	modeCount
)

var modeLabels = [modeCount]string{"Until I stop it", "For a duration…", "Until a time…", "During work hours…"}

// options are Home's toggles; they carry over from one session to the next.
type options struct {
	active, display, keys bool
	battery               int // threshold while on; 0 = off
}

type homeState struct {
	cursor int
	opts   options
	// The last values used this run, else the configured ones.
	lastDuration time.Duration
	lastUntil    string // "17:00"
	lastSchedule string // normalized
	lastBattery  int

	battery                      batteryMsg
	activityReason, activityHint string
	noBattery                    bool     // b was pressed on a machine without one
	instance                     *Warning // another keepalive holds the claim
	problem                      *Warning // why the last session failed
}

func newHome(base session.Config) homeState {
	h := homeState{
		opts: options{
			active:  base.Active,
			display: base.KeepDisplay,
			keys:    base.Activity.Keys,
			battery: base.BatteryThreshold,
		},
		lastDuration: base.Duration,
		lastBattery:  base.BatteryThreshold,
	}
	if !base.Until.IsZero() {
		h.lastUntil = base.Until.Format("15:04")
	}
	if base.Schedule != nil {
		h.lastSchedule = base.Schedule.String()
		h.cursor = modeSchedule
	}
	return h
}

func (m Model) homeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	h := &m.home
	switch msg.String() {
	case "up":
		h.cursor = max(h.cursor-1, 0)
	case "down":
		h.cursor = min(h.cursor+1, modeCount-1)
	case "enter":
		return m.choose()
	case "a":
		h.opts.active = !h.opts.active
	case "d":
		h.opts.display = !h.opts.display
	case "k":
		h.opts.keys = !h.opts.keys
	case "b":
		switch {
		case h.opts.battery > 0:
			h.opts.battery = 0
		case h.battery.read && (h.battery.err != nil || !h.battery.status.Available):
			h.noBattery = true
		default:
			return m.openInput(inputBattery), nil
		}
	case "?":
		m.help = true
	case "q":
		return m, tea.Quit
	case "esc":
		h.noBattery, h.problem = false, nil
	}
	return m, nil
}

// choose starts the selected way, or opens its input screen.
func (m Model) choose() (tea.Model, tea.Cmd) {
	switch m.home.cursor {
	case modeDuration:
		return m.openInput(inputDuration), nil
	case modeUntil:
		return m.openInput(inputUntil), nil
	case modeSchedule:
		return m.openInput(inputSchedule), nil
	}
	return m.startSession(m.sessionConfig())
}

// sessionConfig is Base with Home's options and no end; the caller adds
// the chosen limit.
func (m Model) sessionConfig() session.Config {
	cfg := m.base
	cfg.Duration, cfg.Until, cfg.Schedule = 0, time.Time{}, nil
	o := m.home.opts
	cfg.Active, cfg.KeepDisplay, cfg.Activity.Keys, cfg.BatteryThreshold = o.active, o.display, o.keys, o.battery
	return cfg
}

func (m Model) homeView() string {
	h, st := m.home, m.st
	p := newPage(st, m.width)
	p.add(st.Title.Render("keepalive " + m.version))
	p.blank()

	p.add(st.Heading.Render("Keep this computer awake"))
	labelW := max(lenRunes(modeLabels[modeSchedule])+2, p.width-26)
	values := [modeCount]string{"", lastText(shortDurationOr(h.lastDuration)), lastText(h.lastUntil), h.lastSchedule}
	for i, label := range modeLabels {
		prefix := "  "
		if i == h.cursor {
			prefix, label = st.Selected.Render("▸ "), st.Selected.Render(label)
		}
		p.add(prefix + row(p.width-2, label, labelW, st.Muted.Render(values[i]), ""))
	}
	p.blank()

	p.add(st.Heading.Render("Options"))
	battery := "Stop at battery"
	if h.opts.battery > 0 {
		battery = fmt.Sprintf("Stop at battery %d%%", h.opts.battery)
	}
	opts := []struct {
		on                 bool
		label, desc, short string
		key                string
	}{
		{h.opts.active, "Simulate activity", "keeps Teams/Slack Active", "", "a"},
		{h.opts.display, "Keep display on", "", "", "d"},
		{h.opts.battery > 0, battery, h.batteryText(), h.batteryShort(), "b"},
		{h.opts.keys, "Tap Shift too", "only with activity", "", "k"},
	}
	optW := 0
	for _, o := range opts {
		optW = max(optW, lenRunes("[x] "+o.label)+2)
	}
	room := p.width - 2 - optW - 3 // the key and its gap
	for _, o := range opts {
		box := "[ ] "
		if o.on {
			box = "[x] "
		}
		desc := o.desc
		if lenRunes(desc) > room {
			desc = o.short
		}
		p.add("  " + row(p.width-2, box+o.label, optW, st.Muted.Render(desc), st.Accent.Render(o.key)))
	}

	if ws := m.homeWarnings(); len(ws) > 0 {
		p.blank()
		for _, w := range ws {
			p.notice(w.mark, w.style(st), w.Text, w.Fix)
		}
	}
	p.blank()
	p.footer("↑↓ choose", "enter start", "? help", "q quit")
	return p.String()
}

// batteryShort is batteryText for narrow terminals.
func (h homeState) batteryShort() string {
	if h.battery.read && h.battery.err == nil && h.battery.status.Available {
		return fmt.Sprintf("%d%%", h.battery.status.Percentage)
	}
	return h.batteryText()
}

func (h homeState) batteryText() string {
	switch {
	case !h.battery.read:
		return ""
	case h.battery.err != nil || !h.battery.status.Available:
		return "no battery"
	}
	return fmt.Sprintf("current %d%%", h.battery.status.Percentage)
}

type homeWarning struct {
	Warning
	mark string // "!" or "✗"
}

func (w homeWarning) style(st Styles) lipgloss.Style {
	if w.mark == "✗" {
		return st.Problem
	}
	return st.Warn
}

// homeWarnings are the problems Home explains, most important first.
func (m Model) homeWarnings() []homeWarning {
	h := m.home
	var ws []homeWarning
	if h.problem != nil {
		ws = append(ws, homeWarning{*h.problem, "✗"})
	}
	if h.instance != nil {
		ws = append(ws, homeWarning{*h.instance, "!"})
	}
	if h.opts.active && h.activityReason != "" {
		ws = append(ws, homeWarning{Warning{"Activity simulation won't work: " + h.activityReason, h.activityHint}, "!"})
	}
	if h.noBattery {
		ws = append(ws, homeWarning{Warning{Text: `No battery found: "Stop at battery" only works on a machine with a battery.`}, "!"})
	}
	return ws
}

// activityIdle is the idle time before simulating, for "1m 10s of 2m".
func activityIdle(cfg session.Config) time.Duration {
	if cfg.Activity.IdleThreshold > 0 {
		return cfg.Activity.IdleThreshold
	}
	return activity.DefaultIdleThreshold
}

func lastText(v string) string {
	if v == "" {
		return ""
	}
	return "last: " + v
}

func shortDurationOr(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return shortDuration(d)
}

func lenRunes(s string) int { return len([]rune(s)) }

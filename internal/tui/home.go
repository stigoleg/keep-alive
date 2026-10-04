package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	return m.fit(func(level int) string { return m.homePage(level).String() })
}

func (m Model) homePage(level int) *canvas {
	h, st := m.home, m.st
	c := newCanvas(st, m.width, level, true)
	c.header(st.Muted.Render(versionText(m.version)))
	c.spacer()

	c.add(st.label("Keep this computer awake", 0))
	values := [modeCount]string{"", shortDurationOr(h.lastDuration), h.lastUntil, st.scheduleText(h.lastSchedule)}
	labelW := 0
	for _, label := range modeLabels {
		labelW = max(labelW, lipgloss.Width(st.text(label))+2)
	}
	for i := range values { // the labels stay whole; long values are cut
		values[i] = ansi.Truncate(values[i], max(c.width-labelW-2, 1), st.g.ellipsis)
	}
	for i, label := range modeLabels {
		if i == h.cursor {
			c.add(st.band(c.width, []seg{{st.g.sel + " " + label, st.Selected}}, []seg{{values[i], st.Muted}}))
			continue
		}
		c.spread("  "+st.text(label), st.Muted.Render(values[i]))
	}
	c.spacer()

	c.add(st.label("Options", 0))
	battery := "Stop at battery"
	if h.opts.battery > 0 {
		battery = fmt.Sprintf("Stop at battery %d%%", h.opts.battery)
	}
	opts := []struct {
		on                 bool
		label, desc, short string
		key                string
	}{
		{h.opts.active, "Simulate activity", "keeps Teams & Slack active", "", "a"},
		{h.opts.display, "Keep display on", "", "", "d"},
		{h.opts.battery > 0, battery, h.batteryText(), h.batteryShort(), "b"},
		{h.opts.keys, "Tap Shift too", "with activity only", "", "k"},
	}
	optW := 0
	for _, o := range opts {
		optW = max(optW, lipgloss.Width(o.label)+4) // the mark, a space and a gap of 2
	}
	keyW := lipgloss.Width(st.keycap("a"))
	room := c.width - optW - keyW - 1
	for _, o := range opts {
		mark := st.Dim.Render(st.g.off)
		if o.on {
			mark = st.OK.Render(st.g.on)
		}
		left := mark + " " + o.label
		desc := o.desc
		if lipgloss.Width(desc) > room {
			desc = o.short
		}
		if desc != "" {
			left += strings.Repeat(" ", optW-lipgloss.Width(left)) + st.Muted.Render(st.text(desc))
		}
		c.spread(left, st.keycap(o.key))
	}

	for _, w := range m.homeWarnings() {
		c.spacer()
		c.callout(w.style(st), calloutText{mark: w.mark, text: w.Text, fix: w.Fix})
	}
	c.keyHints(keyHint{"↑↓", "move", ""}, keyHint{"⏎", "start", ""}, keyHint{"?", "help", ""}, keyHint{"q", "quit", ""})
	return c
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
	return fmt.Sprintf("now %d%%", h.battery.status.Percentage)
}

type homeWarning struct {
	Warning
	mark string // "!", or ✗ for a problem that blocks
}

func (w homeWarning) style(st Styles) lipgloss.Style {
	if w.mark == st.g.invalid {
		return st.Problem
	}
	return st.Warn
}

// homeWarnings are the problems Home explains, most important first.
func (m Model) homeWarnings() []homeWarning {
	h := m.home
	var ws []homeWarning
	if h.problem != nil {
		ws = append(ws, homeWarning{*h.problem, m.st.g.invalid})
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

func shortDurationOr(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return shortDuration(d)
}

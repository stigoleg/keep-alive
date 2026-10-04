package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

const (
	// extendStep is what + and - add or take away.
	extendStep = 15 * time.Minute
	// endPause is how long the final line shows before Home (or quitting).
	endPause = 2 * time.Second
	// flashTime is how long a footer message shows.
	flashTime = 5 * time.Second
)

type flashKind int

const (
	flashInfo flashKind = iota
	flashWarn
	flashProblem
)

type dashState struct {
	ctrl     Controller
	snap     session.Snapshot
	idleNeed time.Duration // 0 when unknown (attached)
	auto     bool          // started from flags: quit when it ends on its own
	stopping bool          // s was pressed, or the session failed (local)
	failure  string        // why a local session failed
	confirm  bool          // "Stop the running keepalive? y/n" (attached)

	ended   string // the final line, once the session has ended
	endedAt time.Time

	flash      string
	flashKind  flashKind
	flashUntil time.Time
}

// ---- messages ----

type tickMsg struct{ gen int }

type snapMsg struct {
	c    Controller
	snap session.Snapshot
	err  error
}

type eventMsg struct {
	c  Controller
	ev session.Event
	ok bool
}

type actionMsg struct {
	c    Controller
	text string
	err  error
}

type stopDoneMsg struct {
	c   Controller
	err error
}

type returnMsg struct{ c Controller }

func tickCmd(gen int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{gen: gen} })
}

func snapCmd(c Controller) tea.Cmd {
	return func() tea.Msg {
		s, err := c.Snapshot()
		return snapMsg{c: c, snap: s, err: err}
	}
}

func waitEvent(c Controller) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-c.Events()
		return eventMsg{c: c, ev: ev, ok: ok}
	}
}

func stopCmd(c Controller) tea.Cmd {
	return func() tea.Msg { return stopDoneMsg{c: c, err: c.Stop()} }
}

func releaseCmd(s *slot, c Controller) tea.Cmd {
	return func() tea.Msg {
		_ = s.release(c)
		return nil
	}
}

// ---- transitions ----

// openDashboard follows c. idleNeed is the configured idle time before
// simulating (0 when unknown).
func (m Model) openDashboard(c Controller, idleNeed time.Duration) Model {
	m.slot.set(c)
	m.screen, m.help = screenDashboard, false
	m.tick++
	m.dash = dashState{ctrl: c, idleNeed: idleNeed}
	if ic, ok := c.(*ipcController); ok {
		m.dash.snap = ic.cached()
	}
	return m
}

func (m Model) dashCmds() []tea.Cmd {
	c := m.dash.ctrl
	return []tea.Cmd{tickCmd(m.tick), waitEvent(c), snapCmd(c)}
}

// toHome leaves the dashboard, keeping Home's options. Leaving an attached
// session claims this process as the running keepalive again.
func (m Model) toHome() (Model, tea.Cmd) {
	c := m.dash.ctrl
	m.screen, m.help = screenHome, false
	m.tick++ // ends the tick chain
	m.dash = dashState{}
	cmds := []tea.Cmd{batteryCmd(m.battery)}
	if c != nil {
		cmds = append(cmds, releaseCmd(m.slot, c))
		if c.Attached() != nil && !m.claimed && m.claim != nil {
			cmds = append(cmds, claimCmd(m.claim, nil))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) flash(kind flashKind, text string, d time.Duration) Model {
	m.dash.flash, m.dash.flashKind, m.dash.flashUntil = text, kind, m.now().Add(d)
	return m
}

func (m Model) dashUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	d := &m.dash
	switch msg := msg.(type) {
	case tickMsg:
		if msg.gen != m.tick || m.screen != screenDashboard {
			return m, nil
		}
		if d.flash != "" && !m.now().Before(d.flashUntil) {
			d.flash = ""
		}
		if d.ended != "" {
			return m, tickCmd(m.tick)
		}
		return m, tea.Batch(tickCmd(m.tick), snapCmd(d.ctrl))
	case snapMsg:
		if msg.c == d.ctrl && d.ended == "" && msg.err == nil {
			d.snap = msg.snap
		}
	case eventMsg:
		if msg.c != d.ctrl || m.screen != screenDashboard || d.ended != "" {
			return m, nil
		}
		if !msg.ok {
			return m.ended(session.Event{Time: m.now(), Message: "the running keepalive is gone"})
		}
		return m.onEvent(msg.ev)
	case actionMsg:
		if msg.c != d.ctrl {
			return m, nil
		}
		if msg.err != nil {
			return m.flash(flashProblem, msg.err.Error(), flashTime), nil
		}
		return m.flash(flashInfo, msg.text, flashTime), snapCmd(d.ctrl)
	case stopDoneMsg:
		if msg.c != d.ctrl || m.screen != screenDashboard {
			return m, nil
		}
		if msg.c.Attached() != nil { // the stopped event follows
			if msg.err != nil {
				return m.flash(flashProblem, "could not stop it: "+msg.err.Error(), flashTime), nil
			}
			return m, nil
		}
		failure := d.failure
		m, cmd := m.toHome()
		switch {
		case msg.err != nil:
			m.home.problem = &Warning{Text: "Could not keep awake: " + msg.err.Error(), Fix: power.HintOf(msg.err)}
		case failure != "":
			m.home.problem = &Warning{Text: "Could not keep awake: " + failure}
		}
		return m, cmd
	case returnMsg:
		if msg.c != d.ctrl || m.screen != screenDashboard || d.ended == "" {
			return m, nil
		}
		if d.auto {
			return m, tea.Quit
		}
		return m.toHome()
	}
	return m, nil
}

func (m Model) onEvent(ev session.Event) (tea.Model, tea.Cmd) {
	d := &m.dash
	if ev.Type == session.EventStopped {
		return m.ended(ev)
	}
	d.snap = ev.Snapshot
	switch ev.Type {
	case session.EventWarning:
		m = m.flash(flashWarn, ev.Message, flashTime)
	case session.EventSchedule:
		m = m.flash(flashInfo, ev.Message, flashTime)
	}
	return m, waitEvent(d.ctrl)
}

// ended handles the end of the session (or of the connection to it).
func (m Model) ended(ev session.Event) (tea.Model, tea.Cmd) {
	d := &m.dash
	if d.ctrl.Attached() == nil {
		switch ev.Reason {
		case session.ReasonUser: // s: Home once the hold is released
			if d.stopping {
				return m, nil
			}
			return m.toHome()
		case session.ReasonIPC, session.ReasonSignal: // keepalive stop, or a signal
			return m, tea.Quit
		case session.ReasonError: // the error and its hint come with the stop
			d.stopping, d.failure = true, ev.Message
			return m, stopCmd(d.ctrl)
		}
	}
	msg := ev.Message
	if msg == "" {
		msg = string(ev.Reason)
	}
	when := ev.Time
	if when.IsZero() {
		when = m.now()
	}
	at := session.ClockText(m.now(), when)
	if strings.HasPrefix(msg, "stopped ") {
		d.ended = fmt.Sprintf("Stopped %s at %s", strings.TrimPrefix(msg, "stopped "), at)
	} else {
		d.ended = fmt.Sprintf("Stopped: %s at %s", msg, at)
	}
	d.endedAt, d.confirm, d.flash = when, false, ""
	if d.auto {
		m.final = strings.ToLower(d.ended[:1]) + d.ended[1:]
	}
	c := d.ctrl
	return m, tea.Tick(endPause, func(time.Time) tea.Msg { return returnMsg{c: c} })
}

func (m Model) dashKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	d := &m.dash
	c := d.ctrl
	attached := c.Attached() != nil
	key := msg.String()
	switch {
	case d.ended != "":
		switch key {
		case "q":
			return m, tea.Quit
		case "enter", "esc", "s":
			if d.auto {
				return m, tea.Quit
			}
			return m.toHome()
		}
		return m, nil
	case d.confirm:
		switch key {
		case "y", "Y":
			d.confirm = false
			return m.flash(flashInfo, "stopping it…", flashTime), stopCmd(c)
		case "n", "N", "esc":
			d.confirm = false
		case "q":
			return m, tea.Quit
		}
		return m, nil
	case d.stopping:
		if key == "q" {
			return m, tea.Quit
		}
		return m, nil
	}
	switch key {
	case "a":
		on := !d.snap.Active
		if !attached {
			m.home.opts.active = on
		}
		text := "activity simulation off"
		if on {
			text = "activity simulation on"
		}
		return m, func() tea.Msg { return actionMsg{c: c, text: text, err: c.SetActive(on)} }
	case "+", "=":
		return m.extend(extendStep)
	case "-":
		return m.extend(-extendStep)
	case "s", "esc":
		if attached {
			d.confirm = true
			return m, nil
		}
		d.stopping = true
		return m, stopCmd(c)
	case "q":
		return m, tea.Quit
	case "?":
		m.help = true
	}
	return m, nil
}

func (m Model) extend(by time.Duration) (tea.Model, tea.Cmd) {
	d := &m.dash
	if d.snap.EndsAt.IsZero() {
		return m.flash(flashWarn, "+/- only change a session with an end: for a duration or until a time", flashTime), nil
	}
	c, now := d.ctrl, m.now
	return m, func() tea.Msg {
		if err := c.Extend(by); err != nil {
			return actionMsg{c: c, err: err}
		}
		s, err := c.Snapshot()
		if err != nil || s.EndsAt.IsZero() {
			return actionMsg{c: c, text: "moved the end"}
		}
		t := now()
		return actionMsg{c: c, text: fmt.Sprintf("now until %s (%s left)", session.ClockText(t, s.EndsAt), span(s.EndsAt.Sub(t)))}
	}
}

// ---- view ----

func (m Model) dashView() string {
	d, st := m.dash, m.st
	s := d.snap
	now := m.now()
	if d.ended != "" {
		now = d.endedAt
	}
	p := newPage(st, m.width)
	p.header(st.Title.Render("keepalive "+m.version), m.badge(now))
	inst := d.ctrl.Attached()
	if inst != nil {
		p.add(st.Muted.Render(fmt.Sprintf("attached to %s · pid %d", originLabel(inst.Origin), inst.PID)))
	}
	p.blank()

	switch {
	case d.ended != "":
		p.wrapped(d.ended, "  ", st.Heading)
	case !s.Running:
		p.add("Starting…")
	default:
		p.wrapped(headline(s), "", st.Heading)
		m.timeline(p, s, now)
		p.blank()
		m.rows(p, s, now)
	}
	if d.flash != "" {
		p.blank()
		style := st.Muted
		switch d.flashKind {
		case flashWarn:
			style = st.Warn
		case flashProblem:
			style = st.Problem
		}
		p.wrapped(d.flash, "  ", style)
	}
	p.blank()
	switch {
	case d.ended != "" && d.auto:
		p.footer("q quit")
	case d.ended != "":
		p.footer("enter back", "q quit")
	case d.confirm:
		p.add(st.Warn.Render("Stop the running keepalive? y/n"))
	case d.stopping:
		p.footer("stopping…")
	default:
		items := []string{"a activity on"}
		if s.Active {
			items[0] = "a activity off"
		}
		if !s.EndsAt.IsZero() {
			items = append(items, "+/- 15 min")
		}
		if inst != nil {
			items = append(items, "s stop…", "? help", "q detach")
		} else {
			items = append(items, "s stop", "? help", "q quit")
		}
		p.footer(items...)
	}
	return p.String()
}

func (m Model) badge(now time.Time) string {
	st, s := m.st, m.dash.snap
	switch {
	case m.dash.ended != "":
		return st.Muted.Render("○ STOPPED")
	case !s.Running:
		return st.Muted.Render("○ STARTING")
	case !s.InWindow:
		b := "◐ PAUSED"
		if !s.NextChange.IsZero() {
			b += " until " + session.ClockText(now, s.NextChange)
		}
		return st.Warn.Render(b)
	case problem(s):
		return st.Problem.Render("▲ PROBLEM")
	}
	return st.OK.Render("● AWAKE")
}

// problem reports whether the session is not doing what it should.
func problem(s session.Snapshot) bool {
	return (s.Active && s.Activity.State == activity.StateDegraded) || s.PowerHold == ""
}

func headline(s session.Snapshot) string {
	switch {
	case !s.InWindow:
		return "Outside work hours · the computer may sleep"
	case s.KeepDisplay:
		return "Keeping system and display awake"
	}
	return "Keeping the system awake · the display may sleep"
}

// timeline is a progress bar for a session with an end, else how long it
// has run.
func (m Model) timeline(p *page, s session.Snapshot, now time.Time) {
	if s.EndsAt.IsZero() {
		if !s.StartedAt.IsZero() {
			p.add(m.st.Muted.Render("running for " + span(now.Sub(s.StartedAt))))
		}
		return
	}
	left := max(s.EndsAt.Sub(now), 0)
	text := fmt.Sprintf("%s left · until %s", span(left), session.ClockText(now, s.EndsAt))
	total := s.EndsAt.Sub(s.StartedAt)
	done := 1.0
	if total > 0 {
		done = 1 - float64(left)/float64(total)
	}
	barW := min(p.width-lipgloss.Width(text)-2, 30)
	if barW < 10 {
		p.add(progressBar(m.st, done, min(p.width, 30)))
		p.add(text)
		return
	}
	p.add(progressBar(m.st, done, barW) + "  " + text)
}

// rows are the dashboard's detail lines; rows that do not apply are left
// out.
func (m Model) rows(p *page, s session.Snapshot, now time.Time) {
	st := m.st
	labelW := 13 // "Work hours" and a gap
	if p.width < 50 {
		labelW = 11
	}
	add := func(label, value string) {
		lines := wrapLines(value, p.width-labelW, "")
		for i, l := range lines {
			if i == 0 {
				p.add(row(p.width, label, labelW, l, ""))
			} else {
				p.add(strings.Repeat(" ", labelW) + l)
			}
		}
	}
	fix := func(hint string) {
		for i, l := range wrapLines(hint, p.width-labelW-7, "") {
			if i == 0 {
				p.add(strings.Repeat(" ", labelW+2) + st.Muted.Render("fix:") + " " + l)
			} else {
				p.add(strings.Repeat(" ", labelW+7) + l)
			}
		}
	}

	act, hint := m.activityText(s, now)
	add("Activity", act)
	if hint != "" {
		fix(hint)
	}
	if s.Schedule != "" {
		v := s.Schedule
		switch {
		case s.NextChange.IsZero():
		case s.InWindow:
			v += " · ends " + session.ClockText(now, s.NextChange)
		default:
			v += " · starts " + session.ClockText(now, s.NextChange)
		}
		add("Work hours", v)
	}
	if b := s.Battery; b.Threshold > 0 {
		v := fmt.Sprintf("stops at %d%%", b.Threshold)
		if b.Available {
			v = fmt.Sprintf("%d%% · %s", b.Percent, v)
		}
		add("Battery", v)
	}
	if s.Watching != "" {
		add("Watching", s.Watching)
	}
	switch {
	case !s.InWindow:
		add("Holding", st.Muted.Render("nothing outside the work hours"))
	case s.PowerHold == "":
		add("Holding", st.Problem.Render("▲ lost · re-acquiring"))
	default:
		add("Holding", s.PowerHold)
	}
}

// activityText describes the activity simulation; hint is a fix for a
// degraded state.
func (m Model) activityText(s session.Snapshot, now time.Time) (text, hint string) {
	st, a := m.st, s.Activity
	switch {
	case !s.Active:
		return st.Muted.Render("off"), ""
	case !s.InWindow:
		return st.Muted.Render("on · resumes with the work hours"), ""
	}
	dim := func(t string) string { return st.Muted.Render("○ " + t) }
	switch a.State {
	case activity.StateWaitingIdle:
		t := "waiting for idle · " + span(a.Idle)
		if m.dash.idleNeed > 0 {
			t += " of " + span(m.dash.idleNeed)
		}
		return dim(t), ""
	case activity.StateSimulating:
		parts := []string{"simulating"}
		if !a.LastBurst.IsZero() {
			parts = append(parts, "last move "+span(now.Sub(a.LastBurst))+" ago")
		}
		if a.Method != "" {
			parts = append(parts, a.Method)
		}
		return st.OK.Render("●") + " " + strings.Join(parts, " · "), ""
	case activity.StatePausedUser:
		return dim("paused — you're using the computer"), ""
	case activity.StatePausedLocked:
		return dim("paused — screen locked"), ""
	case activity.StateDegraded:
		reason := a.Reason
		if reason == "" {
			reason = "unknown reason"
		}
		return st.Problem.Render("▲ not working: ") + reason, a.Hint
	case activity.StateOff:
		return st.Muted.Render("off"), ""
	}
	return st.Muted.Render("starting…"), ""
}

func originLabel(o string) string {
	switch o {
	case "run":
		return "keepalive run"
	case "":
		return "terminal"
	}
	return o
}

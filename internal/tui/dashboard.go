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

	bursts burstLog // for the sparkline

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
	m.dash = dashState{ctrl: c, idleNeed: idleNeed, bursts: newBurstLog(m.now())}
	if ic, ok := c.(*ipcController); ok {
		m.dash.setSnap(ic.cached(), m.now())
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

// setSnap follows the session's state, and its bursts for the sparkline.
func (d *dashState) setSnap(s session.Snapshot, now time.Time) {
	d.snap = s
	d.bursts.observe(s.Activity.LastBurst, now)
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
			d.setSnap(msg.snap, m.now())
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
	d.setSnap(ev.Snapshot, m.now())
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
	return m.fit(func(level int) string { return m.dashPage(level).String() })
}

func (m Model) dashPage(level int) *canvas {
	d, st := m.dash, m.st
	s := d.snap
	now := m.now()
	if d.ended != "" {
		now = d.endedAt
	}
	c := newCanvas(st, m.width, level, true)
	pill, aside := m.statusPill(now)
	switch {
	case aside == "":
		c.header(pill)
	case lipgloss.Width(st.wordmark())+2+lipgloss.Width(pill)+1+lipgloss.Width(aside) <= c.width:
		c.header(pill + " " + st.Muted.Render(st.text(aside)))
	default:
		c.header(pill)
		c.spread("", st.Muted.Render(st.text(aside)))
	}
	inst := d.ctrl.Attached()
	if inst != nil {
		c.add(st.Muted.Render(fmt.Sprintf("attached %s %s pid %d", st.g.sep, originLabel(inst.Origin), inst.PID)))
	}
	c.spacer()
	m.hero(c, s, now)

	switch {
	case d.ended != "":
		c.spacer()
		c.wrapped(d.ended, "  ", st.Muted)
	case s.Running:
		c.spacer()
		m.rows(c, s, now)
	}
	if d.flash != "" {
		c.spacer()
		style := st.Muted
		switch d.flashKind {
		case flashWarn:
			style = st.Warn
		case flashProblem:
			style = st.Problem
		}
		c.wrapped(d.flash, "  ", style)
	}
	if d.confirm {
		c.spacer()
		c.add(st.Warn.Bold(true).Render("Stop the running keepalive?"))
	}

	switch {
	case d.ended != "" && d.auto:
		c.keyHints(keyHint{"q", "quit", ""})
	case d.ended != "":
		c.keyHints(keyHint{"⏎", "back", ""}, keyHint{"q", "quit", ""})
	case d.confirm:
		c.keyHints(keyHint{"y", "stop it", ""}, keyHint{"n", "keep it running", "keep it"})
	case d.stopping:
		c.footerText("stopping…", st.Muted)
	default:
		keys := []keyHint{{"a", "activity on", "activity"}}
		if s.Active {
			keys[0].label = "activity off"
		}
		if !s.EndsAt.IsZero() {
			keys = append(keys, keyHint{"+/-", "15 min", ""})
		}
		if inst != nil {
			keys = append(keys, keyHint{"s", "stop…", "stop"}, keyHint{"?", "help", ""}, keyHint{"q", "detach", ""})
		} else {
			keys = append(keys, keyHint{"s", "stop", ""}, keyHint{"?", "help", ""}, keyHint{"q", "quit", ""})
		}
		c.keyHints(keys...)
	}
	return c
}

// statusPill is the header's state, and what goes beside it.
func (m Model) statusPill(now time.Time) (pill, aside string) {
	st, s := m.st, m.dash.snap
	switch {
	case m.dash.ended != "":
		return st.pill(pillMuted, st.g.stopped, "STOPPED"), ""
	case !s.Running:
		return st.pill(pillMuted, st.g.off, "STARTING"), ""
	case !s.InWindow:
		if !s.NextChange.IsZero() {
			aside = "until " + session.ClockText(now, s.NextChange)
		}
		return st.pill(pillWarn, st.g.paused, "PAUSED"), aside
	case s.Paused == session.PauseBattery:
		return st.pill(pillWarn, st.g.paused, "PAUSED"), "battery low"
	case problem(s):
		return st.pill(pillBad, st.g.problem, "ACTION NEEDED"), ""
	}
	return st.pill(pillOK, st.g.awake, "AWAKE"), ""
}

// problem reports whether the session is not doing what it should.
func problem(s session.Snapshot) bool {
	return (s.Active && s.Activity.State == activity.StateDegraded) || s.PowerHold == ""
}

// hero is the time in big digits under the header, with a line below: the
// time left of a timed session (with its progress), the time a session
// has run, or the time until the work hours start. On a short terminal it
// is one line of text.
func (m Model) hero(c *canvas, s session.Snapshot, now time.Time) {
	st, ended := m.st, m.dash.ended != ""
	var (
		digits, label, short string // the digits, their label, and a shorter one
		left, right          string // the same in one line
		line                 string // a bold line instead of digits
		done                 = -1.0 // progress of a timed session; < 0: a plain rule
	)
	sep := " " + st.g.sep + " "
	switch {
	case s.StartedAt.IsZero():
		if !ended {
			line = "Starting…"
		}
	case !s.InWindow && !ended:
		switch until := s.NextChange.Sub(now); {
		case s.NextChange.IsZero():
			line = "Outside work hours"
		case until < 24*time.Hour:
			when := session.ClockText(now, s.NextChange)
			digits, label, short = clockDigits(until, false), "until work hours"+sep+when, "until "+when
			left, right = st.Bold.Render(span(until))+st.Muted.Render(" until work hours"), st.Muted.Render("starts ")+when
		default:
			line = "Resumes " + session.ClockText(now, s.NextChange)
		}
	case s.Paused == session.PauseBattery && !ended:
		line = "Resumes when charging"
		if at := session.BatteryResumeAt(s.Battery.Threshold); at > 0 {
			line = fmt.Sprintf("Resumes at %d%% or when charging", at)
		}
	case !s.EndsAt.IsZero():
		rest := max(s.EndsAt.Sub(now), 0)
		until := session.ClockText(now, s.EndsAt)
		digits, label, short = clockDigits(rest, true), "left"+sep+"until "+until, "left"
		left, right = st.Bold.Render(span(rest))+st.Muted.Render(" left"), st.Muted.Render("until ")+until
		done = 1
		if total := s.EndsAt.Sub(s.StartedAt); total > 0 {
			done = 1 - float64(rest)/float64(total)
		}
	default:
		ran := now.Sub(s.StartedAt)
		digits, label = clockDigits(ran, false), "running"
		left = st.Bold.Render(span(ran)) + st.Muted.Render(" running")
	}
	if ended {
		label, short = "", ""
	}

	switch {
	case digits != "" && c.level < fitNoDigits:
		rows := st.bigDigits(digits)
		if lipgloss.Width(rows[0])+3+lipgloss.Width(st.text(label)) > c.width {
			label = short
		}
		if label != "" {
			rows[2] += "   " + st.Muted.Render(st.text(label))
		}
		c.add(rows[:]...)
	case digits != "":
		c.spread(left, right)
	case line != "":
		c.add(st.Bold.Render(st.text(line)))
	}
	if done >= 0 {
		c.add(st.lineBar(done, c.width))
	} else {
		c.rule()
	}
}

// rows are the dashboard's labelled rows; rows that do not apply are left
// out.
func (m Model) rows(c *canvas, s session.Snapshot, now time.Time) {
	st := m.st
	m.activityRow(c, s, now)
	if s.Schedule != "" {
		right := ""
		switch {
		case s.NextChange.IsZero():
		case s.InWindow:
			right = "ends " + session.ClockText(now, s.NextChange)
		default:
			right = "starts " + session.ClockText(now, s.NextChange)
		}
		c.labelRow("Hours", st.scheduleText(s.Schedule), st.Muted.Render(right))
	}
	if s.Battery.Threshold > 0 {
		m.batteryRow(c, s.Battery, s.Paused == session.PauseBattery)
	}
	if s.Watching != "" {
		c.labelRow("Watching", st.text(s.Watching), "")
	}
	switch {
	case !s.InWindow:
		if c.level < fitNoHolding {
			c.labelRow("Holding", c.pick("", st.Muted.Render("nothing outside the work hours"), st.Muted.Render("nothing while paused")), "")
		}
	case s.Paused == session.PauseBattery:
		if c.level < fitNoHolding {
			c.labelRow("Holding", c.pick("", st.Muted.Render("nothing while the battery is low"), st.Muted.Render("nothing while paused")), "")
		}
	case s.PowerHold == "": // a problem: never left out
		c.labelRow("Holding", st.Problem.Bold(true).Render(st.g.problem+" lost"), "")
		c.callout(st.Problem, calloutText{
			text: "The sleep prevention was lost, so this computer may sleep.",
			fix:  `if it keeps happening, run "keepalive doctor"`,
			note: "keepalive retries on its own, every 30 s at most",
		})
	default:
		if c.level < fitNoHolding {
			c.labelRow("Holding", st.Muted.Render(holdWhat(s)+" awake"), "")
		}
	}
}

// activityRow is the ACTIVITY row: the state, the sparkline of recent
// bursts and when the last one was; a callout explains a problem.
func (m Model) activityRow(c *canvas, s session.Snapshot, now time.Time) {
	st, a := m.st, s.Activity
	t := st.text
	var (
		state string
		spark bool
		fix   *calloutText
	)
	waiting := func(text string) string { return st.Dim.Render(st.g.off) + " " + t(text) }
	muted := func(texts ...string) string {
		for i := range texts {
			texts[i] = st.Muted.Render(t(texts[i]))
		}
		return c.pick("", texts...)
	}
	switch {
	case !s.Active:
		state = st.Muted.Render("off")
	case !s.InWindow:
		state = muted("on · resumes with the work hours", "on · paused")
	case s.Paused == session.PauseBattery:
		state = muted("on · resumes with the battery", "on · paused")
	case a.State == activity.StateWaitingIdle:
		spark = true
		idle := st.Muted.Render(shortDuration(a.Idle))
		if need := m.dash.idleNeed; need > 0 {
			idle = st.Muted.Render(shortDuration(a.Idle) + " / " + shortDuration(need))
			state = waiting("waiting for idle") + " " + st.meter(float64(a.Idle)/float64(need), 8, st.Muted) + " " + idle
		}
		state = c.pick("", state, waiting("waiting for idle")+" "+idle, waiting("waiting")+" "+idle)
	case a.State == activity.StateSimulating:
		mark := st.OK
		if m.pulseDim {
			mark = st.Dim
		}
		state, spark = mark.Render(st.g.on)+" simulating", true
	case a.State == activity.StatePausedUser:
		state, spark = c.pick("", waiting("paused — you're using the computer"), waiting("paused — you're active")), true
	case a.State == activity.StatePausedLocked:
		state, spark = waiting("paused — screen locked"), true
	case a.State == activity.StateDegraded:
		state = st.Problem.Bold(true).Render(st.g.problem + " not working")
		reason := a.Reason
		if reason == "" {
			reason = "unknown reason"
		}
		fix = &calloutText{text: reason, fix: a.Hint}
		if !strings.Contains(a.Hint, "60 s") { // the hint may say so itself
			fix.note = "keepalive checks again every 60 s"
		}
	default:
		// Active, but the simulator has not reported yet (StateOff or no
		// state): it is starting, not off.
		state = st.Muted.Render(t("starting…"))
	}
	ago := ""
	if spark && !a.LastBurst.IsZero() {
		ago = st.Muted.Render(span(now.Sub(a.LastBurst)) + " ago")
	}

	if spark && c.level < fitNoSparkline {
		// The sparkline shares the line with a short state, at a fixed
		// column; a longer one (or one that changes width, like the idle
		// time) gets it on the next line, so it never jumps.
		line := st.sparkline(m.dash.bursts.counts(now))
		sw := lipgloss.Width(state)
		col := 15 // three spaces after "◉ simulating", or two when narrow
		if labelWidth+col+sparkBuckets+gapFor(ago)+lipgloss.Width(ago) > c.width {
			col = 14
		}
		if sw <= col-2 && labelWidth+col+sparkBuckets+gapFor(ago)+lipgloss.Width(ago) <= c.width {
			c.spread(st.label("Activity", labelWidth)+state+strings.Repeat(" ", col-sw)+line, ago)
		} else {
			c.labelRow("Activity", state, "")
			c.spread(strings.Repeat(" ", labelWidth)+line, ago)
		}
	} else {
		c.labelRow("Activity", state, ago)
	}
	if fix != nil {
		c.callout(st.Problem, *fix)
	}
}

// batteryRow is the BATTERY row: a meter of the charge, coloured by how
// close it is to the threshold, and when keepalive stops or pauses.
func (m Model) batteryRow(c *canvas, b session.Battery, paused bool) {
	st := m.st
	right := fmt.Sprintf("stops at %d%%", b.Threshold)
	if b.Pause {
		right = fmt.Sprintf("pauses at %d%%", b.Threshold)
		if paused {
			right = "paused — resumes when charging"
			if at := session.BatteryResumeAt(b.Threshold); at > 0 {
				right = fmt.Sprintf("paused — resumes at %d%%", at)
			}
		}
	}
	right = st.Muted.Render(st.text(right))
	if !b.Available {
		c.labelRow("Battery", st.Muted.Render("no reading"), right)
		return
	}
	pct := fmt.Sprintf(" %d%%", b.Percent)
	meterW := min(20, c.width-labelWidth-len(pct)-1-lipgloss.Width(right))
	if meterW < 6 { // the note goes on a line of its own
		meterW = max(min(20, c.width-labelWidth-len(pct)), 1)
	}
	tone := st.OK
	switch {
	case b.Percent <= b.Threshold:
		tone = st.Problem
	case b.Percent <= b.Threshold+10:
		tone = st.Warn
	}
	c.labelRow("Battery", st.meter(float64(b.Percent)/100, meterW, tone)+pct, right)
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

// holdWhat is what the hold keeps awake.
func holdWhat(s session.Snapshot) string {
	if s.KeepDisplay {
		return "system + display"
	}
	return "system"
}

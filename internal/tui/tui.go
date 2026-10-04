// Package tui is keepalive's interactive terminal UI. Home chooses how long
// to keep the computer awake and with which options, the input screens take
// a duration, a time, work hours or a battery level, and the Dashboard
// shows a running session: one this process runs, or one running in
// another keepalive process (attach mode).
package tui

import (
	"context"
	"errors"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// Options configures New.
type Options struct {
	Version string
	// Context cancels running sessions (signals); nil means Background.
	Context context.Context
	// Deps are passed to every session the UI starts.
	Deps session.Deps
	// Base is the resolved configuration (flags, environment, file): the
	// defaults of Home's options and the template of every session.
	Base session.Config
	// Start begins a session from Base at once (keepalive -d 30); the UI
	// then quits when that session ends on its own.
	Start bool
	// OnSession, when set, is told about every session the UI starts, and
	// nil when it ends (the CLI's control socket follows it).
	OnSession func(*session.Session)
	// Renderer styles the UI (see NewRenderer); nil means plain text.
	Renderer *lipgloss.Renderer
	// Look says whether the UI may use colours and Unicode symbols.
	Look Look

	// Claim makes this process the running keepalive; the UI calls it
	// before starting a session unless Claimed is set. When another
	// keepalive runs a session, Claim returns a controller attached to it;
	// when it cannot claim, an error (a Warning carries a fix). nil means
	// there is no single-instance rule.
	Claim   func() (Controller, error)
	Claimed bool
	// Attach opens the Dashboard on a keepalive running in another process.
	Attach Controller
	// Warning is shown on Home, e.g. why the claim failed.
	Warning error

	// LogPath is the log file; LogEnabled says whether it is written.
	LogPath    string
	LogEnabled bool

	// Battery reads the battery; nil means the platform's.
	Battery func() (platform.BatteryStatus, error)
	// ActivityProblem explains why activity simulation cannot work here
	// ("" when it can); nil means activity.Diagnose, once per process.
	ActivityProblem func() (reason, hint string)
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	// start runs a session; tests replace it.
	start func(session.Config) Controller
}

// Warning is a problem shown on Home, with an optional fix.
type Warning struct{ Text, Fix string }

func (w Warning) Error() string { return w.Text }

func asWarning(err error) *Warning {
	var w Warning
	if errors.As(err, &w) {
		return &w
	}
	return &Warning{Text: err.Error()}
}

// diagnoseActivity runs the static activity checks once per process.
var diagnoseActivity = sync.OnceValues(func() (string, string) {
	return activity.Diagnose().Problem()
})

type screen int

const (
	screenHome screen = iota
	screenInput
	screenDashboard
)

// Model is the UI. Copies share the running session through slot.
type Model struct {
	version string
	st      Styles
	now     func() time.Time
	battery func() (platform.BatteryStatus, error)
	problem func() (string, string)
	logPath string
	logOn   bool

	ctx     context.Context
	base    session.Config
	start   func(session.Config) Controller
	claim   func() (Controller, error)
	claimed bool
	slot    *slot

	width  int
	height int
	screen screen
	help   bool
	tick   int // generation of the dashboard's 1 s tick chain

	// The pulse of the simulating mark: its tick's generation, whether the
	// tick runs, and whether the mark is dim now.
	pulseGen          int
	pulsing, pulseDim bool

	home  homeState
	input inputState
	dash  dashState

	final string // why an autostarted session ended; printed after exit
}

// New returns the UI on Home, on the Dashboard of a session started from
// o.Base (o.Start), or attached to o.Attach.
func New(o Options) Model {
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	m := Model{
		version: o.Version,
		st:      NewStyles(o.Renderer, o.Look),
		now:     o.Now,
		battery: o.Battery,
		problem: o.ActivityProblem,
		logPath: o.LogPath,
		logOn:   o.LogEnabled,
		ctx:     ctx,
		base:    o.Base,
		start:   o.start,
		claim:   o.Claim,
		claimed: o.Claimed || o.Claim == nil,
		slot:    &slot{onSession: o.OnSession},
		home:    newHome(o.Base),
		input:   newInputs(),
	}
	if m.now == nil {
		m.now = time.Now
	}
	if m.battery == nil {
		m.battery = platform.GetBatteryStatus
	}
	if m.problem == nil {
		m.problem = diagnoseActivity
	}
	if m.start == nil {
		deps := o.Deps
		m.start = func(cfg session.Config) Controller { return startLocal(ctx, cfg, deps) }
	}
	if o.Warning != nil {
		m.home.instance = asWarning(o.Warning)
	}
	switch {
	case o.Attach != nil:
		m = m.openDashboard(o.Attach, 0)
		if o.Start {
			m = m.flash(flashWarn, "Not started: another keepalive is already running (start with --replace to take over)", 10*time.Second)
		}
	case o.Start && m.claimed:
		m = m.openDashboard(m.start(o.Base), activityIdle(o.Base))
		m.dash.auto = true
	case o.Start:
		m.home.problem = &Warning{Text: "Not started: another keepalive holds the lock"}
	}
	return m
}

// Shutdown stops the session this process runs, or detaches from an
// attached one, and waits for the power hold to be released. Call it after
// the program has exited.
func (m Model) Shutdown() error {
	if m.slot == nil {
		return nil
	}
	return m.slot.release(m.slot.current())
}

// FinalMessage says why a session started with Options.Start ended on its
// own ("" otherwise), for printing after the UI has closed.
func (m Model) FinalMessage() string { return m.final }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{diagCmd(m.problem), batteryCmd(m.battery)}
	if m.screen == screenDashboard {
		cmds = append(cmds, m.dashCmds()...)
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	tm, cmd := m.update(msg)
	mm, ok := tm.(Model)
	if !ok {
		return tm, cmd
	}
	mm, pulse := mm.syncPulse()
	return mm, tea.Batch(cmd, pulse)
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case pulseMsg:
		return m.onPulse(msg)
	case tea.KeyMsg:
		return m.key(msg)
	case diagMsg:
		m.home.activityReason, m.home.activityHint = msg.reason, msg.hint
		return m, nil
	case batteryMsg:
		m.home.battery = msg
		return m, nil
	case claimMsg:
		return m.onClaim(msg)
	case tickMsg, snapMsg, eventMsg, actionMsg, stopDoneMsg, returnMsg:
		return m.dashUpdate(msg)
	}
	if m.screen == screenInput {
		return m.inputUpdate(msg)
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.help {
		return m.helpView()
	}
	switch m.screen {
	case screenInput:
		return m.inputView()
	case screenDashboard:
		return m.dashView()
	}
	return m.homeView()
}

// key routes a key press. Ctrl+C quits from anywhere: a local session is
// stopped by Shutdown, an attached one keeps running.
func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.help {
		switch msg.String() {
		case "?", "esc", "q":
			m.help = false
		}
		return m, nil
	}
	switch m.screen {
	case screenInput:
		return m.inputKey(msg)
	case screenDashboard:
		return m.dashKey(msg)
	}
	return m.homeKey(msg)
}

// startSession runs cfg in this process, claiming the instance first when
// needed.
func (m Model) startSession(cfg session.Config) (Model, tea.Cmd) {
	if !m.claimed {
		return m, claimCmd(m.claim, &cfg)
	}
	m.home.problem, m.home.instance = nil, nil
	m = m.openDashboard(m.start(cfg), activityIdle(cfg))
	return m, tea.Batch(m.dashCmds()...)
}

// onClaim applies the result of Claim: this process may now start a
// session, another one runs (attach to it), or it holds the claim and keeps
// nothing awake (say so on Home).
func (m Model) onClaim(msg claimMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.home.instance = asWarning(msg.err)
		return m, nil
	case msg.c != nil:
		if m.screen == screenDashboard { // a session started meanwhile; keep it
			_ = msg.c.Close()
			return m, nil
		}
		m.home.instance = nil
		m = m.openDashboard(msg.c, 0)
		if msg.start != nil {
			m = m.flash(flashWarn, "Not started: another keepalive is already running; this is it", 10*time.Second)
		}
		return m, tea.Batch(m.dashCmds()...)
	}
	m.claimed = true
	m.home.instance = nil
	if msg.start != nil && m.screen != screenDashboard {
		return m.startSession(*msg.start)
	}
	return m, nil
}

// ---- messages and commands ----

type diagMsg struct{ reason, hint string }

type batteryMsg struct {
	status platform.BatteryStatus
	err    error
	read   bool
}

type claimMsg struct {
	c     Controller
	err   error
	start *session.Config // the session to start once claimed
}

func diagCmd(problem func() (string, string)) tea.Cmd {
	return func() tea.Msg {
		reason, hint := problem()
		return diagMsg{reason: reason, hint: hint}
	}
}

func batteryCmd(read func() (platform.BatteryStatus, error)) tea.Cmd {
	return func() tea.Msg {
		st, err := read()
		return batteryMsg{status: st, err: err, read: true}
	}
}

func claimCmd(claim func() (Controller, error), start *session.Config) tea.Cmd {
	return func() tea.Msg {
		c, err := claim()
		return claimMsg{c: c, err: err, start: start}
	}
}

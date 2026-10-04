package tui

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/timer"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/stigoleg/keep-alive/v2/internal/session"

	tea "github.com/charmbracelet/bubbletea"
)

const defaultTerminalWidth = 80

// stopTimeout bounds how long stopping a session from the TUI waits for the
// power hold to be released.
const stopTimeout = 10 * time.Second

// state represents the different states of the TUI.
type state int

const (
	stateMenu state = iota
	stateTimedInput
	stateClockInput
	stateBatteryInput
	stateRunning
)

// Model holds the current state of the UI, including user input and keep-alive state.
type Model struct {
	State              state
	Selected           int
	textInput          textinput.Model
	ErrorMessage       string
	StartTime          time.Time
	Duration           time.Duration
	Clock              time.Time
	ShowHelp           bool
	ShowDependencyInfo bool
	DependencyWarning  string
	ActivityWarning    string
	version            string
	Keys               KeyMap
	Help               help.Model
	HelpViewport       viewport.Model
	timer              timer.Model
	progress           progress.Model
	SimulateActivity   bool
	BatteryThreshold   int
	BatteryPercentage  int
	BatteryError       string
	Width              int
	Height             int

	// Session wiring. sessions is a pointer so every copy of the model that
	// bubbletea makes shares the running session.
	ctx      context.Context
	deps     session.Deps
	base     session.Config
	sessions *sessionSlot
}

// Options configures New.
type Options struct {
	Version string
	// Context cancels running sessions (signals); nil means Background.
	Context context.Context
	// Deps are passed to every session the TUI starts.
	Deps session.Deps
	// Base is the session template: activity tuning, display, battery
	// threshold and limits from the command line.
	Base session.Config
	// Start begins a session from Base immediately.
	Start bool
}

// InitialModel returns the initial model for the TUI.
func InitialModel() Model {
	return New(Options{})
}

// New returns a model in the menu, or already running when o.Start is set.
func New(o Options) Model {
	ctx := o.Context
	if ctx == nil {
		ctx = context.Background()
	}
	m := Model{
		State:              stateMenu,
		Selected:           0,
		textInput:          newMinutesTextInput(),
		ShowHelp:           false,
		ShowDependencyInfo: false,
		DependencyWarning:  "",
		ActivityWarning:    "",
		version:            o.Version,
		Keys:               DefaultKeys(),
		Help:               NewHelpModel(),
		HelpViewport:       newHelpViewport(defaultTerminalWidth, 20),
		progress:           progress.New(progress.WithDefaultGradient(), progress.WithWidth(34)),
		SimulateActivity:   o.Base.Active,
		BatteryThreshold:   o.Base.BatteryThreshold,
		Width:              defaultTerminalWidth,
		ctx:                ctx,
		deps:               o.Deps,
		base:               o.Base,
		sessions:           &sessionSlot{},
	}
	if o.Start {
		dur := o.Base.Duration
		if !o.Base.Until.IsZero() {
			dur = time.Until(o.Base.Until)
		}
		if dur > 0 {
			m.textInput.SetValue(strconv.Itoa(int(dur.Minutes())))
		}
		m, _ = startSession(m, dur, o.Base.Until)
	}
	return m
}

// Shutdown stops the running session, if any, and waits for it to release
// the power hold. Call it after the program exits.
func (m Model) Shutdown() error {
	if m.sessions == nil {
		return nil
	}
	return m.sessions.stop()
}

// sessionSlot holds the session the TUI is currently running.
type sessionSlot struct {
	mu  sync.Mutex
	cur *runner
}

func (s *sessionSlot) set(r *runner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = r
}

func (s *sessionSlot) current() *runner {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// stop ends the current session (if any) and clears the slot.
func (s *sessionSlot) stop() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	r := s.cur
	s.cur = nil
	s.mu.Unlock()
	if r == nil {
		return nil
	}
	return r.stop()
}

// runner is one session started by the TUI.
type runner struct {
	sess   *session.Session
	events <-chan session.Event
	unsub  func()
	done   chan session.Result
}

func startRunner(ctx context.Context, cfg session.Config, deps session.Deps) *runner {
	s := session.New(cfg, deps)
	events, unsub := s.Subscribe()
	r := &runner{sess: s, events: events, unsub: unsub, done: make(chan session.Result, 1)}
	go func() { r.done <- s.Run(ctx) }()
	return r
}

// stop asks the session to end and waits until Run has returned.
func (r *runner) stop() error {
	r.sess.Stop(session.ReasonUser)
	defer r.unsub()
	select {
	case res := <-r.done:
		r.done <- res // keep the result for anyone else waiting
		return res.Err
	case <-time.After(stopTimeout):
		return context.DeadlineExceeded
	}
}

// sessionEventMsg carries one session event into Update.
type sessionEventMsg struct {
	r  *runner
	ev session.Event
}

// waitForEvent delivers the runner's next event; nil once the stream ends.
func waitForEvent(r *runner) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-r.events
		if !ok {
			return nil
		}
		return sessionEventMsg{r: r, ev: ev}
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	if m.State == stateRunning {
		return runningCommands(m)
	}
	return nil
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := Update(msg, m)
	return newModel, cmd
}

// View implements tea.Model
func (m Model) View() string {
	return View(m)
}

// TimeRemaining returns the remaining duration for timed keep-alive
func (m Model) TimeRemaining() time.Duration {
	if m.State != stateRunning {
		return 0
	}
	elapsed := time.Since(m.StartTime)
	remaining := m.Duration - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// SetVersion sets the version for the help text
func (m *Model) SetVersion(version string) {
	m.version = version
}

// Version returns the current version
func (m Model) Version() string {
	return m.version
}

// SetDependencyWarning sets the dependency warning message
func (m *Model) SetDependencyWarning(message string) {
	m.DependencyWarning = message
}

func (m *Model) SetActivityWarning(message string) {
	m.ActivityWarning = message
}

// newMinutesTextInput constructs a focused text input configured for minute entry.
func newMinutesTextInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "e.g. 30 or 2h30m"
	ti.CharLimit = 16
	ti.Width = 20
	ti.Focus()
	return ti
}

func newClockTextInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "e.g. 22:00 or 10:00PM"
	ti.CharLimit = 16
	ti.Width = 24
	ti.Focus()
	return ti
}

func newBatteryTextInput(threshold int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "e.g. 20"
	ti.CharLimit = 3
	ti.Width = 12
	if threshold > 0 {
		ti.SetValue(strconv.Itoa(threshold))
	}
	ti.Focus()
	return ti
}

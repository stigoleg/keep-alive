package ui

import (
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/timer"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/stigoleg/keep-alive/internal/keepalive"
	"github.com/stigoleg/keep-alive/internal/platform"

	tea "github.com/charmbracelet/bubbletea"
)

const batteryPollInterval = 30 * time.Second

const defaultTerminalWidth = 80

// state represents the different states of the TUI.
type state int

const (
	stateMenu state = iota
	stateTimedInput
	stateRunning
)

// Model holds the current state of the UI, including user input and keep-alive state.
type Model struct {
	State              state
	Selected           int
	textInput          textinput.Model
	KeepAlive          *keepalive.Keeper
	ErrorMessage       string
	StartTime          time.Time
	Duration           time.Duration
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
}

// InitialModel returns the initial model for the TUI.
func InitialModel() Model {
	return Model{
		State:              stateMenu,
		Selected:           0,
		textInput:          newMinutesTextInput(),
		KeepAlive:          keepalive.NewKeeper(),
		ShowHelp:           false,
		ShowDependencyInfo: false,
		DependencyWarning:  "",
		ActivityWarning:    "",
		Keys:               DefaultKeys(),
		Help:               NewHelpModel(),
		HelpViewport:       newHelpViewport(defaultTerminalWidth, 20),
		progress:           progress.New(progress.WithDefaultGradient(), progress.WithWidth(34)),
		SimulateActivity:   false,
		Width:              defaultTerminalWidth,
	}
}

// InitialModelWithDuration returns a model initialized with a specific duration and starts running.
func InitialModelWithDuration(minutes int, simulateActivity bool) Model {
	return InitialModelWithLimits(minutes, 0, platform.BatteryStatus{}, simulateActivity)
}

// InitialModelWithBattery returns a model initialized in battery threshold mode.
func InitialModelWithBattery(threshold int, status platform.BatteryStatus, simulateActivity bool) Model {
	return InitialModelWithLimits(0, threshold, status, simulateActivity)
}

// InitialModelWithLimits returns a model initialized with any active runtime limits.
func InitialModelWithLimits(minutes int, threshold int, status platform.BatteryStatus, simulateActivity bool) Model {
	m := InitialModel()
	m.SimulateActivity = simulateActivity
	if minutes > 0 {
		m.textInput.SetValue(strconv.Itoa(minutes))
		m.Duration = time.Duration(minutes) * time.Minute
		m.timer = timer.NewWithInterval(m.Duration, time.Second/10)
	}
	if threshold > 0 {
		m.BatteryThreshold = threshold
		m.BatteryPercentage = status.Percentage
	}

	m.State = stateRunning
	m.StartTime = time.Now()

	m.KeepAlive.SetSimulateActivity(simulateActivity)
	var err error
	if m.Duration > 0 {
		err = m.KeepAlive.StartTimed(m.Duration)
	} else {
		err = m.KeepAlive.StartIndefinite()
	}
	if err != nil {
		m.ErrorMessage = err.Error()
		m.State = stateMenu
		return m
	}

	return m
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	if m.State == stateRunning {
		var cmds []tea.Cmd
		if m.Duration > 0 {
			cmds = append(cmds, m.timer.Init(), m.progress.SetPercent(0))
		}
		if m.BatteryThreshold > 0 {
			cmds = append(cmds, batteryPollCmd())
		}
		if len(cmds) > 0 {
			return tea.Batch(cmds...)
		}
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

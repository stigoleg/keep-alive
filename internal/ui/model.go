package ui

import (
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/timer"
	"github.com/stigoleg/keep-alive/internal/keepalive"

	tea "github.com/charmbracelet/bubbletea"
)

// state represents the different states of the TUI.
type state int

const (
	stateMenu state = iota
	stateTimedInput
	stateRunning
)

// SimulationStatus represents the current state of activity simulation
type SimulationStatus int

const (
	SimulationStatusUnknown SimulationStatus = iota
	SimulationStatusAvailable
	SimulationStatusUnavailable
	SimulationStatusActive
	SimulationStatusFailed
)

// UI layout constants.
const (
	progressBarWidth = 34
)

// Model holds the current state of the UI, including user input and keep-alive state.
type Model struct {
	State              state
	Selected           int
	textInput          textinput.Model
	durationStringMode bool
	KeepAlive          *keepalive.Keeper
	ErrorMessage       string
	StartTime          time.Time
	Duration           time.Duration
	ShowHelp           bool
	ShowDependencyInfo bool
	DependencyWarning  string
	version            string
	Keys               KeyMap
	Help               help.Model
	timer              timer.Model
	progress           progress.Model
	SimulateActivity   bool

	// Simulation capability and status
	SimulationCapable     bool             // Whether simulation can work on this system
	SimulationStatus      SimulationStatus // Current simulation state during runtime
	SimulationMessage     string           // Error/instruction message if not capable
	SimulationCanPrompt   bool             // Whether we can trigger a system permission dialog
	ShowSimulationWarning bool             // Show blocking warning dialog when --active fails
}

// InitialModel returns the initial model for the TUI.
func InitialModel() Model {
	return Model{
		State:                 stateMenu,
		Selected:              0,
		textInput:             newMinutesTextInput(),
		durationStringMode:    false,
		KeepAlive:             &keepalive.Keeper{},
		ShowHelp:              false,
		ShowDependencyInfo:    false,
		DependencyWarning:     "",
		Keys:                  DefaultKeys(),
		Help:                  NewHelpModel(),
		progress:              progress.New(progress.WithDefaultGradient(), progress.WithWidth(progressBarWidth)),
		SimulateActivity:      false,
		SimulationCapable:     true, // Assume capable until checked
		SimulationStatus:      SimulationStatusUnknown,
		SimulationMessage:     "",
		SimulationCanPrompt:   false,
		ShowSimulationWarning: false,
	}
}

// InitialModelWithDuration returns a model initialized with a specific duration and starts running.
func InitialModelWithDuration(minutes int, simulateActivity bool) Model {
	m := InitialModel()
	m.SimulateActivity = simulateActivity
	m.textInput.SetValue(strconv.Itoa(minutes))
	m.State = stateRunning
	m.StartTime = time.Now()
	m.Duration = time.Duration(minutes) * time.Minute
	m.timer = timer.NewWithInterval(m.Duration, time.Second/10)

	// Set simulate activity before starting
	m.KeepAlive.SetSimulateActivity(simulateActivity)

	// Start the keep-alive process
	err := m.KeepAlive.StartTimed(time.Duration(minutes) * time.Minute)
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
		if m.Duration > 0 {
			return tea.Batch(
				m.timer.Init(),
				m.progress.SetPercent(0),
			)
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

// SetSimulationCapability sets the simulation capability status
func (m *Model) SetSimulationCapability(capable bool, message string, canPrompt bool) {
	m.SimulationCapable = capable
	m.SimulationMessage = message
	m.SimulationCanPrompt = canPrompt
	if capable {
		m.SimulationStatus = SimulationStatusAvailable
	} else {
		m.SimulationStatus = SimulationStatusUnavailable
	}
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

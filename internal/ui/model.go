package ui

import (
	"strconv"
	"time"

	"github.com/stigoleg/keep-alive/internal/keepalive"

	tea "github.com/charmbracelet/bubbletea"
)

// state represents the different states of the TUI.
type state int

const (
	stateMenu state = iota
	stateTimedInput
	stateRunning
	stateHelp
)

// Model holds the current state of the UI, including user input and keep-alive state.
type Model struct {
	State        state
	Selected     int
	Input        string
	KeepAlive    *keepalive.Keeper
	ErrorMessage string
	StartTime    time.Time
	Duration     time.Duration
	ShowHelp     bool
}

// InitialModel returns the initial model for the TUI.
func InitialModel() Model {
	return Model{
		State:     stateMenu,
		Selected:  0,
		Input:     "",
		KeepAlive: &keepalive.Keeper{},
		ShowHelp:  false,
	}
}

// InitialModelWithDuration returns a model initialized with a specific duration and starts running.
func InitialModelWithDuration(minutes int) Model {
	m := InitialModel()
	m.Input = strconv.Itoa(minutes)
	m.State = stateRunning
	m.StartTime = time.Now()
	m.Duration = time.Duration(minutes) * time.Minute

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
		return tick()
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

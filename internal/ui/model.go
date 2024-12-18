package ui

import (
	"keepalive/internal/keepalive"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// state represents the different states of the TUI.
type state int

const (
	stateMenu state = iota
	stateTimedInput
	stateRunning
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
}

// InitialModel returns the initial model for the TUI.
func InitialModel() Model {
	return Model{
		State:     stateMenu,
		Selected:  0,
		Input:     "",
		KeepAlive: &keepalive.Keeper{},
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
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
	if !m.KeepAlive.IsRunning() || m.Duration == 0 {
		return 0
	}
	elapsed := time.Since(m.StartTime)
	remaining := m.Duration - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

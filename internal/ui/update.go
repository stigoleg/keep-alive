package ui

import (
	"strconv"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// tickMsg is sent when the countdown timer ticks
type tickMsg time.Time

// Update handles messages and updates the model accordingly.
func Update(msg tea.Msg, m Model) (Model, tea.Cmd) {
	if m.ShowHelp {
		return handleHelpState(msg, m)
	}

	switch m.State {
	case stateMenu:
		return handleMenuState(msg, m)
	case stateTimedInput:
		return handleTimedInputState(msg, m)
	case stateRunning:
		return handleRunningState(msg, m)
	}

	return m, nil
}

// handleHelpState handles messages when help is being displayed
func handleHelpState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "esc", "ctrl+c":
			m.ShowHelp = false
		}
	}
	return m, nil
}

// handleMenuState handles messages in the menu state
func handleMenuState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleMenuKeyMsg(msg, m)
	}
	return m, nil
}

// handleMenuKeyMsg handles keyboard input in the menu state
func handleMenuKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch msg.String() {
	case "h", "?":
		m.ShowHelp = true
	case "up", "k":
		if m.Selected > 0 {
			m.Selected--
		}
	case "down", "j":
		if m.Selected < 2 {
			m.Selected++
		}
	case "enter", " ":
		return handleMenuSelection(m)
	case "q", "ctrl+c":
		return handleQuit(m)
	}
	return m, nil
}

// handleMenuSelection processes the selected menu item
func handleMenuSelection(m Model) (Model, tea.Cmd) {
	switch m.Selected {
	case 0:
		if err := m.KeepAlive.StartIndefinite(); err != nil {
			m.ErrorMessage = err.Error()
			return m, nil
		}
		m.State = stateRunning
		m.Duration = 0
	case 1:
		m.State = stateTimedInput
		m.Input = ""
		m.ErrorMessage = ""
	case 2:
		return handleQuit(m)
	}
	return m, nil
}

// handleTimedInputState handles messages in the timed input state
func handleTimedInputState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleTimedInputKeyMsg(msg, m)
	}
	return m, nil
}

// handleTimedInputKeyMsg handles keyboard input in the timed input state
func handleTimedInputKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.State = stateMenu
		m.Input = ""
		m.ErrorMessage = ""
	case "enter":
		return handleTimedInputSubmit(m)
	case "backspace":
		if len(m.Input) > 0 {
			m.Input = m.Input[:len(m.Input)-1]
		}
	default:
		if len(msg.String()) == 1 && unicode.IsDigit(rune(msg.String()[0])) {
			m.Input += msg.String()
		}
	}
	return m, nil
}

// handleTimedInputSubmit processes the submitted duration
func handleTimedInputSubmit(m Model) (Model, tea.Cmd) {
	if m.Input == "" {
		m.ErrorMessage = "Duration Required • Please enter the number of minutes"
		return m, nil
	}

	minutes, err := strconv.Atoi(m.Input)
	if err != nil || minutes <= 0 {
		m.ErrorMessage = "Invalid Input • Please enter a positive number"
		return m, nil
	}

	if err := m.KeepAlive.StartTimed(time.Duration(minutes) * time.Minute); err != nil {
		m.ErrorMessage = "System Error • " + err.Error()
		return m, nil
	}

	m.State = stateRunning
	m.StartTime = time.Now()
	m.Duration = time.Duration(minutes) * time.Minute
	m.ErrorMessage = "" // Clear any previous error message
	return m, tick()
}

// handleRunningState handles messages in the running state
func handleRunningState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleRunningKeyMsg(msg, m)
	case tickMsg:
		return handleTick(m)
	}
	return m, nil
}

// handleRunningKeyMsg handles keyboard input in the running state
func handleRunningKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return handleQuit(m)
	case "enter":
		if err := m.KeepAlive.Stop(); err != nil {
			m.ErrorMessage = err.Error()
			return m, nil
		}
		m.State = stateMenu
		m.ErrorMessage = ""
	}
	return m, nil
}

// handleTick processes timer ticks in the running state
func handleTick(m Model) (Model, tea.Cmd) {
	if m.Duration > 0 && time.Since(m.StartTime) >= m.Duration {
		if err := m.KeepAlive.Stop(); err != nil {
			m.ErrorMessage = err.Error()
			return m, nil
		}
		return m, tea.Quit
	}
	return m, tick()
}

// handleQuit handles quitting the application
func handleQuit(m Model) (Model, tea.Cmd) {
	if m.KeepAlive.IsRunning() {
		if err := m.KeepAlive.Stop(); err != nil {
			m.ErrorMessage = err.Error()
			return m, nil
		}
	}
	return m, tea.Quit
}

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

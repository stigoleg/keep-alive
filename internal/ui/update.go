package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stigoleg/keep-alive/internal/util"
)

// tickMsg is sent when the countdown timer ticks
type tickMsg time.Time

// Update handles messages and updates the model accordingly.
func Update(msg tea.Msg, m Model) (Model, tea.Cmd) {
	if m.ShowHelp {
		// Still process timer messages so progress and timeout continue under the overlay
		switch msg.(type) {
		case timer.TickMsg, timer.TimeoutMsg:
			return handleRunningState(msg, m)
		}
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
		switch {
		case key.Matches(keyMsg, m.Keys.ToggleHelp):
			m.ShowHelp = false
		case key.Matches(keyMsg, m.Keys.Quit):
			m.ShowHelp = false
		case key.Matches(keyMsg, m.Keys.Back):
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
	switch {
	case key.Matches(msg, m.Keys.ToggleHelp):
		m.ShowHelp = true
	case key.Matches(msg, m.Keys.Up):
		if m.Selected > 0 {
			m.Selected--
		}
	case key.Matches(msg, m.Keys.Down):
		if m.Selected < 2 {
			m.Selected++
		}
	case key.Matches(msg, m.Keys.Select):
		return handleMenuSelection(m)
	case msg.Type == tea.KeyEnter:
		// Fallback for tests sending KeyEnter type
		return handleMenuSelection(m)
	case key.Matches(msg, m.Keys.Quit):
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
		m.ErrorMessage = ""
		m.textInput = newMinutesTextInput()
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
	switch {
	case key.Matches(msg, m.Keys.Back):
		m.State = stateMenu
		m.ErrorMessage = ""
		return m, nil
	case key.Matches(msg, m.Keys.Submit) || msg.Type == tea.KeyEnter:
		return handleTimedInputSubmit(m)
	}

	// Route remaining input to text input component (allow letters like h/m)
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	// Minimal realtime feedback
	value := m.textInput.Value()
	if value == "" {
		m.ErrorMessage = "Duration Required • Enter minutes or duration (e.g., 30 or 2h30m)"
	} else {
		m.ErrorMessage = ""
	}

	return m, cmd
}

// handleTimedInputSubmit processes the submitted duration
func handleTimedInputSubmit(m Model) (Model, tea.Cmd) {
	value := m.textInput.Value()
	if value == "" {
		m.ErrorMessage = "Duration Required • Enter minutes or duration (e.g., 30 or 2h30m)"
		return m, nil
	}

	dur, err := util.ParseDuration(value)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	if dur <= 0 {
		m.ErrorMessage = "Invalid Input • Please enter a positive number"
		return m, nil
	}

	if err := m.KeepAlive.StartTimed(dur); err != nil {
		m.ErrorMessage = "System Error • " + err.Error()
		return m, nil
	}

	m.State = stateRunning
	m.StartTime = time.Now()
	m.Duration = dur
	m.timer = timer.NewWithInterval(dur, time.Second/10)
	m.ErrorMessage = "" // Clear any previous error message
	return m, tea.Batch(
		m.timer.Init(),
		m.progress.SetPercent(0),
	)
}

// handleRunningState handles messages in the running state
func handleRunningState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	// Always let progress process messages so SetPercent's internal messages are applied
	var cmds []tea.Cmd
	if pm := msg; pm != nil {
		var pc tea.Cmd
		var newProg tea.Model
		newProg, pc = m.progress.Update(pm)
		if pc != nil {
			cmds = append(cmds, pc)
		}
		if np, ok := newProg.(progress.Model); ok {
			m.progress = np
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleRunningKeyMsg(msg, m)
	case tickMsg:
		return handleTick(m)
	case timer.TickMsg:
		var tcmd tea.Cmd
		m.timer, tcmd = m.timer.Update(msg)
		if tcmd != nil {
			cmds = append(cmds, tcmd)
		}
		if m.Duration > 0 {
			remaining := time.Until(m.StartTime.Add(m.Duration))
			if remaining < 0 {
				remaining = 0
			}
			percent := 1 - (float64(remaining) / float64(m.Duration))
			if percent < 0 {
				percent = 0
			}
			if percent > 1 {
				percent = 1
			}
			cmds = append(cmds, m.progress.SetPercent(percent))
		}
		return m, tea.Batch(cmds...)
	case timer.TimeoutMsg:
		return handleQuit(m)
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleRunningKeyMsg handles keyboard input in the running state
func handleRunningKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Quit):
		return handleQuit(m)
	case key.Matches(msg, m.Keys.ToggleHelp):
		m.ShowHelp = true
	case key.Matches(msg, m.Keys.Stop):
		return handleStopAndReturn(m)
	}
	return m, nil
}

// handleTick processes timer ticks in the running state
func handleTick(m Model) (Model, tea.Cmd) {
	if m.Duration > 0 && time.Since(m.StartTime) >= m.Duration {
		return handleQuit(m)
	}
	return m, nil
}

// cleanup stops the keep-alive process and resets the model state
func cleanup(m Model) (Model, error) {
	if err := m.KeepAlive.Stop(); err != nil {
		return m, err
	}

	// Reset all state
	m.State = stateMenu
	m.Duration = 0
	m.StartTime = time.Time{}
	m.ErrorMessage = ""
	// Reset timer and progress models
	m.timer = timer.Model{}
	m.progress = progress.New(progress.WithDefaultGradient(), progress.WithWidth(64))

	return m, nil
}

// handleStopAndReturn stops the keep-alive and returns to the main menu
func handleStopAndReturn(m Model) (Model, tea.Cmd) {
	cleanedModel, err := cleanup(m)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	return cleanedModel, nil
}

// handleQuit handles quitting the application
func handleQuit(m Model) (Model, tea.Cmd) {
	cleanedModel, err := cleanup(m)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	return cleanedModel, tea.Quit
}

func tick() tea.Cmd { // deprecated
	return nil
}

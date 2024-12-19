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
	var cmd tea.Cmd

	// Handle help state first
	if m.ShowHelp {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "esc", "ctrl+c":
				m.ShowHelp = false
				return m, nil
			}
		}
		return m, nil
	}

	switch m.State {
	case stateMenu:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "h", "?":
				m.ShowHelp = true
				return m, nil
			case "up", "k":
				if m.Selected > 0 {
					m.Selected--
				}
			case "down", "j":
				if m.Selected < 2 {
					m.Selected++
				}
			case "enter", " ":
				switch m.Selected {
				case 0:
					// Indefinite keep-alive
					if err := m.KeepAlive.StartIndefinite(); err != nil {
						m.ErrorMessage = err.Error()
						return m, nil
					}
					m.State = stateRunning
					m.Duration = 0 // Clear any previous duration
					return m, nil
				case 1:
					// Timed input
					m.State = stateTimedInput
					m.Input = ""
					m.ErrorMessage = ""
					return m, nil
				case 2:
					// Quit keep-alive
					if m.KeepAlive.IsRunning() {
						if err := m.KeepAlive.Stop(); err != nil {
							m.ErrorMessage = err.Error()
							return m, nil
						}
					}
					return m, tea.Quit
				}
			case "q", "ctrl+c":
				if m.KeepAlive.IsRunning() {
					if err := m.KeepAlive.Stop(); err != nil {
						m.ErrorMessage = err.Error()
						return m, nil
					}
				}
				return m, tea.Quit
			}
		}

	case stateTimedInput:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.State = stateMenu
				m.Input = ""
				m.ErrorMessage = ""
			case "enter":
				if m.Input == "" {
					m.ErrorMessage = "Please enter a duration in minutes"
					return m, nil
				}
				minutes, err := strconv.Atoi(m.Input)
				if err != nil || minutes <= 0 {
					m.ErrorMessage = "Please enter a valid positive number"
					return m, nil
				}
				if err := m.KeepAlive.StartTimed(time.Duration(minutes) * time.Minute); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				m.State = stateRunning
				m.StartTime = time.Now()
				m.Duration = time.Duration(minutes) * time.Minute
				return m, tick()
			case "backspace":
				if len(m.Input) > 0 {
					m.Input = m.Input[:len(m.Input)-1]
				}
			default:
				if len(msg.String()) == 1 && unicode.IsDigit(rune(msg.String()[0])) {
					m.Input += msg.String()
				}
			}
		}

	case stateRunning:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				if err := m.KeepAlive.Stop(); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				return m, tea.Quit
			case "enter":
				if err := m.KeepAlive.Stop(); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				m.State = stateMenu
				m.ErrorMessage = ""
				return m, nil
			}
		case tickMsg:
			if m.Duration > 0 && time.Since(m.StartTime) >= m.Duration {
				if err := m.KeepAlive.Stop(); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				return m, tea.Quit
			}
			return m, tick()
		}
	}

	return m, cmd
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

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

	switch m.State {
	case stateMenu:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
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
			case "q", "esc", "ctrl+c":
				return m, tea.Quit
			}

		case tickMsg:
			if m.Duration > 0 && time.Since(m.StartTime) >= m.Duration {
				if err := m.KeepAlive.Stop(); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				m.State = stateMenu
				m.ErrorMessage = ""
				return m, nil
			}
			return m, tick()
		}

	case stateTimedInput:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				if m.Input == "" {
					m.ErrorMessage = "Please enter a duration"
					return m, nil
				}
				minutes, err := strconv.Atoi(m.Input)
				if err != nil {
					m.ErrorMessage = "Invalid duration"
					return m, nil
				}
				if minutes <= 0 {
					m.ErrorMessage = "Duration must be positive"
					return m, nil
				}
				if err := m.KeepAlive.StartTimed(minutes); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				m.State = stateRunning
				m.StartTime = time.Now()
				m.Duration = time.Duration(minutes) * time.Minute
				m.ErrorMessage = ""
				return m, tick()
			case "esc":
				m.State = stateMenu
				m.ErrorMessage = ""
				return m, nil
			case "backspace":
				if len(m.Input) > 0 {
					m.Input = m.Input[:len(m.Input)-1]
					m.ErrorMessage = ""
				}
				return m, nil
			default:
				if len(msg.String()) == 1 && unicode.IsDigit(rune(msg.String()[0])) {
					if len(m.Input) < 4 { // Limit input to 4 digits
						m.Input += msg.String()
						m.ErrorMessage = ""
					}
				}
				return m, nil
			}
		}

	case stateRunning:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				if err := m.KeepAlive.Stop(); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				m.State = stateMenu
				m.ErrorMessage = ""
				return m, nil
			case "q", "esc", "ctrl+c":
				return m, tea.Quit
			}
		case tickMsg:
			if m.Duration > 0 && time.Since(m.StartTime) >= m.Duration {
				if err := m.KeepAlive.Stop(); err != nil {
					m.ErrorMessage = err.Error()
					return m, nil
				}
				m.State = stateMenu
				m.ErrorMessage = ""
				return m, nil
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

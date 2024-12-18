package ui

import (
	"fmt"
	"strings"
	"time"
)

// View renders the UI based on the current model state.
func View(m Model) string {
	switch m.State {
	case stateMenu:
		return menuView(m)
	case stateTimedInput:
		return timedInputView(m)
	case stateRunning:
		return runningView(m)
	default:
		return "Invalid state"
	}
}

func menuView(m Model) string {
	var b strings.Builder

	// Title
	b.WriteString(Current.Title.Render("Keep Alive Options"))
	b.WriteString("\n\n")

	// Status
	if m.KeepAlive.IsRunning() {
		b.WriteString(Current.ActiveStatus.Render("System is being kept awake"))
	} else {
		b.WriteString(Current.InactiveStatus.Render("System is in normal state"))
	}
	b.WriteString("\n\n")

	// Menu options
	menuItems := []string{
		"Keep system awake indefinitely",
		"Keep system awake for X minutes",
		"Quit keep-alive",
	}

	for i, opt := range menuItems {
		var menuLine strings.Builder

		// Cursor
		if i == m.Selected {
			menuLine.WriteString("> ")
		} else {
			menuLine.WriteString("  ")
		}

		// Option text with styling
		if i == m.Selected {
			menuLine.WriteString(Current.SelectedItem.Render(opt))
		} else if i == 2 && !m.KeepAlive.IsRunning() {
			menuLine.WriteString(Current.DisabledItem.Render(opt))
		} else {
			menuLine.WriteString(Current.Menu.Render(opt))
		}

		b.WriteString(menuLine.String() + "\n")
	}

	if m.ErrorMessage != "" {
		b.WriteString("\n" + Current.Error.Render(m.ErrorMessage))
	}

	b.WriteString("\n\n" + Current.Help.Render("Press j/k or ↑/↓ to select • enter to confirm • q or esc to quit"))
	return b.String()
}

func timedInputView(m Model) string {
	var b strings.Builder

	b.WriteString(Current.Title.Render("Enter Duration"))
	b.WriteString("\n\n")

	input := m.Input
	if input == "" {
		input = " "
	}
	b.WriteString(Current.InputBox.Render(input))
	b.WriteString("\n\n")

	// Help text
	b.WriteString(Current.Help.Render("Enter number of minutes"))
	b.WriteString("\n")
	b.WriteString(Current.Help.Render("Press enter to start • backspace to clear • esc to cancel"))

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + Current.Error.Render(m.ErrorMessage))
	}

	return b.String()
}

func runningView(m Model) string {
	var b strings.Builder

	b.WriteString(Current.Title.Render("Keep Alive Active"))
	b.WriteString("\n\n")

	b.WriteString(Current.ActiveStatus.Render("System is being kept awake"))
	b.WriteString("\n")

	// Show countdown if this is a timed session
	if m.Duration > time.Duration(0) {
		remaining := m.TimeRemaining()
		minutes := int(remaining.Minutes())
		seconds := int(remaining.Seconds()) % 60
		countdown := fmt.Sprintf("%d:%02d remaining", minutes, seconds)
		b.WriteString(Current.Countdown.Render(countdown))
		b.WriteString("\n")
	}

	b.WriteString("\n" + Current.Help.Render("Press enter to stop and return to menu • q or esc to quit"))

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + Current.Error.Render(m.ErrorMessage))
	}

	return b.String()
}

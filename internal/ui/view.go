package ui

import (
	"fmt"
	"strings"
	"time"
)

// View renders the current state of the model to a string.
func View(m Model) string {
	if m.ShowHelp {
		return helpView(m)
	}

	switch m.State {
	case stateMenu:
		return menuView(m)
	case stateTimedInput:
		return timedInputView(m)
	case stateRunning:
		return runningView(m)
	}

	return ""
}

func menuView(m Model) string {
	var b strings.Builder

	b.WriteString(Current.Title.Render("Keep Alive Options"))
	b.WriteString("\n\n")

	b.WriteString(Current.Unselected.Render("Select an option:"))
	b.WriteString("\n\n")

	menuItems := []string{
		"Keep system awake indefinitely",
		"Keep system awake for X minutes",
		"Quit keep-alive",
	}

	for i, opt := range menuItems {
		var menuLine strings.Builder

		if i == m.Selected {
			menuLine.WriteString(Current.Selected.Render("> "))
		} else {
			menuLine.WriteString(Current.Unselected.Render("  "))
		}

		if i == m.Selected {
			menuLine.WriteString(Current.Selected.Render(opt))
		} else {
			menuLine.WriteString(Current.Unselected.Render(opt))
		}

		b.WriteString(menuLine.String() + "\n")
	}

	if m.ErrorMessage != "" {
		b.WriteString("\n" + Current.Error.Render(m.ErrorMessage))
	}

	// contextual help footer
	footer := m.Help.View(m.Keys.ForState(stateMenu))
	b.WriteString("\n\n" + footer)
	return b.String()
}

func timedInputView(m Model) string {
	var b strings.Builder

	b.WriteString(Current.Title.Render("Enter Duration"))
	b.WriteString("\n\n")

	b.WriteString(Current.Unselected.Render("Enter minutes or duration (e.g., 30 or 2h30m):"))
	b.WriteString("\n")

	// Render input component
	inputView := m.textInput.View()
	if strings.TrimSpace(inputView) == "" {
		inputView = " "
	}
	b.WriteString(Current.InputBox.Render(inputView))
	b.WriteString("\n\n")

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + Current.Error.Render(m.ErrorMessage))
	}

	// contextual help footer
	footer := m.Help.View(m.Keys.ForState(stateTimedInput))
	b.WriteString("\n" + footer)

	return b.String()
}

func runningView(m Model) string {
	var b strings.Builder

	b.WriteString(Current.Title.Render("Keep Alive Active"))
	b.WriteString("\n\n")

	b.WriteString(Current.Awake.Render("System is being kept awake"))
	b.WriteString("\n")

	// Show countdown and progress bar if this is a timed session
	if m.Duration > time.Duration(0) {
		remaining := m.TimeRemaining()
		minutes := int(remaining.Minutes())
		seconds := int(remaining.Seconds()) % 60
		countdown := fmt.Sprintf("%d:%02d remaining", minutes, seconds)
		b.WriteString(Current.Unselected.Render(countdown))
		b.WriteString("\n\n")

		// Render bubbles progress component (percent maintained in update)
		b.WriteString(Current.ProgressBarContainer.Render(m.progress.View()))
		b.WriteString("\n")
	}

	footer := m.Help.View(m.Keys.ForState(stateRunning))
	b.WriteString("\n" + footer)

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + Current.Error.Render(m.ErrorMessage))
	}

	return b.String()
}

// Help overlay with version and CLI usage
func helpView(m Model) string {
	help := `Keep-Alive — Help
Version: %s

Usage:
  keepalive [flags]

Flags:
  -d, --duration string   Duration to keep system alive (e.g., "2h30m" or "150")
  -c, --clock string     Time to keep system alive until (e.g., "22:00" or "10:00PM")
  -v, --version          Show version information
  -h, --help            Show help message

Examples:
  keepalive                    # Start with interactive TUI
  keepalive -d 2h30m          # Keep system awake for 2 hours and 30 minutes
  keepalive -d 150            # Keep system awake for 150 minutes
  keepalive -c 22:00          # Keep system awake until 10:00 PM
  keepalive -c 10:00PM        # Keep system awake until 10:00 PM
  keepalive --version         # Show version information

Navigation:
  ↑/k, ↓/j  : Navigate menu
  Enter      : Select option
  h/?        : Toggle help overlay
  q/Esc      : Quit/Back
`
	return Current.Help.Render(fmt.Sprintf(help, m.Version()))
}

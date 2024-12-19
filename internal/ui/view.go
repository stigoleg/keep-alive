package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			PaddingLeft(2).
			PaddingRight(2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingLeft(2)

	unselectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(2)

	awakeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#43BF6D")).
			PaddingLeft(2)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			PaddingLeft(2)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			PaddingLeft(2).
			PaddingRight(2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4"))

	inputBoxStyle = lipgloss.NewStyle().
			Width(10).
			PaddingLeft(2).
			PaddingRight(2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4"))
)

// View renders the current state of the model to a string.
func View(m Model) string {
	if m.ShowHelp {
		return helpView()
	}

	var s strings.Builder

	// Title
	s.WriteString(titleStyle.Render("Keep-Alive"))
	s.WriteString("\n\n")

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

	b.WriteString(titleStyle.Render("Keep Alive Options"))
	b.WriteString("\n\n")

	b.WriteString(unselectedStyle.Render("Select an option:"))
	b.WriteString("\n\n")

	menuItems := []string{
		"Keep system awake indefinitely",
		"Keep system awake for X minutes",
		"Quit keep-alive",
	}

	for i, opt := range menuItems {
		var menuLine strings.Builder

		if i == m.Selected {
			menuLine.WriteString(selectedStyle.Render("> "))
		} else {
			menuLine.WriteString(unselectedStyle.Render("  "))
		}

		if i == m.Selected {
			menuLine.WriteString(selectedStyle.Render(opt))
		} else {
			menuLine.WriteString(unselectedStyle.Render(opt))
		}

		b.WriteString(menuLine.String() + "\n")
	}

	if m.ErrorMessage != "" {
		b.WriteString("\n" + errorStyle.Render(m.ErrorMessage))
	}

	b.WriteString("\n\n" + helpStyle.Render("Press j/k or ↑/↓ to select • enter to confirm • q or esc to quit"))
	return b.String()
}

func timedInputView(m Model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Enter Duration"))
	b.WriteString("\n\n")

	b.WriteString(unselectedStyle.Render("Enter duration in minutes:"))
	b.WriteString("\n")
	input := m.Input
	if input == "" {
		input = " "
	}
	b.WriteString(inputBoxStyle.Render(input))
	// b.WriteString(Current.InputBox.Render(input))
	b.WriteString("\n\n")

	// Help text
	// b.WriteString(helpStyle.Render("Enter number of minutes"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press enter to start • backspace to clear • esc to cancel"))

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + errorStyle.Render(m.ErrorMessage))
	}

	return b.String()
}

func runningView(m Model) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Keep Alive Active"))
	b.WriteString("\n\n")

	b.WriteString(awakeStyle.Render("System is being kept awake"))
	b.WriteString("\n")

	// Show countdown if this is a timed session
	if m.Duration > time.Duration(0) {
		remaining := m.TimeRemaining()
		minutes := int(remaining.Minutes())
		seconds := int(remaining.Seconds()) % 60
		countdown := fmt.Sprintf("%d:%02d remaining", minutes, seconds)
		b.WriteString(unselectedStyle.Render(countdown))
		b.WriteString("\n")
	}

	b.WriteString("\n" + helpStyle.Render("Press enter to stop and return to menu • q or esc to quit"))

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + errorStyle.Render(m.ErrorMessage))
	}

	return b.String()
}

func helpView() string {
	help := `Keep-Alive Help

Usage:
  keepalive [flags]

Flags:
  -d, --duration string   Duration to keep system alive (e.g., "2h30m")
  -v, --version          Show version information
  -h, --help            Show help message

Examples:
  keepalive                    # Start with interactive TUI
  keepalive -d 2h30m          # Keep system awake for 2 hours and 30 minutes
  keepalive -d 150            # Keep system awake for 150 minutes
  keepalive --version         # Show version information

Navigation:
  ↑/k, ↓/j  : Navigate menu
  Enter      : Select option
  h          : Show this help
  q/Esc      : Quit/Back

Press 'q' or 'Esc' to close help`

	return helpStyle.Render(help)
}

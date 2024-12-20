package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current state of the model to a string.
func View(m Model) string {
	if m.ShowHelp {
		return helpView()
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

	b.WriteString("\n\n" + Current.Help.Render("j/k or ↑/↓ to select • enter to confirm • q to quit"))
	return b.String()
}

func timedInputView(m Model) string {
	var b strings.Builder

	b.WriteString(Current.Title.Render("Enter Duration"))
	b.WriteString("\n\n")

	b.WriteString(Current.Unselected.Render("Enter duration in minutes:"))
	b.WriteString("\n")
	input := m.Input
	if input == "" {
		input = " "
	}
	b.WriteString(Current.InputBox.Render(input))
	b.WriteString("\n\n")

	b.WriteString("\n" + Current.Help.Render("Press enter to start • backspace to clear • esc to cancel"))

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + Current.Error.Render(m.ErrorMessage))
	}

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

		// Calculate progress
		progress := 1.0 - (float64(remaining) / float64(m.Duration))
		width := 20 // Width of the progress bar - adjusted to match help text width

		// Create progress bar components
		filled := int(progress * float64(width))
		if filled > width {
			filled = width
		}

		// Define gradient colors (from purple to green with more steps)
		gradientColors := []string{
			"#7D56F4", "#7857F4", "#7359F5", "#6E5AF5", "#695CF6",
			"#645DF6", "#5F5FF7", "#5A60F7", "#5562F8", "#5063F8",
			"#4B65F9", "#4666F9", "#4168FA", "#3C69FA", "#376BFB",
			"#326CFB", "#2D6EFC", "#286FFC", "#2371FD", "#1E72FD",
			"#1974FE", "#1475FE", "#0F77FF", "#0A78FF", "#057AFF",
			"#007BFF", "#007DFA", "#007FF5", "#0081F0", "#0083EB",
			"#0085E6", "#0087E1", "#0089DC", "#008BD7", "#008DD2",
			"#008FCD", "#0091C8", "#0093C3", "#0095BE", "#0097B9",
			"#0099B4", "#009BAF", "#009DAA", "#009FA5", "#00A1A0",
			"#00A39B", "#00A596", "#00A791", "#00A98C", "#00AB87",
			"#00AD82", "#00AF7D", "#00B178", "#00B373", "#00B56E",
			"#00B769", "#00B964", "#00BB5F", "#00BD5A", "#00BF55",
			"#43BF6D",
		}

		// Build the progress bar with smooth gradient
		var bar strings.Builder
		for i := 0; i < width; i++ {
			if i < filled {
				// Calculate color index for smooth gradient
				colorIndex := int(float64(i) / float64(width) * float64(len(gradientColors)-1))
				if colorIndex >= len(gradientColors) {
					colorIndex = len(gradientColors) - 1
				}
				blockStyle := Current.ProgressBar.Copy().
					Background(lipgloss.Color(gradientColors[colorIndex]))
				bar.WriteString(blockStyle.Render(" "))
			} else {
				bar.WriteString(Current.ProgressBar.Copy().Render(" "))
			}
		}

		b.WriteString(Current.ProgressBarContainer.Render(bar.String()))
		b.WriteString("\n")
	}

	b.WriteString("\n" + Current.Help.Render("Press enter to stop and return to menu • q or esc to quit"))

	if m.ErrorMessage != "" {
		b.WriteString("\n\n" + Current.Error.Render(m.ErrorMessage))
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

	return Current.Help.Render(help)
}

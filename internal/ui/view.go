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

		// Define gradient colors (from purple to green with very fine steps)
		gradientColors := []string{
			"#7D56F4", "#7B57F4", "#7958F4", "#7759F4", "#755AF4", "#735BF4", "#715CF5", "#6F5DF5",
			"#6D5EF5", "#6B5FF5", "#695FF5", "#6760F5", "#6561F6", "#6362F6", "#6163F6", "#5F64F6",
			"#5D65F6", "#5B66F6", "#5967F7", "#5768F7", "#5569F7", "#536AF7", "#516BF7", "#4F6CF7",
			"#4D6DF8", "#4B6EF8", "#496FF8", "#4770F8", "#4571F8", "#4372F8", "#4173F9", "#3F74F9",
			"#3D75F9", "#3B76F9", "#3977F9", "#3778F9", "#3579FA", "#337AFA", "#317BFA", "#2F7CFA",
			"#2D7DFA", "#2B7EFA", "#297FFB", "#2780FB", "#2581FB", "#2382FB", "#2183FB", "#1F84FB",
			"#1D85FC", "#1B86FC", "#1987FC", "#1788FC", "#1589FC", "#138AFC", "#118BFD", "#0F8CFD",
			"#0D8DFD", "#0B8EFD", "#098FFD", "#0790FD", "#0591FE", "#0392FE", "#0193FE", "#0094FE",
			"#0095FA", "#0096F6", "#0097F2", "#0098EE", "#0099EA", "#009AE6", "#009BE2", "#009CDE",
			"#009DDA", "#009ED6", "#009FD2", "#00A0CE", "#00A1CA", "#00A2C6", "#00A3C2", "#00A4BE",
			"#00A5BA", "#00A6B6", "#00A7B2", "#00A8AE", "#00A9AA", "#00AAA6", "#00ABA2", "#00AC9E",
			"#00AD9A", "#00AE96", "#00AF92", "#00B08E", "#00B18A", "#00B286", "#00B382", "#00B47E",
			"#00B57A", "#00B676", "#00B772", "#00B86E", "#00B96A", "#00BA66", "#00BB62", "#00BC5E",
			"#00BD5A", "#00BE56", "#43BF6D",
		}

		// Build the progress bar with smooth gradient
		var bar strings.Builder
		for i := 0; i < width; i++ {
			// Calculate exact position progress
			pos := float64(i) / float64(width)
			if pos <= progress {
				// Calculate color index for smooth gradient
				colorIndex := int(pos * float64(len(gradientColors)-1))
				if colorIndex >= len(gradientColors) {
					colorIndex = len(gradientColors) - 1
				}
				blockStyle := Current.ProgressBar.
					Background(lipgloss.Color(gradientColors[colorIndex]))
				bar.WriteString(blockStyle.Render(" "))
			} else {
				bar.WriteString(Current.ProgressBar.Render(" "))
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

func helpView(m Model) string {
	help := `Keep-Alive Help
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
  h          : Show this help
  q/Esc      : Quit/Back

Press 'q' or 'Esc' to close help`

	return Current.Help.Render(fmt.Sprintf(help, m.Version()))
}

package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
)

const (
	minHelpPopupWidth  = 24
	minHelpPopupHeight = 8
	maxHelpPopupWidth  = 104
	maxHelpPopupHeight = 34
	helpPopupMargin    = 1
)

// View renders the current state of the model to a string.
func View(m Model) string {
	if m.ShowDependencyInfo {
		return dependencyInfoView(m)
	}
	if m.ShowHelp {
		return renderWithHelpOverlay(m)
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

func baseView(m Model) string {
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

func ErrorBanner(message string) string {
	return "\n" + Current.Error.Render(strings.TrimSpace(message)) + "\n"
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

	// Activity simulation toggle
	b.WriteString("\n")
	activeStatus := "[ ]"
	if m.SimulateActivity {
		activeStatus = "[x]"
	}
	activeText := fmt.Sprintf("%s Simulate activity (Slack/Teams)", activeStatus)
	b.WriteString(Current.Unselected.Render(activeText) + " " + Current.Unselected.Render("(press 'a' to toggle)"))
	b.WriteString("\n")

	// Dependency warning notification
	if hasInfoWarning(m) && !m.ShowDependencyInfo {
		b.WriteString("\n")
		warningText := "Missing activity/dependency information. Press 'i' for details."
		b.WriteString(Current.Error.Render(warningText))
		b.WriteString("\n")
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
	if m.SimulateActivity {
		if m.ActivityWarning != "" {
			b.WriteString(Current.Error.Render("Activity simulation unavailable"))
		} else {
			b.WriteString(Current.Unselected.Render("Activity simulation enabled"))
		}
		b.WriteString("\n")
	}

	if m.BatteryThreshold > 0 {
		b.WriteString(Current.Unselected.Render(fmt.Sprintf("Battery: %d%%", m.BatteryPercentage)))
		b.WriteString("\n")
		b.WriteString(Current.Unselected.Render(fmt.Sprintf("Stopping at or below: %d%%", m.BatteryThreshold)))
		b.WriteString("\n")
		if m.BatteryError != "" {
			b.WriteString(Current.Error.Render("Battery status unavailable: " + m.BatteryError))
			b.WriteString("\n")
		}
	}

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
	if m.Height <= 0 {
		return fullHelpView(m)
	}
	m = syncHelpViewport(m)
	return helpPopupView(m)
}

func fullHelpView(m Model) string {
	outerWidth, _ := helpPopupSize(m.Width, maxHelpPopupHeight)
	style := helpPopupStyle(outerWidth)
	bodyWidth := outerWidth - style.GetHorizontalFrameSize()
	if bodyWidth < 1 {
		bodyWidth = 1
	}

	header := lipgloss.NewStyle().
		Width(bodyWidth).
		Bold(true).
		Foreground(defaultColors.Highlight).
		Render(fmt.Sprintf("Keep-Alive Help  v%s", m.Version()))

	return style.Render(header + "\n" + helpContent(m))
}

func helpContent(m Model) string {
	width := helpBodyWidth(m)

	var b strings.Builder
	b.WriteString("Usage:\n")
	b.WriteString(wrapHelpLine("keepalive [flags]", width))
	b.WriteString("\n\n")

	if width < 52 {
		b.WriteString("Flags:\n")
		b.WriteString(renderDefinitionList(flagHelpRows(), width))
		b.WriteString("\n\nExamples:\n")
		b.WriteString(renderDefinitionList(exampleHelpRows(), width))
	} else {
		tableWidth := maxInt(20, width-2)
		b.WriteString("Flags:\n")
		b.WriteString(renderHelpTable(tableWidth, "FLAG", "DESCRIPTION", flagHelpRows()))
		b.WriteString("\n\nExamples:\n")
		b.WriteString(renderHelpTable(tableWidth, "COMMAND", "DESCRIPTION", exampleHelpRows()))
	}

	b.WriteString("\n\nNavigation:\n")
	b.WriteString(renderKeyValueRows(navigationHelpRows(), width))
	return b.String()
}

func renderWithHelpOverlay(m Model) string {
	base := baseView(m)
	if m.Width <= 0 || m.Height <= 0 {
		return helpView(m)
	}
	return overlayBlock(base, helpView(m), m.Width, m.Height)
}

func newHelpViewport(width int, height int) viewport.Model {
	vp := viewport.New(width, height)
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	return vp
}

func syncHelpViewport(m Model) Model {
	outerWidth, outerHeight := helpPopupSize(m.Width, m.Height)
	style := helpPopupStyle(outerWidth)
	bodyWidth := outerWidth - style.GetHorizontalFrameSize()
	bodyHeight := helpViewportHeight(outerHeight, style)

	if bodyWidth < 1 {
		bodyWidth = 1
	}
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	if m.HelpViewport.Width == 0 || m.HelpViewport.Height == 0 {
		m.HelpViewport = newHelpViewport(bodyWidth, bodyHeight)
	}
	m.HelpViewport.Width = bodyWidth
	m.HelpViewport.Height = bodyHeight
	m.HelpViewport.SetContent(helpContent(m))
	return m
}

func helpPopupView(m Model) string {
	outerWidth, _ := helpPopupSize(m.Width, m.Height)
	style := helpPopupStyle(outerWidth)
	bodyWidth := outerWidth - style.GetHorizontalFrameSize()
	if bodyWidth < 1 {
		bodyWidth = 1
	}

	header := lipgloss.NewStyle().
		Width(bodyWidth).
		Bold(true).
		Foreground(defaultColors.Highlight).
		Render(fmt.Sprintf("Keep-Alive Help  v%s", m.Version()))

	footer := helpFooter(m, bodyWidth)
	content := strings.Join([]string{header, m.HelpViewport.View(), footer}, "\n")
	return style.Render(content)
}

func helpPopupStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(width-4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(defaultColors.Highlight).
		Padding(0, 1)
}

func helpFooter(m Model, width int) string {
	position := "all"
	if m.HelpViewport.TotalLineCount() > m.HelpViewport.VisibleLineCount() {
		position = fmt.Sprintf("%d%%", int(m.HelpViewport.ScrollPercent()*100))
	}
	text := fmt.Sprintf("up/down scroll  pgup/pgdn page  esc/q close  %s", position)
	if width < 58 {
		text = fmt.Sprintf("up/down scroll  esc/q close  %s", position)
	}
	if width < 36 {
		text = fmt.Sprintf("scroll up/down  close esc/q  %s", position)
	}
	return lipgloss.NewStyle().
		Width(width).
		Foreground(defaultColors.Subtle).
		Render(text)
}

func helpViewportHeight(outerHeight int, style lipgloss.Style) int {
	const headerAndFooter = 2
	return outerHeight - style.GetVerticalFrameSize() - headerAndFooter
}

func helpPopupSize(width int, height int) (int, int) {
	if width <= 0 {
		width = defaultTerminalWidth
	}
	if height <= 0 {
		height = maxHelpPopupHeight
	}

	outerWidth := minInt(maxHelpPopupWidth, width-helpPopupMargin)
	if outerWidth < minHelpPopupWidth {
		outerWidth = maxInt(10, width)
	}

	outerHeight := minInt(maxHelpPopupHeight, height-helpPopupMargin)
	if outerHeight < minHelpPopupHeight {
		outerHeight = maxInt(5, height)
	}

	return outerWidth, outerHeight
}

func helpBodyWidth(m Model) int {
	outerWidth, _ := helpPopupSize(m.Width, m.Height)
	style := helpPopupStyle(outerWidth)
	width := outerWidth - style.GetHorizontalFrameSize()
	if width < 1 {
		return 1
	}
	return width
}

func helpContentWidth(width int) int {
	if width <= 0 {
		width = defaultTerminalWidth
	}
	return maxInt(20, width-8)
}

func renderHelpTable(width int, leftHeader string, rightHeader string, rows [][]string) string {
	headerStyle := lipgloss.NewStyle().
		Foreground(defaultColors.Highlight).
		Bold(true).
		Align(lipgloss.Center)
	cellStyle := lipgloss.NewStyle().Padding(0, 1)
	oddRowStyle := cellStyle.Foreground(defaultColors.Subtle)
	evenRowStyle := cellStyle.Foreground(lipgloss.Color("#FAFAFA"))

	return table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(defaultColors.Highlight)).
		Width(width).
		Wrap(true).
		Headers(leftHeader, rightHeader).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return headerStyle
			case row%2 == 0:
				return evenRowStyle
			default:
				return oddRowStyle
			}
		}).
		Render()
}

func renderDefinitionList(rows [][]string, width int) string {
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(wrapHelpLine(row[0], width))
		if len(row) > 1 && row[1] != "" {
			b.WriteString("\n")
			b.WriteString(wrapHelpLine("  "+row[1], width))
		}
	}
	return b.String()
}

func renderKeyValueRows(rows [][]string, width int) string {
	var b strings.Builder
	keyWidth := 13
	if width < 42 {
		keyWidth = 10
	}
	if width < 30 {
		return renderDefinitionList(rows, width)
	}

	for i, row := range rows {
		if i > 0 {
			b.WriteString("\n")
		}
		keyText := lipgloss.NewStyle().Width(keyWidth).Render(row[0])
		descWidth := maxInt(8, width-keyWidth-1)
		desc := ""
		if len(row) > 1 {
			desc = lipgloss.NewStyle().Width(descWidth).Render(row[1])
		}
		descLines := strings.Split(desc, "\n")
		b.WriteString(keyText + " " + descLines[0])
		for _, line := range descLines[1:] {
			b.WriteString("\n")
			b.WriteString(strings.Repeat(" ", keyWidth+1) + line)
		}
	}
	return b.String()
}

func wrapHelpLine(value string, width int) string {
	return lipgloss.NewStyle().Width(width).Render(value)
}

func flagHelpRows() [][]string {
	return [][]string{
		{"-d, --duration string", `Duration to keep system alive (e.g., "2h30m" or "150")`},
		{"-c, --clock string", `Time to keep system alive until (e.g., "22:00" or "10:00PM")`},
		{"-b, --battery int", "Keep system awake until battery reaches this percentage"},
		{"-a, --active", "Simulate activity when a real input backend is available"},
		{"-l, --log", "Enable logging to debug.log"},
		{"-v, --version", "Show version information"},
		{"-h, --help", "Show help message"},
	}
}

func exampleHelpRows() [][]string {
	return [][]string{
		{"keepalive", "Start with interactive TUI"},
		{"keepalive -d 2h30m", "Keep system awake for 2 hours and 30 minutes"},
		{"keepalive --active", "Keep system awake and simulate activity when supported"},
		{"keepalive -d 150", "Keep system awake for 150 minutes"},
		{"keepalive -c 22:00", "Keep system awake until 10:00 PM"},
		{"keepalive -b 20", "Keep system awake until battery is 20% or lower"},
		{"keepalive -d 20 -b 65", "Exit when duration ends or battery reaches 65%"},
		{"keepalive --version", "Show version information"},
	}
}

func navigationHelpRows() [][]string {
	return [][]string{
		{"up/k, down/j", "Navigate menu"},
		{"Enter", "Select option"},
		{"h/?", "Toggle help overlay"},
		{"i", "Show dependency information if available"},
		{"q/Esc", "Quit or go back"},
	}
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func overlayBlock(base string, overlay string, width int, height int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	if width <= 0 || height <= 0 {
		return overlay
	}

	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}

	overlayWidth := blockWidth(overlayLines)
	overlayHeight := len(overlayLines)
	x := maxInt(0, (width-overlayWidth)/2)
	y := maxInt(0, (height-overlayHeight)/2)

	for i, overlayLine := range overlayLines {
		target := y + i
		if target < 0 || target >= len(baseLines) {
			continue
		}
		line := padLine(baseLines[target], width)
		end := minInt(width, x+lipgloss.Width(overlayLine))
		prefix := ansi.Cut(line, 0, x)
		suffix := ansi.Cut(line, end, width)
		baseLines[target] = padLine(prefix, x) + overlayLine + suffix
	}

	return strings.Join(baseLines, "\n")
}

func blockWidth(lines []string) int {
	width := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > width {
			width = w
		}
	}
	return width
}

func padLine(line string, width int) string {
	if lipgloss.Width(line) >= width {
		return ansi.Cut(line, 0, width)
	}
	return line + strings.Repeat(" ", width-lipgloss.Width(line))
}

// dependencyInfoView displays detailed dependency information
func dependencyInfoView(m Model) string {
	message := infoMessage(m)
	if message == "" {
		return Current.Help.Render("No dependency information available.")
	}

	header := `Keep-Alive — Dependency Information
Version: %s

%s

Press 'i' or 'Esc' to close this view.
`
	return Current.Help.Render(fmt.Sprintf(header, m.Version(), message))
}

func hasInfoWarning(m Model) bool {
	return m.DependencyWarning != "" || m.ActivityWarning != ""
}

func infoMessage(m Model) string {
	var parts []string
	if m.ActivityWarning != "" {
		parts = append(parts, m.ActivityWarning)
	}
	if m.DependencyWarning != "" {
		parts = append(parts, m.DependencyWarning)
	}
	return strings.Join(parts, "\n\n")
}

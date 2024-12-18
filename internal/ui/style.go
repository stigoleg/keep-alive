// Package ui provides the terminal user interface for the keep-alive application.
package ui

import "github.com/charmbracelet/lipgloss"

// Colors defines the color scheme used throughout the application
type Colors struct {
	Subtle    lipgloss.AdaptiveColor
	Highlight lipgloss.AdaptiveColor
	Special   lipgloss.AdaptiveColor
	Error     lipgloss.AdaptiveColor
}

// DefaultColors returns the default color scheme
var defaultColors = Colors{
	Subtle:    lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"},
	Highlight: lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"},
	Special:   lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"},
	Error:     lipgloss.AdaptiveColor{Light: "#FF0000", Dark: "#FF4040"},
}

// Style represents a collection of styles used in the application
type Style struct {
	Title          lipgloss.Style
	ActiveStatus   lipgloss.Style
	InactiveStatus lipgloss.Style
	DisabledItem   lipgloss.Style
	SelectedItem   lipgloss.Style
	Menu           lipgloss.Style
	InputBox       lipgloss.Style
	Help           lipgloss.Style
	Error          lipgloss.Style
	Countdown      lipgloss.Style
}

// DefaultStyle returns the default style configuration
func DefaultStyle() Style {
	base := lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1)

	return Style{
		Title: base.Copy().
			Bold(true).
			Foreground(defaultColors.Highlight),

		ActiveStatus: base.Copy().
			Foreground(defaultColors.Special),

		InactiveStatus: base.Copy().
			Foreground(defaultColors.Subtle),

		DisabledItem: base.Copy().
			Foreground(defaultColors.Subtle),

		SelectedItem: base.Copy().
			Bold(true).
			Foreground(defaultColors.Highlight),

		Menu: base.Copy(),

		InputBox: base.Copy().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(defaultColors.Highlight).
			Padding(0, 1),

		Help: base.Copy().
			Foreground(defaultColors.Subtle),

		Error: base.Copy().
			Foreground(defaultColors.Error),

		Countdown: base.Copy().
			Foreground(defaultColors.Highlight).
			Bold(true),
	}
}

// Current holds the current style configuration
var Current = DefaultStyle()

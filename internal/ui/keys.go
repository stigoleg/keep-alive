package ui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap defines key bindings for various UI states and common actions.
type KeyMap struct {
	// Common
	Quit       key.Binding
	ToggleHelp key.Binding

	// Menu navigation
	Up     key.Binding
	Down   key.Binding
	Select key.Binding

	// Timed input
	Back      key.Binding
	Submit    key.Binding
	Backspace key.Binding

	// Running
	Stop key.Binding
}

// DefaultKeys returns the default key bindings for the application.
func DefaultKeys() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		ToggleHelp: key.NewBinding(
			key.WithKeys("h", "?"),
			key.WithHelp("h/?", "toggle help"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter", " "),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "start"),
		),
		Backspace: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "delete"),
		),
		Stop: key.NewBinding(
			key.WithKeys("s", "esc"),
			key.WithHelp("s/esc", "stop"),
		),
	}
}

// NewHelpModel returns a configured help model.
func NewHelpModel() help.Model {
	h := help.New()
	return h
}

// stateKeyMap adapts bindings to the current UI state for contextual help.
type stateKeyMap struct {
	keys  KeyMap
	state state
}

// ForState returns a contextual key map implementing help.KeyMap for the given state.
func (k KeyMap) ForState(s state) help.KeyMap {
	return stateKeyMap{keys: k, state: s}
}

// ShortHelp implements help.KeyMap for contextual help (compact).
func (s stateKeyMap) ShortHelp() []key.Binding {
	switch s.state {
	case stateMenu:
		return []key.Binding{s.keys.Up, s.keys.Down, s.keys.Select, s.keys.ToggleHelp, s.keys.Quit}
	case stateTimedInput:
		return []key.Binding{s.keys.Submit, s.keys.Backspace, s.keys.Back, s.keys.Quit}
	case stateRunning:
		return []key.Binding{s.keys.Stop, s.keys.Quit, s.keys.ToggleHelp}
	default:
		return []key.Binding{s.keys.ToggleHelp, s.keys.Quit}
	}
}

// FullHelp implements help.KeyMap for contextual help (expanded).
func (s stateKeyMap) FullHelp() [][]key.Binding {
	switch s.state {
	case stateMenu:
		return [][]key.Binding{{s.keys.Up, s.keys.Down, s.keys.Select}, {s.keys.ToggleHelp, s.keys.Quit}}
	case stateTimedInput:
		return [][]key.Binding{{s.keys.Submit, s.keys.Backspace, s.keys.Back}, {s.keys.Quit}}
	case stateRunning:
		return [][]key.Binding{{s.keys.Stop, s.keys.Quit}, {s.keys.ToggleHelp}}
	default:
		return [][]key.Binding{{s.keys.ToggleHelp, s.keys.Quit}}
	}
}

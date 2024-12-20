package ui

type State int

const (
	StateMenu State = iota
	StateTimedInput
	StateRunning
	StateHelp
)

func (s State) String() string {
	switch s {
	case StateMenu:
		return "Menu"
	case StateTimedInput:
		return "TimedInput"
	case StateRunning:
		return "Running"
	case StateHelp:
		return "Help"
	default:
		return "Unknown"
	}
}

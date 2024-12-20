package ui

import (
	"github.com/stigoleg/keep-alive/internal/keepalive"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInitialModel(t *testing.T) {
	m := InitialModel()
	if m.State != stateMenu {
		t.Error("expected initial state to be stateMenu")
	}
	if m.Selected != 0 {
		t.Error("expected initial selected to be 0")
	}
	if m.Input != "" {
		t.Error("expected initial input to be empty")
	}
	if m.ErrorMessage != "" {
		t.Error("expected initial error message to be empty")
	}
}

func TestMenuView(t *testing.T) {
	m := InitialModel()
	view := View(m)

	// Check for menu options
	expectedOptions := []string{
		"Keep system awake indefinitely",
		"Keep system awake for X minutes",
		"Quit keep-alive",
	}

	for _, opt := range expectedOptions {
		if !strings.Contains(view, opt) {
			t.Errorf("expected view to contain option %q", opt)
		}
	}

	// Check cursor position
	lines := strings.Split(view, "\n")
	foundCursor := false
	for _, line := range lines {
		if strings.Contains(line, ">") && strings.Contains(line, "Keep system awake indefinitely") {
			foundCursor = true
			break
		}
	}
	if !foundCursor {
		t.Error("expected cursor to be at first option")
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name     string
		msg      tea.Msg
		model    Model
		wantType state
	}{
		{
			name:     "up key at top stays at top",
			msg:      tea.KeyMsg{Type: tea.KeyUp},
			model:    Model{State: stateMenu, Selected: 0},
			wantType: stateMenu,
		},
		{
			name:     "down key moves selection",
			msg:      tea.KeyMsg{Type: tea.KeyDown},
			model:    Model{State: stateMenu, Selected: 0},
			wantType: stateMenu,
		},
		{
			name:     "enter on timed input moves to input state",
			msg:      tea.KeyMsg{Type: tea.KeyEnter},
			model:    Model{State: stateMenu, Selected: 1},
			wantType: stateTimedInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.model.KeepAlive = &keepalive.Keeper{}
			got, _ := Update(tt.msg, tt.model)
			if got.State != tt.wantType {
				t.Errorf("Update() state = %v, want %v", got.State, tt.wantType)
			}
		})
	}
}

func TestTimedInputView(t *testing.T) {
	m := Model{
		State:     stateTimedInput,
		Input:     "5",
		KeepAlive: &keepalive.Keeper{},
	}
	view := View(m)

	if !strings.Contains(view, "minutes") {
		t.Error("expected view to contain duration prompt")
	}
	if !strings.Contains(view, "5") {
		t.Error("expected view to show input value")
	}
}

func TestRunningView(t *testing.T) {
	m := Model{
		State:     stateRunning,
		StartTime: time.Now(),
		Duration:  5 * time.Minute,
		KeepAlive: &keepalive.Keeper{},
	}
	view := View(m)

	if !strings.Contains(view, "Keep Alive Active") {
		t.Error("expected view to show active status")
	}
	if !strings.Contains(view, "System is being kept awake") {
		t.Error("expected view to show system status")
	}
	if !strings.Contains(view, "remaining") {
		t.Error("expected view to show remaining time")
	}
}

func TestErrorDisplay(t *testing.T) {
	m := Model{
		State:        stateMenu,
		ErrorMessage: "test error",
		KeepAlive:    &keepalive.Keeper{},
	}
	view := View(m)

	if !strings.Contains(view, "test error") {
		t.Error("expected view to show error message")
	}
}

func TestTimeRemaining(t *testing.T) {
	now := time.Now()
	keeper := &keepalive.Keeper{}
	_ = keeper.StartIndefinite() // Start the keeper for the test

	tests := []struct {
		name      string
		model     Model
		wantZero  bool
		wantRange time.Duration
	}{
		{
			name: "no duration",
			model: Model{
				StartTime:  now,
				Duration:  0,
				KeepAlive: keeper,
			},
			wantZero: true,
		},
		{
			name: "with duration",
			model: Model{
				StartTime:  now,
				Duration:  5 * time.Minute,
				KeepAlive: keeper,
				State:     stateRunning,
			},
			wantZero:  false,
			wantRange: 5 * time.Minute,
		},
		{
			name: "expired duration",
			model: Model{
				StartTime:  now.Add(-6 * time.Minute),
				Duration:  5 * time.Minute,
				KeepAlive: keeper,
			},
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.model.TimeRemaining()
			if tt.wantZero && got != 0 {
				t.Errorf("TimeRemaining() = %v, want 0", got)
			}
			if !tt.wantZero && (got < 0 || got > tt.wantRange) {
				t.Errorf("TimeRemaining() = %v, want between 0 and %v", got, tt.wantRange)
			}
		})
	}

	// Clean up
	_ = keeper.Stop()
}

package ui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/internal/keepalive"

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
		KeepAlive: &keepalive.Keeper{},
	}
	m.textInput = newMinutesTextInput()
	m.textInput.SetValue("5")
	view := View(m)

	if !strings.Contains(view, "minutes") {
		t.Error("expected view to contain duration prompt")
	}
	if !strings.Contains(view, "5") {
		t.Error("expected view to show input value")
	}
}

func TestTimedInputValidationErrors(t *testing.T) {
	// Empty input
	m := Model{State: stateTimedInput, KeepAlive: &keepalive.Keeper{}}
	m.textInput = newMinutesTextInput()
	m.textInput.SetValue("")
	got, _ := Update(tea.KeyMsg{Type: tea.KeyEnter}, m)
	if got.ErrorMessage == "" {
		t.Error("expected error for empty input")
	}

	// Zero minutes
	m2 := Model{State: stateTimedInput, KeepAlive: &keepalive.Keeper{}}
	m2.textInput = newMinutesTextInput()
	m2.textInput.SetValue("0")
	got2, _ := Update(tea.KeyMsg{Type: tea.KeyEnter}, m2)
	if !strings.Contains(got2.ErrorMessage, "Invalid Input") {
		t.Error("expected invalid input error for zero")
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
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Add test timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Kill any existing caffeinate processes
	if runtime.GOOS == "darwin" {
		exec.Command("pkill", "-9", "caffeinate").Run()
	}

	// Cleanup after test
	t.Cleanup(func() {
		if runtime.GOOS == "darwin" {
			exec.Command("pkill", "-9", "caffeinate").Run()
		}
	})

	// Create a keeper with a short duration
	k := &keepalive.Keeper{}
	defer k.Stop() // Ensure cleanup even if test fails

	// Start timed with a short duration
	duration := 2 * time.Second
	err := k.StartTimed(duration)
	if err != nil && err.Error() == "unsupported platform" {
		t.Skip("Skipping on unsupported platform")
	}
	if err != nil {
		t.Fatalf("StartTimed failed: %v", err)
	}

	// Wait for context or short timeout
	select {
	case <-ctx.Done():
		t.Fatal("test timeout")
	case <-time.After(200 * time.Millisecond):
	}

	// Check time remaining
	remaining := k.TimeRemaining()
	if remaining > duration {
		t.Errorf("TimeRemaining returned %v, expected <= %v", remaining, duration)
	}
	if remaining <= 0 {
		t.Error("TimeRemaining returned <= 0 immediately after start")
	}

	// Stop and verify cleanup
	err = k.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if k.TimeRemaining() != 0 {
		t.Error("TimeRemaining not 0 after stop")
	}
}

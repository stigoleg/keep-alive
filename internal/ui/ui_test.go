package ui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stigoleg/keep-alive/internal/keepalive"
	"github.com/stigoleg/keep-alive/internal/platform"

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
			tt.model.KeepAlive = keepalive.NewKeeper()
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
		KeepAlive: keepalive.NewKeeper(),
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
	m := Model{State: stateTimedInput, KeepAlive: keepalive.NewKeeper()}
	m.textInput = newMinutesTextInput()
	m.textInput.SetValue("")
	got, _ := Update(tea.KeyMsg{Type: tea.KeyEnter}, m)
	if got.ErrorMessage == "" {
		t.Error("expected error for empty input")
	}

	// Zero minutes
	m2 := Model{State: stateTimedInput, KeepAlive: keepalive.NewKeeper()}
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
		KeepAlive: keepalive.NewKeeper(),
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

func TestRunningViewBatteryMode(t *testing.T) {
	m := Model{
		State:             stateRunning,
		KeepAlive:         keepalive.NewKeeper(),
		BatteryThreshold:  20,
		BatteryPercentage: 42,
	}
	view := View(m)

	if !strings.Contains(view, "Battery: 42%") {
		t.Error("expected view to show current battery percentage")
	}
	if !strings.Contains(view, "Stopping at or below: 20%") {
		t.Error("expected view to show battery threshold")
	}
}

func TestRunningViewCombinedLimits(t *testing.T) {
	m := Model{
		State:             stateRunning,
		StartTime:         time.Now(),
		Duration:          5 * time.Minute,
		KeepAlive:         keepalive.NewKeeper(),
		BatteryThreshold:  20,
		BatteryPercentage: 42,
	}
	view := View(m)

	if !strings.Contains(view, "remaining") {
		t.Error("expected view to show remaining time")
	}
	if !strings.Contains(view, "Battery: 42%") {
		t.Error("expected view to show battery percentage")
	}
}

func TestBatteryStatusAtThresholdQuits(t *testing.T) {
	m := Model{
		State:            stateRunning,
		KeepAlive:        keepalive.NewKeeper(),
		BatteryThreshold: 20,
	}

	got, cmd := Update(batteryStatusMsg{status: platformBatteryStatus(20)}, m)
	if got.State != stateMenu {
		t.Fatalf("Update() state = %v, want %v", got.State, stateMenu)
	}
	if cmd == nil {
		t.Fatal("Update() command is nil, want quit command")
	}
}

func TestBatteryStatusAboveThresholdKeepsRunning(t *testing.T) {
	m := Model{
		State:            stateRunning,
		KeepAlive:        keepalive.NewKeeper(),
		BatteryThreshold: 20,
	}

	got, cmd := Update(batteryStatusMsg{status: platformBatteryStatus(21)}, m)
	if got.State != stateRunning {
		t.Fatalf("Update() state = %v, want %v", got.State, stateRunning)
	}
	if got.BatteryPercentage != 21 {
		t.Fatalf("Update() BatteryPercentage = %d, want 21", got.BatteryPercentage)
	}
	if cmd == nil {
		t.Fatal("Update() command is nil, want next battery poll command")
	}
}

func TestWindowSizeUpdatesModel(t *testing.T) {
	m := InitialModel()
	got, _ := Update(tea.WindowSizeMsg{Width: 44, Height: 12}, m)

	if got.Width != 44 {
		t.Fatalf("Update() Width = %d, want 44", got.Width)
	}
	if got.Height != 12 {
		t.Fatalf("Update() Height = %d, want 12", got.Height)
	}
}

func TestHelpViewFitsNarrowWidth(t *testing.T) {
	m := InitialModel()
	m.ShowHelp = true
	m.Width = 40
	m.Height = 14
	view := View(m)

	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > m.Width {
			t.Fatalf("help line width = %d, want <= %d: %q", got, m.Width, line)
		}
	}
}

func TestHelpPopupHasCompleteBorderAtSmallHeight(t *testing.T) {
	m := InitialModel()
	m.ShowHelp = true
	m.Width = 48
	m.Height = 10
	view := View(m)

	if !strings.Contains(view, "╭") {
		t.Fatalf("expected help popup top border, got:\n%s", view)
	}
	if !strings.Contains(view, "╰") {
		t.Fatalf("expected help popup bottom border, got:\n%s", view)
	}
}

func TestHelpTableBordersFitNormalWidth(t *testing.T) {
	m := InitialModel()
	m.Width = 80
	m.Height = 24
	content := helpContent(m)

	for _, line := range strings.Split(content, "\n") {
		if got := lipgloss.Width(line); got > helpBodyWidth(m) {
			t.Fatalf("help content line width = %d, want <= %d: %q", got, helpBodyWidth(m), line)
		}
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "─┐" || trimmed == "─┤" || trimmed == "─┘" {
			t.Fatalf("table border fragment appears on its own line:\n%s", content)
		}
	}
}

func TestNavigationRowsRenderOnOneLine(t *testing.T) {
	content := renderKeyValueRows(navigationHelpRows(), 64)

	if !strings.Contains(content, "up/k, down/j  Navigate menu") {
		t.Fatalf("expected navigation key and description on one line, got:\n%s", content)
	}
	if strings.Contains(content, "up/k, down/j\n") {
		t.Fatalf("navigation key rendered without description on same line:\n%s", content)
	}
}

func TestHelpViewportScrolls(t *testing.T) {
	m := InitialModel()
	m.ShowHelp = true
	m.Width = 56
	m.Height = 10
	m = syncHelpViewport(m)

	if m.HelpViewport.TotalLineCount() <= m.HelpViewport.VisibleLineCount() {
		t.Fatalf("expected help content to overflow viewport")
	}

	got, _ := Update(tea.KeyMsg{Type: tea.KeyDown}, m)
	if got.HelpViewport.YOffset <= m.HelpViewport.YOffset {
		t.Fatalf("expected help viewport to scroll down, before=%d after=%d", m.HelpViewport.YOffset, got.HelpViewport.YOffset)
	}
}

func TestHelpCloseDoesNotQuit(t *testing.T) {
	m := InitialModel()
	m.ShowHelp = true
	m = syncHelpViewport(m)

	got, cmd := Update(tea.KeyMsg{Type: tea.KeyEsc}, m)
	if got.ShowHelp {
		t.Fatalf("expected help to close")
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when closing help")
	}
}

func TestErrorBannerHasOwnLines(t *testing.T) {
	banner := ErrorBanner("invalid flag")
	if !strings.HasPrefix(banner, "\n") {
		t.Fatalf("ErrorBanner() = %q, want leading newline", banner)
	}
	if !strings.HasSuffix(banner, "\n") {
		t.Fatalf("ErrorBanner() = %q, want trailing newline", banner)
	}
	if !strings.Contains(banner, "invalid flag") {
		t.Fatalf("ErrorBanner() = %q, want message", banner)
	}
}

func TestErrorDisplay(t *testing.T) {
	m := Model{
		State:        stateMenu,
		ErrorMessage: "test error",
		KeepAlive:    keepalive.NewKeeper(),
	}
	view := View(m)

	if !strings.Contains(view, "test error") {
		t.Error("expected view to show error message")
	}
}

func platformBatteryStatus(percentage int) platform.BatteryStatus {
	return platform.BatteryStatus{Percentage: percentage, Available: true}
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
	k := keepalive.NewKeeper()
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

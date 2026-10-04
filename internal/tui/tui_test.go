package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/session"

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
		"Keep system awake until clock time",
		"Quit keep-alive",
		"Battery threshold",
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
		{
			name:     "enter on clock option moves to clock input state",
			msg:      tea.KeyMsg{Type: tea.KeyEnter},
			model:    Model{State: stateMenu, Selected: 2},
			wantType: stateClockInput,
		},
		{
			name:     "b key moves to battery input state",
			msg:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}},
			model:    Model{State: stateMenu, Selected: 0},
			wantType: stateBatteryInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := Update(tt.msg, tt.model)
			if got.State != tt.wantType {
				t.Errorf("Update() state = %v, want %v", got.State, tt.wantType)
			}
		})
	}
}

func TestTimedInputView(t *testing.T) {
	m := Model{
		State: stateTimedInput,
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

func TestClockInputView(t *testing.T) {
	m := Model{
		State: stateClockInput,
	}
	m.textInput = newClockTextInput()
	m.textInput.SetValue("22:00")
	view := View(m)

	if !strings.Contains(view, "clock time") {
		t.Error("expected view to contain clock prompt")
	}
	if !strings.Contains(view, "22:00") {
		t.Error("expected view to show input value")
	}
}

func TestBatteryInputSetsThreshold(t *testing.T) {
	restore := stubBatteryStatus(platformBatteryStatus(80), nil)
	defer restore()

	m := Model{State: stateBatteryInput}
	m.textInput = newBatteryTextInput(0)
	m.textInput.SetValue("65")

	got, _ := Update(tea.KeyMsg{Type: tea.KeyEnter}, m)
	if got.State != stateMenu {
		t.Fatalf("Update() state = %v, want %v", got.State, stateMenu)
	}
	if got.BatteryThreshold != 65 {
		t.Fatalf("Update() BatteryThreshold = %d, want 65", got.BatteryThreshold)
	}
	if got.BatteryPercentage != 80 {
		t.Fatalf("Update() BatteryPercentage = %d, want 80", got.BatteryPercentage)
	}
}

func TestBatteryInputRejectsThresholdAtCurrentBattery(t *testing.T) {
	restore := stubBatteryStatus(platformBatteryStatus(65), nil)
	defer restore()

	m := Model{State: stateBatteryInput}
	m.textInput = newBatteryTextInput(0)
	m.textInput.SetValue("65")

	got, _ := Update(tea.KeyMsg{Type: tea.KeyEnter}, m)
	if got.State != stateBatteryInput {
		t.Fatalf("Update() state = %v, want %v", got.State, stateBatteryInput)
	}
	if got.ErrorMessage == "" {
		t.Fatal("expected battery validation error")
	}
}

func TestTimedInputValidationErrors(t *testing.T) {
	// Empty input
	m := Model{State: stateTimedInput}
	m.textInput = newMinutesTextInput()
	m.textInput.SetValue("")
	got, _ := Update(tea.KeyMsg{Type: tea.KeyEnter}, m)
	if got.ErrorMessage == "" {
		t.Error("expected error for empty input")
	}

	// Zero minutes
	m2 := Model{State: stateTimedInput}
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

func TestBatteryStoppedEventQuits(t *testing.T) {
	m, _ := startTestSession(t, Options{Base: session.Config{BatteryThreshold: 20}, Start: true})
	r := m.sessions.current()
	stopped := session.Event{Type: session.EventStopped, Reason: session.ReasonBattery}

	got, cmd := Update(sessionEventMsg{r: r, ev: stopped}, m)
	if got.State != stateMenu {
		t.Fatalf("Update() state = %v, want %v", got.State, stateMenu)
	}
	if cmd == nil {
		t.Fatal("Update() command is nil, want quit command")
	}
	if !strings.Contains(got.ErrorMessage, "20%") {
		t.Fatalf("ErrorMessage = %q, want the threshold", got.ErrorMessage)
	}
}

func TestBatteryEventUpdatesPercentage(t *testing.T) {
	m, _ := startTestSession(t, Options{Base: session.Config{BatteryThreshold: 20}, Start: true})
	r := m.sessions.current()
	ev := session.Event{Type: session.EventBattery, Snapshot: session.Snapshot{Battery: session.Battery{Percent: 21, Available: true, Threshold: 20}}}

	got, cmd := Update(sessionEventMsg{r: r, ev: ev}, m)
	if got.State != stateRunning {
		t.Fatalf("Update() state = %v, want %v", got.State, stateRunning)
	}
	if got.BatteryPercentage != 21 {
		t.Fatalf("Update() BatteryPercentage = %d, want 21", got.BatteryPercentage)
	}
	if cmd == nil {
		t.Fatal("Update() command is nil, want to keep waiting for events")
	}
	stopTestSession(t, got)
}

func TestStaleSessionEventIgnored(t *testing.T) {
	m, _ := startTestSession(t, Options{Start: true})
	stale := &runner{}
	got, cmd := Update(sessionEventMsg{r: stale, ev: session.Event{Type: session.EventStopped, Reason: session.ReasonDuration}}, m)
	if got.State != stateRunning || cmd != nil {
		t.Fatalf("stale event changed state to %v (cmd %v)", got.State, cmd)
	}
	stopTestSession(t, got)
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

func TestCLIHelpRendersFullMenuWithoutScrollFooter(t *testing.T) {
	m := InitialModel()
	m.ShowHelp = true
	m.Width = 80
	m.Height = 0

	view := View(m)
	if strings.Contains(view, "pgup/pgdn") || strings.Contains(view, "esc/q close") {
		t.Fatalf("CLI help should not render scroll footer:\n%s", view)
	}
	if !strings.Contains(view, "Examples:") || !strings.Contains(view, "Navigation:") {
		t.Fatalf("CLI help should render full help content:\n%s", view)
	}
}

func TestHelpPopupUsesMoreAvailableSpace(t *testing.T) {
	width, height := helpPopupSize(120, 40)

	if width != maxHelpPopupWidth {
		t.Fatalf("helpPopupSize() width = %d, want %d", width, maxHelpPopupWidth)
	}
	if height != maxHelpPopupHeight {
		t.Fatalf("helpPopupSize() height = %d, want %d", height, maxHelpPopupHeight)
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

func TestErrorDisplay(t *testing.T) {
	m := Model{
		State:        stateMenu,
		ErrorMessage: "test error",
	}
	view := View(m)

	if !strings.Contains(view, "test error") {
		t.Error("expected view to show error message")
	}
}

func platformBatteryStatus(percentage int) platform.BatteryStatus {
	return platform.BatteryStatus{Percentage: percentage, Available: true}
}

func stubBatteryStatus(status platform.BatteryStatus, err error) func() {
	original := readBatteryStatus
	readBatteryStatus = func() (platform.BatteryStatus, error) {
		return status, err
	}
	return func() {
		readBatteryStatus = original
	}
}

func TestStartAndStopThroughSessionEngine(t *testing.T) {
	m, deps := startTestSession(t, Options{})
	m.Selected = 0

	m, cmd := Update(tea.KeyMsg{Type: tea.KeyEnter}, m)
	if m.State != stateRunning || cmd == nil {
		t.Fatalf("state = %v, cmd = %v; want running with commands", m.State, cmd)
	}
	ev := nextEvent(t, m)
	if ev.ev.Type != session.EventStarted {
		t.Fatalf("first event = %s, want started", ev.ev.Type)
	}
	m, _ = Update(ev, m)
	if deps.power.acquires.Load() != 1 {
		t.Fatalf("power acquired %d times, want 1", deps.power.acquires.Load())
	}

	m, cmd = Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}, m)
	if m.State != stateMenu || cmd != nil {
		t.Fatalf("after stop: state = %v, cmd = %v", m.State, cmd)
	}
	if r := deps.power.releases.Load(); r != 1 {
		t.Fatalf("power released %d times, want 1", r)
	}
	if m.sessions.current() != nil {
		t.Fatal("session slot not cleared")
	}
}

func TestTimedSessionEndsAndQuits(t *testing.T) {
	m, deps := startTestSession(t, Options{Base: session.Config{Active: true}})
	m.State = stateTimedInput
	m.textInput = newMinutesTextInput()
	m.textInput.SetValue("1")
	m, _ = Update(tea.KeyMsg{Type: tea.KeyEnter}, m)
	if m.State != stateRunning || m.Duration != time.Minute {
		t.Fatalf("state = %v duration = %v", m.State, m.Duration)
	}
	if snap := m.sessions.current().sess.Snapshot(); snap.Mode != session.ModeDuration || !snap.Active {
		t.Fatalf("session snapshot = %+v", snap)
	}

	// Simulate the session reporting the end of its duration.
	r := m.sessions.current()
	m, cmd := Update(sessionEventMsg{r: r, ev: session.Event{Type: session.EventStopped, Reason: session.ReasonDuration}}, m)
	if m.State != stateMenu || cmd == nil {
		t.Fatalf("state = %v cmd = %v, want menu + quit", m.State, cmd)
	}
	if a, rel := deps.power.acquires.Load(), deps.power.releases.Load(); a != rel {
		t.Fatalf("acquires=%d releases=%d", a, rel)
	}
}

func TestStartFromOptionsAndShutdown(t *testing.T) {
	until := time.Now().Add(2 * time.Hour)
	m, deps := startTestSession(t, Options{Base: session.Config{Until: until}, Start: true})
	if m.State != stateRunning || !m.Clock.Equal(until) {
		t.Fatalf("state = %v clock = %v", m.State, m.Clock)
	}
	if m.Init() == nil {
		t.Fatal("Init() returned no commands for a running session")
	}
	nextEvent(t, m) // started
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if a, r := deps.power.acquires.Load(), deps.power.releases.Load(); a != 1 || r != 1 {
		t.Fatalf("acquires=%d releases=%d, want 1/1", a, r)
	}
}

func TestSessionErrorReturnsToMenu(t *testing.T) {
	m, deps := startTestSession(t, Options{})
	deps.power.err = errDenied
	m, _ = startSession(m, 0, time.Time{})
	ev := nextEvent(t, m)
	for ev.ev.Type != session.EventStopped {
		ev = nextEvent(t, m)
	}
	m, cmd := Update(ev, m)
	if m.State != stateMenu || cmd != nil {
		t.Fatalf("state = %v cmd = %v, want menu without quitting", m.State, cmd)
	}
	if !strings.Contains(m.ErrorMessage, "denied") {
		t.Fatalf("ErrorMessage = %q", m.ErrorMessage)
	}
}

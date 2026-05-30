package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stigoleg/keep-alive/internal/platform"
	"github.com/stigoleg/keep-alive/internal/util"
)

type batteryStatusMsg struct {
	status platform.BatteryStatus
	err    error
}

var readBatteryStatus = platform.GetBatteryStatus

func batteryPollCmd() tea.Cmd {
	return tea.Tick(batteryPollInterval, func(time.Time) tea.Msg {
		status, err := readBatteryStatus()
		return batteryStatusMsg{status: status, err: err}
	})
}

func runningCommands(m Model) tea.Cmd {
	var cmds []tea.Cmd
	if m.Duration > 0 {
		cmds = append(cmds, m.timer.Init(), m.progress.SetPercent(0))
	}
	if m.BatteryThreshold > 0 {
		cmds = append(cmds, batteryPollCmd())
	}
	return tea.Batch(cmds...)
}

// Update handles messages and updates the model accordingly.
func Update(msg tea.Msg, m Model) (Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.Width = sizeMsg.Width
		m.Height = sizeMsg.Height
		m.Help.Width = sizeMsg.Width
		m = syncHelpViewport(m)
		return m, nil
	}

	if m.ShowDependencyInfo {
		// Still process timer messages so progress and timeout continue under the overlay
		switch msg.(type) {
		case timer.TickMsg, timer.TimeoutMsg, batteryStatusMsg:
			return handleRunningState(msg, m)
		}
		return handleDependencyInfoState(msg, m)
	}
	if m.ShowHelp {
		// Still process timer messages so progress and timeout continue under the overlay
		switch msg.(type) {
		case timer.TickMsg, timer.TimeoutMsg, batteryStatusMsg:
			return handleRunningState(msg, m)
		}
		return handleHelpState(msg, m)
	}

	switch m.State {
	case stateMenu:
		return handleMenuState(msg, m)
	case stateTimedInput:
		return handleTimedInputState(msg, m)
	case stateClockInput:
		return handleClockInputState(msg, m)
	case stateBatteryInput:
		return handleBatteryInputState(msg, m)
	case stateRunning:
		return handleRunningState(msg, m)
	}

	return m, nil
}

// handleHelpState handles messages when help is being displayed
func handleHelpState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(keyMsg, m.Keys.ToggleHelp):
			m.ShowHelp = false
			return m, nil
		case key.Matches(keyMsg, m.Keys.Quit):
			m.ShowHelp = false
			return m, nil
		case key.Matches(keyMsg, m.Keys.Back):
			m.ShowHelp = false
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.HelpViewport, cmd = m.HelpViewport.Update(msg)
	return m, cmd
}

// handleDependencyInfoState handles messages when dependency info is being displayed
func handleDependencyInfoState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(keyMsg, m.Keys.ToggleDependencyInfo):
			m.ShowDependencyInfo = false
		case key.Matches(keyMsg, m.Keys.Quit):
			m.ShowDependencyInfo = false
		case key.Matches(keyMsg, m.Keys.Back):
			m.ShowDependencyInfo = false
		}
	}
	return m, nil
}

// handleMenuState handles messages in the menu state
func handleMenuState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleMenuKeyMsg(msg, m)
	}
	return m, nil
}

// handleMenuKeyMsg handles keyboard input in the menu state
func handleMenuKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.ToggleHelp):
		m.ShowHelp = true
		m = syncHelpViewport(m)
	case key.Matches(msg, m.Keys.ToggleDependencyInfo):
		if m.DependencyWarning != "" || m.ActivityWarning != "" {
			m.ShowDependencyInfo = true
		}
	case key.Matches(msg, m.Keys.Up):
		if m.Selected > 0 {
			m.Selected--
		}
	case key.Matches(msg, m.Keys.Down):
		if m.Selected < 3 {
			m.Selected++
		}
	case key.Matches(msg, m.Keys.Select):
		return handleMenuSelection(m)
	case msg.Type == tea.KeyEnter:
		// Fallback for tests sending KeyEnter type
		return handleMenuSelection(m)
	case key.Matches(msg, m.Keys.Quit):
		return handleQuit(m)
	case msg.String() == "a":
		m.SimulateActivity = !m.SimulateActivity
		m.ActivityWarning = activityWarningFor(m.SimulateActivity)
		return m, nil
	case msg.String() == "b":
		m.State = stateBatteryInput
		m.ErrorMessage = ""
		m.textInput = newBatteryTextInput(m.BatteryThreshold)
		return m, nil
	case msg.String() == "B":
		m.BatteryThreshold = 0
		m.BatteryPercentage = 0
		m.BatteryError = ""
		m.ErrorMessage = ""
		return m, nil
	}
	return m, nil
}

// handleMenuSelection processes the selected menu item
func handleMenuSelection(m Model) (Model, tea.Cmd) {
	switch m.Selected {
	case 0:
		return startSession(m, 0, time.Time{})
	case 1:
		m.State = stateTimedInput
		m.ErrorMessage = ""
		m.textInput = newMinutesTextInput()
	case 2:
		m.State = stateClockInput
		m.ErrorMessage = ""
		m.textInput = newClockTextInput()
	case 3:
		return handleQuit(m)
	}
	return m, nil
}

// handleClockInputState handles messages in the clock input state
func handleClockInputState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleClockInputKeyMsg(msg, m)
	}
	return m, nil
}

func handleClockInputKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.ToggleHelp):
		m.ShowHelp = true
		m = syncHelpViewport(m)
		return m, nil
	case key.Matches(msg, m.Keys.Back):
		m.State = stateMenu
		m.ErrorMessage = ""
		return m, nil
	case key.Matches(msg, m.Keys.Submit) || msg.Type == tea.KeyEnter:
		return handleClockInputSubmit(m)
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	if m.textInput.Value() == "" {
		m.ErrorMessage = "Clock Required • Enter a time (e.g., 22:00 or 10:00PM)"
	} else {
		m.ErrorMessage = ""
	}

	return m, cmd
}

func handleClockInputSubmit(m Model) (Model, tea.Cmd) {
	value := m.textInput.Value()
	if value == "" {
		m.ErrorMessage = "Clock Required • Enter a time (e.g., 22:00 or 10:00PM)"
		return m, nil
	}

	now := time.Now()
	target, err := util.ParseTimeStringWithNow(value, now)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	if target.Before(now) {
		target = target.Add(24 * time.Hour)
	}
	if !target.After(now) {
		m.ErrorMessage = "Invalid Clock • Please enter a future time"
		return m, nil
	}

	return startSession(m, target.Sub(now), target)
}

// handleBatteryInputState handles messages in the battery input state
func handleBatteryInputState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleBatteryInputKeyMsg(msg, m)
	}
	return m, nil
}

func handleBatteryInputKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.ToggleHelp):
		m.ShowHelp = true
		m = syncHelpViewport(m)
		return m, nil
	case key.Matches(msg, m.Keys.Back):
		m.State = stateMenu
		m.ErrorMessage = ""
		return m, nil
	case key.Matches(msg, m.Keys.Submit) || msg.Type == tea.KeyEnter:
		return handleBatteryInputSubmit(m)
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	if m.textInput.Value() == "" {
		m.ErrorMessage = "Battery Required • Enter a percentage from 1 to 100"
	} else {
		m.ErrorMessage = ""
	}

	return m, cmd
}

func handleBatteryInputSubmit(m Model) (Model, tea.Cmd) {
	value := strings.TrimSpace(m.textInput.Value())
	if value == "" {
		m.ErrorMessage = "Battery Required • Enter a percentage from 1 to 100"
		return m, nil
	}

	threshold, err := strconv.Atoi(value)
	if err != nil {
		m.ErrorMessage = "Invalid Battery • Enter a whole percentage from 1 to 100"
		return m, nil
	}
	if threshold < 1 || threshold > 100 {
		m.ErrorMessage = "Invalid Battery • Enter a percentage from 1 to 100"
		return m, nil
	}

	status, err := readBatteryStatus()
	if err != nil {
		m.ErrorMessage = "Battery Error • " + err.Error()
		return m, nil
	}
	if status.Percentage <= threshold {
		m.ErrorMessage = fmt.Sprintf("Invalid Battery • Current battery is %d%%, threshold must be lower", status.Percentage)
		return m, nil
	}

	m.BatteryThreshold = threshold
	m.BatteryPercentage = status.Percentage
	m.BatteryError = ""
	m.ErrorMessage = ""
	m.State = stateMenu
	return m, nil
}

// handleTimedInputState handles messages in the timed input state
func handleTimedInputState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleTimedInputKeyMsg(msg, m)
	}
	return m, nil
}

// handleTimedInputKeyMsg handles keyboard input in the timed input state
func handleTimedInputKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.ToggleHelp):
		m.ShowHelp = true
		m = syncHelpViewport(m)
		return m, nil
	case key.Matches(msg, m.Keys.Back):
		m.State = stateMenu
		m.ErrorMessage = ""
		return m, nil
	case key.Matches(msg, m.Keys.Submit) || msg.Type == tea.KeyEnter:
		return handleTimedInputSubmit(m)
	}

	// Route remaining input to text input component (allow letters like h/m)
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	// Minimal realtime feedback
	value := m.textInput.Value()
	if value == "" {
		m.ErrorMessage = "Duration Required • Enter minutes or duration (e.g., 30 or 2h30m)"
	} else {
		m.ErrorMessage = ""
	}

	return m, cmd
}

// handleTimedInputSubmit processes the submitted duration
func handleTimedInputSubmit(m Model) (Model, tea.Cmd) {
	value := m.textInput.Value()
	if value == "" {
		m.ErrorMessage = "Duration Required • Enter minutes or duration (e.g., 30 or 2h30m)"
		return m, nil
	}

	dur, err := util.ParseDuration(value)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	if dur <= 0 {
		m.ErrorMessage = "Invalid Input • Please enter a positive number"
		return m, nil
	}

	return startSession(m, dur, time.Time{})
}

func startSession(m Model, dur time.Duration, clock time.Time) (Model, tea.Cmd) {
	m.ActivityWarning = activityWarningFor(m.SimulateActivity)
	m.KeepAlive.SetSimulateActivity(m.SimulateActivity)

	var err error
	if dur > 0 {
		err = m.KeepAlive.StartTimed(dur)
	} else {
		err = m.KeepAlive.StartIndefinite()
	}
	if err != nil {
		m.ErrorMessage = "System Error • " + err.Error()
		return m, nil
	}

	m.State = stateRunning
	m.StartTime = time.Now()
	m.Duration = dur
	m.Clock = clock
	m.ErrorMessage = ""
	if dur > 0 {
		m.timer = timer.NewWithInterval(dur, time.Second/10)
	}
	return m, runningCommands(m)
}

// handleRunningState handles messages in the running state
func handleRunningState(msg tea.Msg, m Model) (Model, tea.Cmd) {
	// Always let progress process messages so SetPercent's internal messages are applied
	var cmds []tea.Cmd
	if pm := msg; pm != nil {
		var pc tea.Cmd
		var newProg tea.Model
		newProg, pc = m.progress.Update(pm)
		if pc != nil {
			cmds = append(cmds, pc)
		}
		if np, ok := newProg.(progress.Model); ok {
			m.progress = np
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleRunningKeyMsg(msg, m)
	case timer.TickMsg:
		var tcmd tea.Cmd
		m.timer, tcmd = m.timer.Update(msg)
		if tcmd != nil {
			cmds = append(cmds, tcmd)
		}
		if m.Duration > 0 {
			remaining := time.Until(m.StartTime.Add(m.Duration))
			if remaining < 0 {
				remaining = 0
			}
			percent := 1 - (float64(remaining) / float64(m.Duration))
			if percent < 0 {
				percent = 0
			}
			if percent > 1 {
				percent = 1
			}
			cmds = append(cmds, m.progress.SetPercent(percent))
		}
		return m, tea.Batch(cmds...)
	case timer.TimeoutMsg:
		return handleQuit(m)
	case batteryStatusMsg:
		return handleBatteryStatusMsg(msg, m)
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func handleBatteryStatusMsg(msg batteryStatusMsg, m Model) (Model, tea.Cmd) {
	if m.BatteryThreshold == 0 {
		return m, nil
	}

	if msg.err != nil {
		m.BatteryError = msg.err.Error()
		return m, batteryPollCmd()
	}

	m.BatteryPercentage = msg.status.Percentage
	m.BatteryError = ""
	if m.BatteryPercentage <= m.BatteryThreshold {
		m.ErrorMessage = fmt.Sprintf("Battery reached %d%% threshold", m.BatteryThreshold)
		return handleQuit(m)
	}

	return m, batteryPollCmd()
}

// handleRunningKeyMsg handles keyboard input in the running state
func handleRunningKeyMsg(msg tea.KeyMsg, m Model) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Quit):
		return handleQuit(m)
	case key.Matches(msg, m.Keys.ToggleHelp):
		m.ShowHelp = true
		m = syncHelpViewport(m)
	case key.Matches(msg, m.Keys.ToggleDependencyInfo):
		if m.DependencyWarning != "" || m.ActivityWarning != "" {
			m.ShowDependencyInfo = true
		}
	case key.Matches(msg, m.Keys.Stop):
		return handleStopAndReturn(m)
	}
	return m, nil
}

func activityWarningFor(enabled bool) string {
	if !enabled {
		return ""
	}
	status := platform.GetActivitySimulationStatus()
	if status.Available {
		return ""
	}
	return strings.TrimSpace(status.Message)
}

// cleanup stops the keep-alive process and resets the model state
func cleanup(m Model) (Model, error) {
	if err := m.KeepAlive.Stop(); err != nil {
		return m, err
	}

	// Reset all state
	m.State = stateMenu
	m.Duration = 0
	m.Clock = time.Time{}
	m.StartTime = time.Time{}
	m.ErrorMessage = ""
	m.BatteryThreshold = 0
	m.BatteryPercentage = 0
	m.BatteryError = ""
	// Reset timer and progress models
	m.timer = timer.Model{}
	m.progress = progress.New(progress.WithDefaultGradient(), progress.WithWidth(34))

	return m, nil
}

// handleStopAndReturn stops the keep-alive and returns to the main menu
func handleStopAndReturn(m Model) (Model, tea.Cmd) {
	cleanedModel, err := cleanup(m)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	return cleanedModel, nil
}

// handleQuit handles quitting the application
func handleQuit(m Model) (Model, tea.Cmd) {
	cleanedModel, err := cleanup(m)
	if err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	return cleanedModel, tea.Quit
}

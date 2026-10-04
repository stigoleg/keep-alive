// Package session runs one keep-alive session: it holds the power assertion,
// drives the activity simulator, watches the end conditions and publishes an
// event stream. All session state is owned by the goroutine inside Run; the
// public methods talk to it over channels.
package session

import (
	"errors"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/notify"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/proc"
	"github.com/stigoleg/keep-alive/v2/internal/schedule"
)

// Config describes what the session should do.
type Config struct {
	Duration         time.Duration // 0 = none
	Until            time.Time     // zero = none (resolved from -c)
	BatteryThreshold int           // 0 = none
	// BatteryPause pauses the session at the threshold instead of ending
	// it (the login service, which nobody restarts by hand): no power hold,
	// no simulated activity until the battery is BatteryResumeMargin above
	// the threshold or charging.
	BatteryPause bool
	Active       bool
	Activity     activity.Config
	KeepDisplay  bool
	// Schedule limits keeping awake to its windows; outside them the
	// session is paused (no power hold, no simulated activity). nil = always.
	Schedule *schedule.Schedule
	// WatchPIDs and WatchProcess end the session (ReasonProcessExited) once
	// none of the processes is running.
	WatchPIDs    []int
	WatchProcess string
	// Command names the command `keepalive run` waits for; it only shows up
	// in Snapshot.Watching (the CLI stops the session when it exits).
	Command string
	// StartWarnings are shown as warning events right after the start
	// event; the caller logs them.
	StartWarnings []string
}

// Deps are the session's collaborators; tests pass fakes.
type Deps struct {
	Clock    clock.Clock
	Power    power.Inhibitor
	Activity activity.Simulator
	Battery  func() (platform.BatteryStatus, error)
	// Processes looks up watched processes; nil means the system, without
	// this process.
	Processes proc.Lister
	// Notifier shows desktop notifications for unusual stops and problems;
	// nil disables them.
	Notifier notify.Notifier
	// NotifyLimiter decides whether a notification of a kind may be shown;
	// the login service passes one that remembers across restarts. nil
	// means once per kind every NotifyInterval within this session.
	NotifyLimiter NotifyLimiter
}

// NotifyLimiter rate-limits notifications by kind ("stopped", "activity",
// "power"). Allow records the notification when it returns true.
type NotifyLimiter interface {
	Allow(kind string, now time.Time) bool
}

// Reason says why a session ended.
type Reason string

const (
	ReasonUser          Reason = "user"
	ReasonDuration      Reason = "duration"
	ReasonUntil         Reason = "until"
	ReasonBattery       Reason = "battery"
	ReasonSignal        Reason = "signal"
	ReasonError         Reason = "error"
	ReasonProcessExited Reason = "process_exited"
	ReasonCommandExited Reason = "command_exited"
	ReasonIPC           Reason = "ipc"
)

// Result is returned by Run.
type Result struct {
	Reason    Reason
	Err       error
	StartedAt time.Time
	EndedAt   time.Time
}

// Mode is how the session ends on its own.
type Mode string

const (
	ModeIndefinite Mode = "indefinite"
	ModeDuration   Mode = "duration"
	ModeUntil      Mode = "until"
)

// Battery is the battery part of a Snapshot.
type Battery struct {
	Percent   int
	Available bool
	Threshold int  // 0 = none
	Pause     bool // the threshold pauses the session instead of ending it
}

// Pause says why a running session holds nothing ("" = it does not pause).
type Pause string

const (
	PauseSchedule Pause = "schedule" // outside the work hours
	PauseBattery  Pause = "battery"  // battery at or below the threshold
)

// Snapshot is the externally visible session state.
type Snapshot struct {
	Running     bool
	StartedAt   time.Time
	EndsAt      time.Time // zero when the session has no end time
	Remaining   time.Duration
	Mode        Mode
	Active      bool
	Activity    activity.Status
	Battery     Battery
	KeepDisplay bool
	PowerHold   string
	// Schedule is the normalized work-hours spec ("" = none). InWindow is
	// false while the session is paused outside the work hours (always true
	// without a schedule); NextChange is the next enter or leave (zero when
	// there is none).
	Schedule   string
	InWindow   bool
	NextChange time.Time
	// Watching describes the watched processes, e.g. "zoom" or "process 4242".
	Watching string
	// Paused is why the session holds no power and simulates nothing right
	// now; the work hours win when both apply.
	Paused Pause
}

// EventType names an Event.
type EventType string

const (
	EventStarted  EventType = "started"
	EventSnapshot EventType = "snapshot" // any state change, plus a heartbeat
	EventActivity EventType = "activity" // activity status changed
	EventBattery  EventType = "battery"  // every battery poll
	EventWarning  EventType = "warning"
	EventSchedule EventType = "schedule" // entered or left the work hours
	EventStopping EventType = "stopping"
	EventStopped  EventType = "stopped"
)

// Event is published to subscribers.
type Event struct {
	Time     time.Time
	Type     EventType
	Snapshot Snapshot
	Message  string
	Reason   Reason
}

// Timing of the session loop.
const (
	HeartbeatInterval   = 30 * time.Second
	BatteryPollInterval = 30 * time.Second
	// MinRemainingAfterShorten is the floor a negative Extend cannot cross.
	MinRemainingAfterShorten = time.Minute
	// ActivityStopTimeout bounds how long stopping waits for the simulator.
	ActivityStopTimeout = 5 * time.Second
	// WatchInterval is how often watched processes are checked.
	WatchInterval = 2 * time.Second
	// NotifyInterval is the minimum gap between two notifications of the
	// same kind.
	NotifyInterval = 10 * time.Minute
	// BatteryResumeMargin is how far above the threshold the battery must
	// be before a battery pause ends (unless it is charging), so the
	// session does not flap around the threshold.
	BatteryResumeMargin = 5
)

// BatteryResumeAt is the level at which a battery pause at threshold ends
// without charging, or 0 when no level can end it (threshold plus
// BatteryResumeMargin is above 100%): then only charging does.
func BatteryResumeAt(threshold int) int {
	if at := threshold + BatteryResumeMargin; at <= 100 {
		return at
	}
	return 0
}

func (c Config) mode() Mode {
	switch {
	case !c.Until.IsZero():
		return ModeUntil
	case c.Duration > 0:
		return ModeDuration
	default:
		return ModeIndefinite
	}
}

func (c Config) validate() error {
	switch {
	case c.Duration < 0:
		return errors.New("session: negative duration")
	case c.Duration > 0 && !c.Until.IsZero():
		return errors.New("session: duration and until are mutually exclusive")
	case c.BatteryThreshold < 0 || c.BatteryThreshold > 100:
		return errors.New("session: battery threshold must be 0-100")
	}
	return nil
}

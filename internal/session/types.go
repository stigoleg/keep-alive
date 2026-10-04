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
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
)

// Config describes what the session should do.
type Config struct {
	Duration         time.Duration // 0 = none
	Until            time.Time     // zero = none (resolved from -c)
	BatteryThreshold int           // 0 = none
	Active           bool
	Activity         activity.Config
	KeepDisplay      bool
	ScheduleSpec     string // phase 4
	WatchPIDs        []int  // phase 4
	WatchProcess     string // phase 4
}

// Deps are the session's collaborators; tests pass fakes.
type Deps struct {
	Clock    clock.Clock
	Power    power.Inhibitor
	Activity activity.Simulator
	Battery  func() (platform.BatteryStatus, error)
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
	ReasonProcessExited Reason = "process_exited" // phase 4
	ReasonCommandExited Reason = "command_exited" // phase 4
	ReasonIPC           Reason = "ipc"            // phase 5
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
	Threshold int // 0 = none
}

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
}

// EventType names an Event.
type EventType string

const (
	EventStarted  EventType = "started"
	EventSnapshot EventType = "snapshot" // any state change, plus a heartbeat
	EventActivity EventType = "activity" // activity status changed
	EventBattery  EventType = "battery"  // every battery poll
	EventWarning  EventType = "warning"
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
)

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

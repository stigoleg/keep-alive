// Package activity simulates user input so chat apps (Slack, Teams) keep the
// user shown as active.
package activity

import (
	"context"
	"time"
)

// State is the simulator's current phase.
type State string

const (
	StateOff          State = "off"
	StateWaitingIdle  State = "waiting_idle"
	StateSimulating   State = "simulating"
	StatePausedUser   State = "paused_user"
	StatePausedLocked State = "paused_locked"
	StateDegraded     State = "degraded"
)

// Status is reported by a Simulator whenever its state changes.
type Status struct {
	State State
	// Method names the input mechanism, e.g. "CoreGraphics mouse events".
	Method string
	// Reason explains the state, Hint suggests a fix (mostly for degraded).
	Reason, Hint string
	LastBurst    time.Time
	Idle         time.Duration
}

// Config tunes the simulator.
type Config struct {
	// IdleThreshold is how long the user must be idle before simulating.
	IdleThreshold time.Duration
	// Interval is the mean gap between activity bursts.
	Interval time.Duration
	// Keys adds a harmless key press per burst.
	Keys bool
}

// Default values for Config.
const (
	DefaultIdleThreshold = 2 * time.Minute
	DefaultInterval      = 30 * time.Second
	MinIdleThreshold     = 10 * time.Second
	MinInterval          = 5 * time.Second
)

// Simulator runs until ctx is done, calling report on every status change.
type Simulator interface {
	Run(ctx context.Context, cfg Config, report func(Status)) error
}

// New returns the simulator for the current OS.
func New() Simulator { return newLegacy() }

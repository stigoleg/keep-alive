// Package power keeps the system (and optionally the display) awake. Callers
// acquire a Hold and release it when the session ends.
package power

import "context"

// Options describes the hold being requested.
type Options struct {
	// KeepDisplay keeps the display on as well; false keeps only the system
	// awake.
	KeepDisplay bool
	// Reason is shown by OS tools that list power assertions.
	Reason string
}

// Hold is an acquired power assertion. Release must be called exactly once.
type Hold interface {
	Release() error
	// Describe is a short human description of the mechanism in use, e.g.
	// "caffeinate -dims".
	Describe() string
}

// Inhibitor acquires power holds.
type Inhibitor interface {
	Acquire(ctx context.Context, o Options) (Hold, error)
	Name() string
}

// New returns the inhibitor for the current OS.
func New() Inhibitor { return newLegacy() }

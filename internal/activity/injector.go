package activity

import (
	"context"
	"time"
)

// Injector posts synthetic input through one OS mechanism. Every injector is
// also either an AbsoluteInjector or a RelativeInjector.
type Injector interface {
	// Name is the method shown to the user, e.g. "CoreGraphics" or "uinput".
	Name() string
	// Available reports why the injector cannot be used right now; an
	// *Unavailable error carries a hint.
	Available() error
	// Tap presses and releases a harmless key (right Shift).
	Tap() error
	// Diagnose explains why bursts may not reset the idle timer.
	Diagnose() (reason, hint string)
	Close() error
}

// AbsoluteInjector warps the pointer to absolute screen coordinates.
type AbsoluteInjector interface {
	Injector
	MoveTo(x, y float64) error
	// Position is the current pointer location.
	Position() (x, y float64, ok bool)
	// Bounds is the display area the pointer may move in.
	Bounds() (Rect, bool)
}

// RelativeInjector can only move the pointer by a delta.
type RelativeInjector interface {
	Injector
	MoveBy(dx, dy int) error
}

// pathPlayer is implemented by injectors that replay a whole burst at once,
// e.g. xdotool chains every move into one process.
type pathPlayer interface {
	Play(ctx context.Context, ox, oy float64, p Path) error
}

// finisher is implemented by absolute injectors whose return stroke can
// land a pixel off the origin (SendInput); FinishAt puts the pointer back
// exactly without generating input.
type finisher interface {
	FinishAt(ox, oy float64)
}

// minStepper is implemented by relative injectors that are too slow for a
// move every few milliseconds (ydotool starts a process per move).
type minStepper interface {
	MinStep() time.Duration
}

// IdleSource reads how long the user has not touched any input device.
type IdleSource interface {
	Name() string
	Idle() (time.Duration, error)
}

// LockSource reports whether the session is locked.
type LockSource interface {
	Locked() (bool, error)
}

// Rect is a screen area in the injector's coordinate space.
type Rect struct{ X, Y, W, H float64 }

// Contains reports whether (x, y) lies inside r.
func (r Rect) Contains(x, y float64) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Unavailable is returned when an injector or backend cannot be used.
type Unavailable struct {
	Reason string
	Hint   string
}

func (u *Unavailable) Error() string { return u.Reason }

// sleepFunc waits d or until ctx is done.
type sleepFunc func(ctx context.Context, d time.Duration) error

func realSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

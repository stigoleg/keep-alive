package activity

import (
	"fmt"
	"math"
	"strings"
)

// Decisions behind the Windows backend, kept free of system calls so they
// are tested on every OS.

// lockedSessionOrDesktop asks the WTS session flags first and, when they
// cannot be read or hold an unknown value, the input desktop.
func lockedSessionOrDesktop(wts, desktop func() (bool, error)) (bool, error) {
	locked, err := wts()
	if err == nil {
		return locked, nil
	}
	locked, derr := desktop()
	if derr != nil {
		return false, fmt.Errorf("%v; input desktop: %v", err, derr)
	}
	return locked, nil
}

// inputDesktopLocked reads what OpenInputDesktop showed. While the session
// is locked the input desktop is Winlogon's secure desktop, which a user
// process may not open: access denied means locked, and so does any input
// desktop other than "Default".
func inputDesktopLocked(name string, openErr error, accessDenied bool) (bool, error) {
	if openErr != nil {
		if accessDenied {
			return true, nil
		}
		return false, openErr
	}
	return !strings.EqualFold(name, "Default"), nil
}

// snapBack puts the cursor exactly on the origin when the return stroke
// landed one or two pixels off, as normalised SendInput coordinates can.
// SetCursorPos is not input, so this does not count as activity. A cursor
// further off is the user's and is left alone.
func snapBack(get func() (x, y float64, ok bool), set func(x, y int32) error, ox, oy float64) (bool, error) {
	x, y, ok := get()
	if !ok {
		return false, nil
	}
	tx, ty := math.Round(ox), math.Round(oy)
	off := max(math.Abs(x-tx), math.Abs(y-ty))
	if off < 1 || off > 2 {
		return false, nil
	}
	return true, set(int32(tx), int32(ty))
}

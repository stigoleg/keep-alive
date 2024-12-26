package ui

import (
	"time"

	"github.com/stigoleg/keep-alive/internal/keepalive"
)

// UI handles the user interface interactions for the keep-alive functionality
type UI struct {
	keeper *keepalive.Keeper
}

// TimeRemaining returns the remaining time for the keep-alive timer
func (u *UI) TimeRemaining() time.Duration {
	return u.keeper.TimeRemaining()
}

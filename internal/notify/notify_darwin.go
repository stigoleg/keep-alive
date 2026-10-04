package notify

import (
	"context"
	"os/exec"
)

type osascript struct{}

func newPlatform() Notifier { return osascript{} }

// Notify shows a Notification Center banner. macOS attributes it to Script
// Editor, whose notifications the user may have to allow in System Settings.
func (osascript) Notify(ctx context.Context, title, body string) error {
	return run(ctx, "osascript", osascriptArgs(title, body), nil)
}

func available() (bool, string) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return false, "osascript not found"
	}
	return true, "Notification Center via osascript"
}

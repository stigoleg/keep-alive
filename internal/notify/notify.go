// Package notify sends desktop notifications: Notification Center through
// osascript on macOS, org.freedesktop.Notifications on the D-Bus session bus
// (falling back to notify-send) on Linux, and a toast through PowerShell on
// Windows.
//
// Every Notify call is bounded by Timeout, also when the caller's context
// has no deadline, and returns its error instead of printing it.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Timeout bounds every Notify call.
const Timeout = 5 * time.Second

// appName is what notifications are attributed to where the platform lets
// us choose.
const appName = "keepalive"

// Notifier shows a desktop notification.
type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// New returns the Notifier for this operating system.
func New() Notifier { return newPlatform() }

// Available reports whether notifications can be sent and, in either case,
// a short description for doctor: the mechanism used, or why there is none.
func Available() (bool, string) { return available() }

// run starts name with args and waits for it, at most until ctx ends or
// Timeout passes. The process is killed then and run returns at once with
// the context's error. A failed command's output is part of the error.
func run(ctx context.Context, name string, args []string, prepare func(*exec.Cmd)) error {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if prepare != nil {
		prepare(cmd)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("notify: %s: %w", name, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if ctx.Err() != nil {
			return fmt.Errorf("notify: %s: %w", name, ctx.Err())
		}
		if err != nil {
			if msg := strings.TrimSpace(out.String()); msg != "" {
				return fmt.Errorf("notify: %s: %w: %s", name, err, msg)
			}
			return fmt.Errorf("notify: %s: %w", name, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("notify: %s: %w", name, ctx.Err())
	}
}

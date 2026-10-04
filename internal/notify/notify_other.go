//go:build !darwin && !linux && !windows

package notify

import (
	"context"
	"errors"
	"runtime"
)

type unsupported struct{}

func newPlatform() Notifier { return unsupported{} }

func (unsupported) Notify(context.Context, string, string) error {
	return errors.ErrUnsupported
}

func available() (bool, string) {
	return false, "notifications are not supported on " + runtime.GOOS
}

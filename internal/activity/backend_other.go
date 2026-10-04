//go:build !darwin && !linux && !windows

package activity

import (
	"context"
	"runtime"
)

func newBackend(_ context.Context, keys bool) *backend {
	return &backend{open: func() (Injector, error) {
		return nil, &Unavailable{Reason: "activity simulation is not supported on " + runtime.GOOS}
	}}
}

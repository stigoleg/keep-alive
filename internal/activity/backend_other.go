//go:build !darwin && !linux && !windows

package activity

import "runtime"

func newBackend(keys bool) *backend {
	return &backend{open: func() (Injector, error) {
		return nil, &Unavailable{Reason: "activity simulation is not supported on " + runtime.GOOS}
	}}
}

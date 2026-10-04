//go:build darwin && !cgo

package activity

import "context"

func newBackend(_ context.Context, keys bool) *backend {
	return &backend{open: func() (Injector, error) {
		return nil, &Unavailable{
			Reason: "unavailable: built without cgo",
			Hint:   "use a keepalive build made with CGO_ENABLED=1; macOS input simulation needs CoreGraphics",
		}
	}}
}

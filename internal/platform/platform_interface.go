package platform

import "context"

// KeepAlive defines the interface for platform-specific keep-alive functionality
type KeepAlive interface {
	Start(ctx context.Context) error
	Stop() error
}

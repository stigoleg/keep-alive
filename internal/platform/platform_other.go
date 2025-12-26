//go:build !darwin && !windows && !linux

package platform

import (
	"context"
	"errors"
)

// unsupportedKeepAlive implements the KeepAlive interface for unsupported platforms
type unsupportedKeepAlive struct{}

func (k *unsupportedKeepAlive) Start(ctx context.Context) error {
	return errors.New("unsupported platform")
}

func (k *unsupportedKeepAlive) Stop() error {
	return errors.New("unsupported platform")
}

func (k *unsupportedKeepAlive) SetSimulateActivity(simulate bool) {
	// No-op on unsupported platforms
}

// GetDependencyMessage returns empty string on unsupported platforms
func GetDependencyMessage() string {
	return ""
}

// NewKeepAlive creates a new platform-specific keep-alive instance
func NewKeepAlive() (KeepAlive, error) {
	return &unsupportedKeepAlive{}, nil
}

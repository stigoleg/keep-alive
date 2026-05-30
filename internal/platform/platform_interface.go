package platform

import "context"

// KeepAlive defines the interface for platform-specific keep-alive functionality
type KeepAlive interface {
	Start(ctx context.Context) error
	Stop() error
	SetSimulateActivity(simulate bool)
}

// ActivitySimulationStatus describes whether --active can emit real user input.
type ActivitySimulationStatus struct {
	Available bool
	Method    string
	Message   string
}

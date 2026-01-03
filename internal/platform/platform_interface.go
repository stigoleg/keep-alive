package platform

import "context"

// KeepAlive defines the interface for platform-specific keep-alive functionality
type KeepAlive interface {
	Start(ctx context.Context) error
	Stop() error
	SetSimulateActivity(simulate bool)
}

// SimulationCapability represents the result of checking if activity simulation will work
type SimulationCapability struct {
	// CanSimulate indicates whether activity simulation will work on this system
	CanSimulate bool

	// ErrorMessage is a user-friendly error message if simulation won't work
	ErrorMessage string

	// Instructions provides step-by-step instructions to fix the issue
	Instructions string

	// CanPrompt indicates whether we can trigger a system permission dialog
	CanPrompt bool
}

// CheckActivitySimulationCapability checks if the platform can simulate user activity.
// This should be called before enabling the --active flag to provide early feedback.
// Each platform (darwin, windows, linux) implements this function.
// func CheckActivitySimulationCapability() SimulationCapability

// PromptActivitySimulationPermission triggers the system permission dialog if available.
// On macOS, this opens the Accessibility permission prompt.
// On other platforms, this is a no-op.
// func PromptActivitySimulationPermission()

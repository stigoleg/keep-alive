package platform

import "time"

// Common activity simulation constants used across all platforms
const (
	// ActivityInterval is the interval for system-level keep-alive assertions
	ActivityInterval = 10 * time.Second

	// ChatAppActivityInterval is the interval for chat app activity simulation
	// Teams/Slack typically mark as away after 5 minutes, but poll more frequently
	ChatAppActivityInterval = 20 * time.Second

	// IdleThreshold is the minimum idle time before simulating activity
	// This prevents interference when the user is actively using the computer
	IdleThreshold = 5 * time.Second

	// Mouse movement parameters for activity simulation
	MouseMinSizePixels = 5.0
	MouseMaxSizePixels = 20.0
	MouseMinMovePixels = 3.0
	MouseMaxMovePixels = 6.0

	// Timing characteristics for mouse movements (in seconds)
	MouseBaseDelayMinSeconds = 0.005
	MouseBaseDelayMaxSeconds = 0.12
	MouseReturnDelayMin      = 0.01
	MouseReturnDelayMax      = 0.05

	// Movement pattern probabilities
	MousePauseProbability        = 0.12
	MousePauseDurationMin        = 0.15
	MousePauseDurationMax        = 0.4
	MouseIntermediateProbability = 0.35
	MouseIntermediateDistanceMin = 8.0

	// Movement speed factors
	MouseSpeedFactorMin     = 0.7
	MouseSpeedFactorMax     = 1.3
	MouseSpeedFactorLongDist = 1.2
	MouseLongDistanceThreshold = 10.0

	// Intermediate point parameters
	MouseIntermediatePositionFactor = 0.4
	MouseIntermediateJitter          = 1.5
	MouseIntermediateSpeedMin        = 0.6
	MouseIntermediateSpeedMax        = 1.4
)


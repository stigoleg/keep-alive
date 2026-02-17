package platform

import "time"

// Common activity simulation constants used across all platforms
const (
	// ActivityInterval is the interval for system-level keep-alive assertions
	ActivityInterval = 10 * time.Second

	// ChatAppActivityInterval is the interval for chat app activity simulation
	// Run jitter every 30s once the user is idle beyond IdleThreshold.
	ChatAppActivityInterval = 30 * time.Second

	// ChatAppCheckInterval is how often we check idle state for activity simulation.
	// Actual jitter execution remains gated by ChatAppActivityInterval.
	ChatAppCheckInterval = 5 * time.Second

	// IdleThreshold is the minimum idle time before simulating activity
	// This prevents interference when the user is actively using the computer
	IdleThreshold = 2 * time.Minute

	// SyntheticIdleResetTolerance is the allowed drift when comparing observed idle
	// time against the last synthetic jitter timestamp.
	SyntheticIdleResetTolerance = 4 * time.Second

	// Round jitter path geometry
	MouseJitterRadiusMin       = 18.0
	MouseJitterRadiusMax       = 45.0
	MouseJitterRadiusVariation = 0.25
	MouseJitterPointsMin       = 8
	MouseJitterPointsMax       = 14

	// Jitter session duration target (0.5s +/- 0.1s)
	MouseJitterSessionDurationMin = 400 * time.Millisecond
	MouseJitterSessionDurationMax = 600 * time.Millisecond

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
	MouseSpeedFactorMin        = 0.7
	MouseSpeedFactorMax        = 1.3
	MouseSpeedFactorLongDist   = 1.2
	MouseLongDistanceThreshold = 10.0

	// Intermediate point parameters
	MouseIntermediatePositionFactor = 0.4
	MouseIntermediateJitter         = 1.5
	MouseIntermediateSpeedMin       = 0.6
	MouseIntermediateSpeedMax       = 1.4
)

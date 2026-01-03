// Package patterns provides mouse movement pattern generation for activity simulation.
package patterns

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// Activity simulation constants used across all platforms.
const (
	// ActivityInterval is the interval for system-level keep-alive assertions.
	ActivityInterval = 10 * time.Second

	// ChatAppActivityInterval is the interval for chat app activity simulation.
	// Teams/Slack typically mark as away after 5 minutes, but poll more frequently.
	ChatAppActivityInterval = 20 * time.Second

	// IdleThreshold is the minimum idle time before simulating activity.
	// This prevents interference when the user is actively using the computer.
	IdleThreshold = 5 * time.Second

	// ActiveLogInterval is how often to log when skipping simulation due to user activity.
	ActiveLogInterval = 2 * time.Minute

	// Mouse movement parameters for activity simulation.
	MouseMinSizePixels = 5.0
	MouseMaxSizePixels = 20.0

	// Timing characteristics for mouse movements (in seconds).
	MouseBaseDelayMinSeconds = 0.005
	MouseBaseDelayMaxSeconds = 0.12
	MouseReturnDelayMin      = 0.01
	MouseReturnDelayMax      = 0.05

	// Movement pattern probabilities.
	MousePauseProbability        = 0.12
	MousePauseDurationMin        = 0.15
	MousePauseDurationMax        = 0.4
	MouseIntermediateProbability = 0.35
	MouseIntermediateDistanceMin = 8.0

	// Movement speed factors.
	MouseSpeedFactorMin        = 0.7
	MouseSpeedFactorMax        = 1.3
	MouseSpeedFactorLongDist   = 1.2
	MouseLongDistanceThreshold = 10.0

	// Intermediate point parameters.
	MouseIntermediatePositionFactor = 0.4
	MouseIntermediateJitter         = 1.5
	MouseIntermediateSpeedMin       = 0.6
	MouseIntermediateSpeedMax       = 1.4
)

// Point represents a point in a mouse movement pattern.
type Point struct {
	X float64
	Y float64
}

// Generator generates natural mouse movement patterns.
type Generator struct {
	rnd *rand.Rand
}

// NewGenerator creates a new pattern generator with a random source.
func NewGenerator(rnd *rand.Rand) *Generator {
	return &Generator{rnd: rnd}
}

// GenerateShapePoints generates a random mouse movement pattern.
// Returns points relative to origin (0,0) that should be applied as offsets.
func (g *Generator) GenerateShapePoints() []Point {
	shapeType := g.rnd.Intn(4) // 0=circle, 1=square, 2=zigzag, 3=random walk

	size := MouseMinSizePixels + g.rnd.Float64()*(MouseMaxSizePixels-MouseMinSizePixels)
	numPoints := 4 + g.rnd.Intn(8) // 4-11 points

	if numPoints < 4 {
		numPoints = 4
	}

	switch shapeType {
	case 0:
		return g.buildCirclePoints(numPoints, size)
	case 1:
		return g.buildSquarePoints(numPoints, size)
	case 2:
		return g.buildZigZagPoints(numPoints, size)
	default:
		return g.buildRandomWalkPoints(numPoints, size)
	}
}

func (g *Generator) buildCirclePoints(numPoints int, size float64) []Point {
	points := make([]Point, 0, numPoints)
	for i := 0; i < numPoints; i++ {
		angle := 2 * math.Pi * float64(i) / float64(numPoints)
		points = append(points, Point{
			X: size * math.Cos(angle),
			Y: size * math.Sin(angle),
		})
	}
	return points
}

func (g *Generator) buildSquarePoints(numPoints int, size float64) []Point {
	side := int(math.Sqrt(float64(numPoints)))
	if side < 2 {
		side = 2
	}

	points := make([]Point, 0, side*4)

	// Top edge
	for i := 0; i < side; i++ {
		points = append(points, Point{
			X: size * float64(i) / float64(side-1),
			Y: 0,
		})
	}
	// Right edge
	for i := 1; i < side; i++ {
		points = append(points, Point{
			X: size,
			Y: size * float64(i) / float64(side-1),
		})
	}
	// Bottom edge
	for i := side - 2; i >= 0; i-- {
		points = append(points, Point{
			X: size * float64(i) / float64(side-1),
			Y: size,
		})
	}
	// Left edge
	for i := side - 2; i > 0; i-- {
		points = append(points, Point{
			X: 0,
			Y: size * float64(i) / float64(side-1),
		})
	}

	return points
}

func (g *Generator) buildZigZagPoints(numPoints int, size float64) []Point {
	points := make([]Point, 0, numPoints)

	for i := 0; i < numPoints; i++ {
		x := size * float64(i) / float64(numPoints-1)
		y := size * 0.5
		if i%2 == 0 {
			y = -size * 0.5
		}
		points = append(points, Point{X: x, Y: y})
	}

	return points
}

func (g *Generator) buildRandomWalkPoints(numPoints int, size float64) []Point {
	points := make([]Point, 0, numPoints)

	x, y := 0.0, 0.0
	points = append(points, Point{X: 0, Y: 0})
	step := size / 3

	for i := 1; i < numPoints; i++ {
		angle := g.rnd.Float64() * 2 * math.Pi
		x += step * math.Cos(angle)
		y += step * math.Sin(angle)
		points = append(points, Point{X: x, Y: y})
	}

	return points
}

// SegmentDistance calculates the distance from a point to the next point (or origin if last).
func SegmentDistance(points []Point, i int) float64 {
	if len(points) == 0 || i >= len(points) {
		return 0
	}

	pt := points[i]

	if i < len(points)-1 {
		next := points[i+1]
		dx := next.X - pt.X
		dy := next.Y - pt.Y
		return math.Sqrt(dx*dx + dy*dy)
	}

	return math.Sqrt(pt.X*pt.X + pt.Y*pt.Y)
}

// MovementDelay calculates a natural movement delay based on distance.
func (g *Generator) MovementDelay(distance float64) time.Duration {
	baseRange := MouseBaseDelayMaxSeconds - MouseBaseDelayMinSeconds
	baseDelay := MouseBaseDelayMinSeconds + g.rnd.Float64()*baseRange

	speedFactor := MouseSpeedFactorMin + g.rnd.Float64()*(MouseSpeedFactorMax-MouseSpeedFactorMin)
	if distance > MouseLongDistanceThreshold {
		speedFactor *= MouseSpeedFactorLongDist
	}

	delaySeconds := baseDelay * speedFactor
	return time.Duration(delaySeconds * float64(time.Second))
}

// ShouldPause determines if a pause should be inserted (simulates natural stopping).
func (g *Generator) ShouldPause() bool {
	return g.rnd.Float64() < MousePauseProbability
}

// PauseDelay returns a random pause duration.
func (g *Generator) PauseDelay() time.Duration {
	rangeSeconds := MousePauseDurationMax - MousePauseDurationMin
	delaySeconds := MousePauseDurationMin + g.rnd.Float64()*rangeSeconds
	return time.Duration(delaySeconds * float64(time.Second))
}

// ShouldAddIntermediate determines if an intermediate point should be added for smoother movement.
func (g *Generator) ShouldAddIntermediate(points []Point, i int, distance float64) bool {
	if i >= len(points)-1 {
		return false
	}
	if distance <= MouseIntermediateDistanceMin {
		return false
	}
	return g.rnd.Float64() < MouseIntermediateProbability
}

// IntermediatePoint calculates an intermediate point between two points with natural variation.
func (g *Generator) IntermediatePoint(points []Point, i int, baseDelay time.Duration) (Point, time.Duration) {
	pt := points[i]
	next := points[i+1]

	midX := pt.X + (next.X-pt.X)*MouseIntermediatePositionFactor + (g.rnd.Float64()-0.5)*MouseIntermediateJitter
	midY := pt.Y + (next.Y-pt.Y)*MouseIntermediatePositionFactor + (g.rnd.Float64()-0.5)*MouseIntermediateJitter

	speedVariation := MouseIntermediateSpeedMin + g.rnd.Float64()*(MouseIntermediateSpeedMax-MouseIntermediateSpeedMin)
	midDelay := time.Duration(float64(baseDelay) * speedVariation)

	return Point{X: midX, Y: midY}, midDelay
}

// ReturnDelay returns a random delay for returning to origin.
func (g *Generator) ReturnDelay() time.Duration {
	delaySeconds := MouseReturnDelayMin + g.rnd.Float64()*(MouseReturnDelayMax-MouseReturnDelayMin)
	return time.Duration(delaySeconds * float64(time.Second))
}

// IdleTracker tracks user activity state and provides rate-limited logging.
// It helps avoid log spam when repeatedly checking idle state.
type IdleTracker struct {
	lastActiveLogNS int64
}

// NewIdleTracker creates a new idle tracker.
func NewIdleTracker() *IdleTracker {
	return &IdleTracker{}
}

// IdleCheckResult represents the result of an idle check.
type IdleCheckResult struct {
	ShouldSimulate bool
	LogMessage     string // Non-empty if a log message should be emitted
}

// CheckIdle evaluates whether activity simulation should proceed based on idle time.
// Returns whether to simulate and an optional log message.
// The idle parameter should be the current system idle time, or a negative value if idle detection failed.
func (t *IdleTracker) CheckIdle(idle time.Duration, idleErr error, platform string) IdleCheckResult {
	nowNS := time.Now().UnixNano()

	// If idle detection failed, we should simulate but may want to log
	if idleErr != nil {
		return IdleCheckResult{
			ShouldSimulate: true,
			LogMessage:     "",
		}
	}

	// User is active - skip simulation
	if idle <= IdleThreshold {
		lastActiveLog := t.lastActiveLogNS
		if lastActiveLog == 0 || time.Duration(nowNS-lastActiveLog) > ActiveLogInterval {
			t.lastActiveLogNS = nowNS
			return IdleCheckResult{
				ShouldSimulate: false,
				LogMessage:     fmt.Sprintf("%s: user is active (idle: %v); skipping simulation to avoid interference", platform, idle),
			}
		}
		return IdleCheckResult{
			ShouldSimulate: false,
			LogMessage:     "",
		}
	}

	// User became idle - log transition if we were tracking active state
	if t.lastActiveLogNS != 0 {
		t.lastActiveLogNS = 0
		return IdleCheckResult{
			ShouldSimulate: true,
			LogMessage:     fmt.Sprintf("%s: user became idle (%v); resuming activity simulation", platform, idle),
		}
	}

	return IdleCheckResult{
		ShouldSimulate: true,
		LogMessage:     "",
	}
}

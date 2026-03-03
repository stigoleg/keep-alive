package platform

import (
	"math"
	"math/rand"
	"time"
)

// MousePoint represents a point in a mouse movement pattern
type MousePoint struct {
	X float64
	Y float64
}

// MousePatternGenerator generates natural mouse movement patterns
type MousePatternGenerator struct {
	rnd *rand.Rand
}

// NewMousePatternGenerator creates a new pattern generator with a random source
func NewMousePatternGenerator(rnd *rand.Rand) *MousePatternGenerator {
	return &MousePatternGenerator{rnd: rnd}
}

// GenerateRoundJitterPoints generates a small round random pattern around origin.
// Points are absolute offsets relative to origin (0,0).
func (g *MousePatternGenerator) GenerateRoundJitterPoints() []MousePoint {
	pointCount := MouseJitterPointsMin + g.rnd.Intn(MouseJitterPointsMax-MouseJitterPointsMin+1)
	radius := MouseJitterRadiusMin + g.rnd.Float64()*(MouseJitterRadiusMax-MouseJitterRadiusMin)

	direction := 1.0
	if g.rnd.Intn(2) == 0 {
		direction = -1.0
	}

	startAngle := g.rnd.Float64() * 2 * math.Pi
	points := make([]MousePoint, 0, pointCount)

	for i := 0; i < pointCount; i++ {
		angle := startAngle + direction*(2*math.Pi*float64(i)/float64(pointCount))
		localRadius := radius * (1 + (g.rnd.Float64()-0.5)*MouseJitterRadiusVariation)
		points = append(points, MousePoint{
			X: localRadius * math.Cos(angle),
			Y: localRadius * math.Sin(angle),
		})
	}

	return points
}

// JitterSessionDuration returns a random jitter session duration around 0.5s.
func (g *MousePatternGenerator) JitterSessionDuration() time.Duration {
	if MouseJitterSessionDurationMax <= MouseJitterSessionDurationMin {
		return MouseJitterSessionDurationMin
	}

	rangeDuration := MouseJitterSessionDurationMax - MouseJitterSessionDurationMin
	return MouseJitterSessionDurationMin + time.Duration(g.rnd.Int63n(int64(rangeDuration)+1))
}

func jitterStepDelay(total time.Duration, pointCount int) time.Duration {
	if total <= 0 {
		total = MouseJitterSessionDurationMin
	}

	steps := pointCount + 1 // include return-to-origin
	if steps <= 0 {
		steps = 1
	}

	step := total / time.Duration(steps)
	if step < time.Millisecond {
		return time.Millisecond
	}

	return step
}

// JitterStepDelayWithVariance applies ±MouseStepDelayVariance random jitter
// to a base step delay, producing more natural mouse movement timing.
func (g *MousePatternGenerator) JitterStepDelayWithVariance(base time.Duration) time.Duration {
	factor := 1.0 + (g.rnd.Float64()*2-1)*MouseStepDelayVariance
	d := time.Duration(float64(base) * factor)
	if d < time.Millisecond {
		return time.Millisecond
	}
	return d
}

func observedActiveTimestamp(nowNS int64, idle time.Duration) int64 {
	activeNS := nowNS - int64(idle)
	if activeNS < 0 {
		return 0
	}
	return activeNS
}

// relativeStepToPoint converts an absolute point to a relative step from current integer position.
func relativeStepToPoint(currentX, currentY int, pt MousePoint) (dx, dy, targetX, targetY int) {
	targetX = int(math.Round(pt.X))
	targetY = int(math.Round(pt.Y))
	return targetX - currentX, targetY - currentY, targetX, targetY
}

// SegmentDistance calculates the distance from a point to the next point (or origin if last)
func SegmentDistance(points []MousePoint, i int) float64 {
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

// MovementDelay calculates a natural movement delay based on distance
func (g *MousePatternGenerator) MovementDelay(distance float64) time.Duration {
	baseRange := MouseBaseDelayMaxSeconds - MouseBaseDelayMinSeconds
	baseDelay := MouseBaseDelayMinSeconds + g.rnd.Float64()*baseRange

	speedFactor := MouseSpeedFactorMin + g.rnd.Float64()*(MouseSpeedFactorMax-MouseSpeedFactorMin)
	if distance > MouseLongDistanceThreshold {
		speedFactor *= MouseSpeedFactorLongDist
	}

	delaySeconds := baseDelay * speedFactor
	return time.Duration(delaySeconds * float64(time.Second))
}

// ShouldPause determines if a pause should be inserted (simulates natural stopping)
func (g *MousePatternGenerator) ShouldPause() bool {
	return g.rnd.Float64() < MousePauseProbability
}

// PauseDelay returns a random pause duration
func (g *MousePatternGenerator) PauseDelay() time.Duration {
	rangeSeconds := MousePauseDurationMax - MousePauseDurationMin
	delaySeconds := MousePauseDurationMin + g.rnd.Float64()*rangeSeconds
	return time.Duration(delaySeconds * float64(time.Second))
}

// ShouldAddIntermediate determines if an intermediate point should be added for smoother movement
func (g *MousePatternGenerator) ShouldAddIntermediate(points []MousePoint, i int, distance float64) bool {
	if i >= len(points)-1 {
		return false
	}
	if distance <= MouseIntermediateDistanceMin {
		return false
	}
	return g.rnd.Float64() < MouseIntermediateProbability
}

// IntermediatePoint calculates an intermediate point between two points with natural variation
func (g *MousePatternGenerator) IntermediatePoint(points []MousePoint, i int, baseDelay time.Duration) (MousePoint, time.Duration) {
	pt := points[i]
	next := points[i+1]

	midX := pt.X + (next.X-pt.X)*MouseIntermediatePositionFactor + (g.rnd.Float64()-0.5)*MouseIntermediateJitter
	midY := pt.Y + (next.Y-pt.Y)*MouseIntermediatePositionFactor + (g.rnd.Float64()-0.5)*MouseIntermediateJitter

	speedVariation := MouseIntermediateSpeedMin + g.rnd.Float64()*(MouseIntermediateSpeedMax-MouseIntermediateSpeedMin)
	midDelay := time.Duration(float64(baseDelay) * speedVariation)

	return MousePoint{X: midX, Y: midY}, midDelay
}

// ReturnDelay returns a random delay for returning to origin
func (g *MousePatternGenerator) ReturnDelay() time.Duration {
	delaySeconds := MouseReturnDelayMin + g.rnd.Float64()*(MouseReturnDelayMax-MouseReturnDelayMin)
	return time.Duration(delaySeconds * float64(time.Second))
}

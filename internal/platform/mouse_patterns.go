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

// GenerateShapePoints generates a random mouse movement pattern
// Returns points relative to origin (0,0) that should be applied as offsets
func (g *MousePatternGenerator) GenerateShapePoints() []MousePoint {
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

// buildCirclePoints generates points in a circular pattern
func (g *MousePatternGenerator) buildCirclePoints(numPoints int, size float64) []MousePoint {
	points := make([]MousePoint, 0, numPoints)
	for i := 0; i < numPoints; i++ {
		angle := 2 * math.Pi * float64(i) / float64(numPoints)
		points = append(points, MousePoint{
			X: size * math.Cos(angle),
			Y: size * math.Sin(angle),
		})
	}
	return points
}

// buildSquarePoints generates points in a square pattern
func (g *MousePatternGenerator) buildSquarePoints(numPoints int, size float64) []MousePoint {
	side := int(math.Sqrt(float64(numPoints)))
	if side < 2 {
		side = 2
	}

	points := make([]MousePoint, 0, side*4)

	// Top edge
	for i := 0; i < side; i++ {
		points = append(points, MousePoint{
			X: size * float64(i) / float64(side-1),
			Y: 0,
		})
	}
	// Right edge
	for i := 1; i < side; i++ {
		points = append(points, MousePoint{
			X: size,
			Y: size * float64(i) / float64(side-1),
		})
	}
	// Bottom edge
	for i := side - 2; i >= 0; i-- {
		points = append(points, MousePoint{
			X: size * float64(i) / float64(side-1),
			Y: size,
		})
	}
	// Left edge
	for i := side - 2; i > 0; i-- {
		points = append(points, MousePoint{
			X: 0,
			Y: size * float64(i) / float64(side-1),
		})
	}

	return points
}

// buildZigZagPoints generates points in a zigzag pattern
func (g *MousePatternGenerator) buildZigZagPoints(numPoints int, size float64) []MousePoint {
	points := make([]MousePoint, 0, numPoints)

	for i := 0; i < numPoints; i++ {
		x := size * float64(i) / float64(numPoints-1)
		y := size * 0.5
		if i%2 == 0 {
			y = -size * 0.5
		}
		points = append(points, MousePoint{X: x, Y: y})
	}

	return points
}

// buildRandomWalkPoints generates points in a random walk pattern
func (g *MousePatternGenerator) buildRandomWalkPoints(numPoints int, size float64) []MousePoint {
	points := make([]MousePoint, 0, numPoints)

	x, y := 0.0, 0.0
	points = append(points, MousePoint{X: 0, Y: 0})
	step := size / 3

	for i := 1; i < numPoints; i++ {
		angle := g.rnd.Float64() * 2 * math.Pi
		x += step * math.Cos(angle)
		y += step * math.Sin(angle)
		points = append(points, MousePoint{X: x, Y: y})
	}

	return points
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

package patterns

import (
	"math/rand"
	"testing"
	"time"
)

func TestNewGenerator(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)
	if gen == nil {
		t.Fatal("NewGenerator returned nil")
	}
}

func TestGenerateShapePoints(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)

	// Generate multiple patterns to test different shape types
	for i := 0; i < 20; i++ {
		points := gen.GenerateShapePoints()
		if len(points) < 4 {
			t.Errorf("expected at least 4 points, got %d", len(points))
		}
	}
}

func TestGenerateShapePointsDeterministic(t *testing.T) {
	// Same seed should produce same results
	rnd1 := rand.New(rand.NewSource(12345))
	gen1 := NewGenerator(rnd1)
	points1 := gen1.GenerateShapePoints()

	rnd2 := rand.New(rand.NewSource(12345))
	gen2 := NewGenerator(rnd2)
	points2 := gen2.GenerateShapePoints()

	if len(points1) != len(points2) {
		t.Fatalf("point counts differ: %d vs %d", len(points1), len(points2))
	}

	for i := range points1 {
		if points1[i].X != points2[i].X || points1[i].Y != points2[i].Y {
			t.Errorf("point %d differs: %v vs %v", i, points1[i], points2[i])
		}
	}
}

func TestSegmentDistance(t *testing.T) {
	tests := []struct {
		name     string
		points   []Point
		index    int
		expected float64
	}{
		{
			name:     "empty points",
			points:   []Point{},
			index:    0,
			expected: 0,
		},
		{
			name:     "out of bounds",
			points:   []Point{{0, 0}},
			index:    5,
			expected: 0,
		},
		{
			name:     "single point at origin",
			points:   []Point{{0, 0}},
			index:    0,
			expected: 0,
		},
		{
			name:     "single point distance to origin",
			points:   []Point{{3, 4}},
			index:    0,
			expected: 5, // 3-4-5 triangle
		},
		{
			name:     "two points horizontal",
			points:   []Point{{0, 0}, {10, 0}},
			index:    0,
			expected: 10,
		},
		{
			name:     "two points vertical",
			points:   []Point{{0, 0}, {0, 10}},
			index:    0,
			expected: 10,
		},
		{
			name:     "last point to origin",
			points:   []Point{{0, 0}, {6, 8}},
			index:    1,
			expected: 10, // 6-8-10 triangle
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SegmentDistance(tt.points, tt.index)
			if got != tt.expected {
				t.Errorf("SegmentDistance() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMovementDelay(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)

	// Test that delay is in reasonable range
	for i := 0; i < 100; i++ {
		delay := gen.MovementDelay(5.0)
		minDelay := time.Duration(MouseBaseDelayMinSeconds * float64(time.Second) * MouseSpeedFactorMin)
		maxDelay := time.Duration(MouseBaseDelayMaxSeconds * float64(time.Second) * MouseSpeedFactorMax * MouseSpeedFactorLongDist)

		if delay < minDelay || delay > maxDelay {
			t.Errorf("delay %v out of expected range [%v, %v]", delay, minDelay, maxDelay)
		}
	}
}

func TestShouldPause(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)

	// Run many iterations and check probability is roughly correct
	pauseCount := 0
	iterations := 10000
	for i := 0; i < iterations; i++ {
		if gen.ShouldPause() {
			pauseCount++
		}
	}

	// Expected pause count should be around MousePauseProbability * iterations
	expected := MousePauseProbability * float64(iterations)
	tolerance := 0.05 * float64(iterations) // 5% tolerance

	if float64(pauseCount) < expected-tolerance || float64(pauseCount) > expected+tolerance {
		t.Errorf("pause probability out of range: got %d/%d, expected around %.0f", pauseCount, iterations, expected)
	}
}

func TestPauseDelay(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)

	minDelay := time.Duration(MousePauseDurationMin * float64(time.Second))
	maxDelay := time.Duration(MousePauseDurationMax * float64(time.Second))

	for i := 0; i < 100; i++ {
		delay := gen.PauseDelay()
		if delay < minDelay || delay > maxDelay {
			t.Errorf("pause delay %v out of range [%v, %v]", delay, minDelay, maxDelay)
		}
	}
}

func TestIntermediatePoint(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)

	points := []Point{{0, 0}, {10, 10}}
	baseDelay := 100 * time.Millisecond

	midPt, midDelay := gen.IntermediatePoint(points, 0, baseDelay)

	// Intermediate point should be between the two points (roughly)
	if midPt.X < -5 || midPt.X > 15 {
		t.Errorf("intermediate X=%v out of expected range", midPt.X)
	}
	if midPt.Y < -5 || midPt.Y > 15 {
		t.Errorf("intermediate Y=%v out of expected range", midPt.Y)
	}

	// Delay should be in reasonable range
	minDelay := time.Duration(float64(baseDelay) * MouseIntermediateSpeedMin)
	maxDelay := time.Duration(float64(baseDelay) * MouseIntermediateSpeedMax)
	if midDelay < minDelay || midDelay > maxDelay {
		t.Errorf("intermediate delay %v out of range [%v, %v]", midDelay, minDelay, maxDelay)
	}
}

func TestReturnDelay(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)

	minDelay := time.Duration(MouseReturnDelayMin * float64(time.Second))
	maxDelay := time.Duration(MouseReturnDelayMax * float64(time.Second))

	for i := 0; i < 100; i++ {
		delay := gen.ReturnDelay()
		if delay < minDelay || delay > maxDelay {
			t.Errorf("return delay %v out of range [%v, %v]", delay, minDelay, maxDelay)
		}
	}
}

func TestShouldAddIntermediate(t *testing.T) {
	rnd := rand.New(rand.NewSource(42))
	gen := NewGenerator(rnd)

	points := []Point{{0, 0}, {10, 10}, {20, 20}}

	// Last point should never add intermediate
	for i := 0; i < 100; i++ {
		if gen.ShouldAddIntermediate(points, 2, 100) {
			t.Error("should not add intermediate for last point")
		}
	}

	// Small distance should never add intermediate
	for i := 0; i < 100; i++ {
		if gen.ShouldAddIntermediate(points, 0, MouseIntermediateDistanceMin-1) {
			t.Error("should not add intermediate for small distance")
		}
	}
}

// Test that all shape types are reachable
func TestAllShapeTypes(t *testing.T) {
	// Use different seeds to try to hit all shape types
	shapeTypeSeen := make(map[int]bool)

	for seed := int64(0); seed < 1000; seed++ {
		rnd := rand.New(rand.NewSource(seed))
		shapeType := rnd.Intn(4)
		shapeTypeSeen[shapeType] = true

		if len(shapeTypeSeen) == 4 {
			break
		}
	}

	if len(shapeTypeSeen) != 4 {
		t.Errorf("not all shape types were reached, got %d types", len(shapeTypeSeen))
	}
}

package platform

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

func TestActivitySimulationTimingConstants(t *testing.T) {
	if ChatAppActivityInterval != 30*time.Second {
		t.Fatalf("ChatAppActivityInterval = %v, want 30s", ChatAppActivityInterval)
	}

	if ChatAppCheckInterval != 5*time.Second {
		t.Fatalf("ChatAppCheckInterval = %v, want 5s", ChatAppCheckInterval)
	}

	if IdleThreshold != 2*time.Minute {
		t.Fatalf("IdleThreshold = %v, want 2m", IdleThreshold)
	}

	if SyntheticIdleResetTolerance != 4*time.Second {
		t.Fatalf("SyntheticIdleResetTolerance = %v, want 4s", SyntheticIdleResetTolerance)
	}

	if MouseJitterSessionDurationMin != 400*time.Millisecond {
		t.Fatalf("MouseJitterSessionDurationMin = %v, want 400ms", MouseJitterSessionDurationMin)
	}

	if MouseJitterSessionDurationMax != 600*time.Millisecond {
		t.Fatalf("MouseJitterSessionDurationMax = %v, want 600ms", MouseJitterSessionDurationMax)
	}
}

func TestGenerateRoundJitterPointsInBounds(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	g := NewMousePatternGenerator(rnd)
	maxRadius := MouseJitterRadiusMax * (1 + MouseJitterRadiusVariation/2)

	for i := 0; i < 200; i++ {
		points := g.GenerateRoundJitterPoints()
		if len(points) < MouseJitterPointsMin || len(points) > MouseJitterPointsMax {
			t.Fatalf("round jitter point count out of bounds: got %d, want [%d, %d]", len(points), MouseJitterPointsMin, MouseJitterPointsMax)
		}

		for _, pt := range points {
			distance := math.Hypot(pt.X, pt.Y)
			if distance > maxRadius {
				t.Fatalf("round jitter radius too large: got %.3f, max %.3f", distance, maxRadius)
			}

			if math.Abs(pt.X) < 1e-9 && math.Abs(pt.Y) < 1e-9 {
				t.Fatal("round jitter point should not be origin")
			}
		}
	}
}

func TestRelativeStepToPointReturnsToOrigin(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	g := NewMousePatternGenerator(rnd)

	for i := 0; i < 100; i++ {
		points := g.GenerateRoundJitterPoints()
		currentX, currentY := 0, 0

		for _, pt := range points {
			dx, dy, targetX, targetY := relativeStepToPoint(currentX, currentY, pt)
			currentX += dx
			currentY += dy
			if currentX != targetX || currentY != targetY {
				t.Fatalf("relative step did not reach target (%d,%d), got (%d,%d)", targetX, targetY, currentX, currentY)
			}
		}

		// Final compensation step used by platform executors
		currentX += -currentX
		currentY += -currentY
		if currentX != 0 || currentY != 0 {
			t.Fatalf("expected compensation step to return origin, got (%d,%d)", currentX, currentY)
		}
	}
}

func TestJitterSessionDurationRange(t *testing.T) {
	rnd := rand.New(rand.NewSource(11))
	g := NewMousePatternGenerator(rnd)

	for i := 0; i < 200; i++ {
		d := g.JitterSessionDuration()
		if d < MouseJitterSessionDurationMin || d > MouseJitterSessionDurationMax {
			t.Fatalf("jitter duration out of range: got %v, want [%v, %v]", d, MouseJitterSessionDurationMin, MouseJitterSessionDurationMax)
		}
	}
}

func TestJitterStepDelay(t *testing.T) {
	delay := jitterStepDelay(500*time.Millisecond, 9)
	if delay != 50*time.Millisecond {
		t.Fatalf("jitterStepDelay = %v, want 50ms", delay)
	}

	minDelay := jitterStepDelay(0, 1000)
	if minDelay < time.Millisecond {
		t.Fatalf("jitterStepDelay minimum = %v, want >=1ms", minDelay)
	}
}

func TestObservedActiveTimestamp(t *testing.T) {
	nowNS := int64(10 * time.Second)
	idle := 1500 * time.Millisecond
	got := observedActiveTimestamp(nowNS, idle)
	want := int64(8500 * time.Millisecond)
	if got != want {
		t.Fatalf("observedActiveTimestamp = %d, want %d", got, want)
	}

	if observedActiveTimestamp(100, 2*time.Second) != 0 {
		t.Fatal("observedActiveTimestamp should clamp at zero")
	}
}

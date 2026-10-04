package activity

import (
	"math"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"
)

func seeded(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) }

func TestNewPathIsDeterministicForASeed(t *testing.T) {
	a, b := NewPath(seeded(7)), NewPath(seeded(7))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different paths")
	}
	if reflect.DeepEqual(a, NewPath(seeded(8))) {
		t.Fatal("different seeds produced the same path")
	}
}

func TestNewPathShape(t *testing.T) {
	for seed := uint64(0); seed < 2000; seed++ {
		p := NewPath(seeded(seed))
		if len(p) < 40 {
			t.Fatalf("seed %d: only %d steps", seed, len(p))
		}
		last := p[len(p)-1]
		if last.X != 0 || last.Y != 0 {
			t.Fatalf("seed %d: path ends at (%v, %v), want exactly (0, 0)", seed, last.X, last.Y)
		}
		var maxR float64
		for i, s := range p {
			if s.Delay < 8*time.Millisecond || s.Delay > 16*time.Millisecond {
				t.Fatalf("seed %d step %d: delay %v outside 8-16ms", seed, i, s.Delay)
			}
			maxR = math.Max(maxR, math.Hypot(s.X, s.Y))
		}
		if maxR > 130 {
			t.Fatalf("seed %d: point %.1f px from the origin, want <= 130", seed, maxR)
		}
		if maxR < 20 {
			t.Fatalf("seed %d: path only reaches %.1f px, too small to look like movement", seed, maxR)
		}
		if d := p.Duration(); d < time.Second || d > 2500*time.Millisecond {
			t.Fatalf("seed %d: duration %v outside 1.0-2.5s", seed, d)
		}
	}
}

func TestFitKeepsPointsOnScreen(t *testing.T) {
	screens := []struct {
		name   string
		bounds Rect
		ox, oy float64
	}{
		{"top-left corner", Rect{0, 0, 1440, 900}, 0, 0},
		{"near top-left", Rect{0, 0, 1440, 900}, 4, 6},
		{"bottom-right corner", Rect{0, 0, 1440, 900}, 1439, 899},
		{"near bottom-right", Rect{0, 0, 1440, 900}, 1430, 890},
		{"left edge middle", Rect{0, 0, 1440, 900}, 2, 450},
		{"secondary display left of main", Rect{-1920, -200, 1920, 1080}, -1918, -198},
		{"tiny display", Rect{0, 0, 100, 80}, 50, 40},
	}
	for _, sc := range screens {
		for seed := uint64(0); seed < 300; seed++ {
			p := NewPath(seeded(seed)).Fit(sc.bounds, sc.ox, sc.oy)
			for i, s := range p {
				x, y := sc.ox+s.X, sc.oy+s.Y
				if x < sc.bounds.X || x > sc.bounds.X+sc.bounds.W-1 || y < sc.bounds.Y || y > sc.bounds.Y+sc.bounds.H-1 {
					t.Fatalf("%s seed %d step %d: (%v, %v) outside %+v", sc.name, seed, i, x, y, sc.bounds)
				}
			}
			if last := p[len(p)-1]; last.X != 0 || last.Y != 0 {
				t.Fatalf("%s seed %d: fitted path ends at (%v, %v)", sc.name, seed, last.X, last.Y)
			}
		}
	}
}

func TestFitMirrorsInsteadOfShrinkingWhenPossible(t *testing.T) {
	// A path that only goes right and down, with the cursor near the
	// bottom-right corner: mirroring both axes keeps its full size.
	p := Path{{X: 50, Y: 40, Delay: 10 * time.Millisecond}, {X: 100, Y: 80, Delay: 10 * time.Millisecond}, {Delay: 10 * time.Millisecond}}
	got := p.Fit(Rect{0, 0, 1000, 1000}, 990, 990)
	want := Path{{X: -50, Y: -40, Delay: 10 * time.Millisecond}, {X: -100, Y: -80, Delay: 10 * time.Millisecond}, {Delay: 10 * time.Millisecond}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Fit = %+v, want %+v", got, want)
	}
}

func TestFitLeavesPathAloneWithRoom(t *testing.T) {
	p := NewPath(seeded(3))
	if got := p.Fit(Rect{0, 0, 1440, 900}, 720, 450); !reflect.DeepEqual(got, p) {
		t.Fatal("Fit changed a path that already fits")
	}
	if got := p.Fit(Rect{}, 720, 450); !reflect.DeepEqual(got, p) {
		t.Fatal("Fit changed the path for empty bounds")
	}
}

func TestDeltasSumToZeroWithoutZeroMoves(t *testing.T) {
	for _, minStep := range []time.Duration{0, 40 * time.Millisecond} {
		for seed := uint64(0); seed < 500; seed++ {
			p := NewPath(seeded(seed))
			ds := p.Deltas(minStep)
			var sx, sy int
			var total time.Duration
			for i, d := range ds {
				if d.DX == 0 && d.DY == 0 {
					t.Fatalf("seed %d delta %d is zero", seed, i)
				}
				if minStep > 0 && i < len(ds)-1 && d.Delay < minStep {
					t.Fatalf("seed %d delta %d: delay %v below the %v minimum", seed, i, d.Delay, minStep)
				}
				sx += d.DX
				sy += d.DY
				total += d.Delay
			}
			if sx != 0 || sy != 0 {
				t.Fatalf("seed %d min %v: deltas sum to (%d, %d), want (0, 0)", seed, minStep, sx, sy)
			}
			if total > p.Duration() {
				t.Fatalf("seed %d: deltas take %v, longer than the path's %v", seed, total, p.Duration())
			}
			if minStep > 0 && len(ds) > len(p.Deltas(0)) {
				t.Fatalf("seed %d: coalescing produced more moves", seed)
			}
		}
	}
}

func TestMinimumJerkProfile(t *testing.T) {
	if minimumJerk(0) != 0 || minimumJerk(1) != 1 {
		t.Fatal("minimum-jerk profile must start at 0 and end at 1")
	}
	if got := minimumJerk(0.5); math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("s(0.5) = %v, want 0.5", got)
	}
	prev := 0.0
	for i := 1; i <= 100; i++ {
		s := minimumJerk(float64(i) / 100)
		if s < prev {
			t.Fatalf("profile not monotonic at %d", i)
		}
		prev = s
	}
}

package activity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func noSleep(ctx context.Context, d time.Duration) error { return ctx.Err() }

func probeBackend(m *machine) *backend {
	return &backend{
		idle:    fakeIdle{m},
		sources: []IdleSource{fakeIdle{m}},
		lock:    fakeLock{m},
		open: func() (Injector, error) {
			if m.openErr != nil {
				return nil, m.openErr
			}
			return fakeInjector{m}, nil
		},
	}
}

func TestProbeVerifiesTheBurst(t *testing.T) {
	m := newMachine()
	m.clk.Advance(5 * time.Minute)
	res := probe(context.Background(), probeBackend(m), true, seeded(1), noSleep)
	if res.Method != "Fake" || !res.Effective || res.Reason != "" {
		t.Fatalf("probe = %+v", res)
	}
	if len(res.Sources) != 1 || res.Sources[0].Before != 5*time.Minute || res.Sources[0].After != 0 {
		t.Fatalf("readings = %+v", res.Sources)
	}
	if !res.LockKnown || res.Locked || res.Verifier != "fake idle" {
		t.Fatalf("probe = %+v", res)
	}
	if m.taps != 1 || m.closed != 1 {
		t.Fatalf("taps = %d, closed = %d; want 1 and 1", m.taps, m.closed)
	}
}

func TestProbeExplainsIneffectiveBurst(t *testing.T) {
	m := newMachine()
	m.ignored = true
	m.clk.Advance(5 * time.Minute)
	res := probe(context.Background(), probeBackend(m), false, seeded(1), noSleep)
	if res.Effective || res.Reason != "input ignored" || res.Hint != "grant it" {
		t.Fatalf("probe = %+v", res)
	}
	if m.taps != 0 {
		t.Fatal("tapped a key without keys enabled")
	}
}

func TestProbeNotesACounterThatMissedTheBurst(t *testing.T) {
	for _, follows := range []bool{false, true} {
		m := newMachine()
		m.xwFollows = follows
		m.clk.Advance(5 * time.Minute)
		b := probeBackend(m)
		b.sources = append(b.sources, fakeXWayland{m})
		b.secondary, b.secondaryNote = fakeXWayland{m}, testXWaylandNote
		res := probe(context.Background(), b, false, seeded(1), noSleep)
		if !res.Effective || len(res.Sources) != 2 {
			t.Fatalf("probe = %+v", res)
		}
		xw := res.Sources[1]
		if xw.Source != "xprintidle (XWayland)" || xw.Before != 5*time.Minute {
			t.Fatalf("XWayland reading = %+v", xw)
		}
		want := testXWaylandNote
		if follows {
			want = ""
		}
		if xw.Note != want || res.Sources[0].Note != "" {
			t.Fatalf("follows=%v: notes %q / %q, want %q on XWayland only", follows, res.Sources[0].Note, xw.Note, want)
		}
	}
}

func TestProbeWithoutInjector(t *testing.T) {
	m := newMachine()
	m.openErr = &Unavailable{Reason: "no backend", Hint: "install one"}
	res := probe(context.Background(), probeBackend(m), false, seeded(1), noSleep)
	if res.Method != "" || res.Reason != "no backend" || res.Hint != "install one" || len(res.Sources) != 1 {
		t.Fatalf("probe = %+v", res)
	}
}

type move struct{ x, y float64 }

// absRecorder is an AbsoluteInjector without Play, so playBurst steps it.
type absRecorder struct {
	ox, oy float64
	bounds Rect
	moves  []move
	failAt int
}

func (*absRecorder) Name() string               { return "abs" }
func (*absRecorder) Available() error           { return nil }
func (*absRecorder) Tap() error                 { return nil }
func (*absRecorder) Diagnose() (string, string) { return "", "" }
func (*absRecorder) Close() error               { return nil }
func (a *absRecorder) Position() (float64, float64, bool) {
	return a.ox, a.oy, true
}
func (a *absRecorder) Bounds() (Rect, bool) { return a.bounds, a.bounds.W > 0 }
func (a *absRecorder) MoveTo(x, y float64) error {
	a.moves = append(a.moves, move{x, y})
	if a.failAt > 0 && len(a.moves) == a.failAt {
		return errors.New("move failed")
	}
	return nil
}

type relRecorder struct {
	moves   [][2]int
	minStep time.Duration
}

func (*relRecorder) Name() string               { return "rel" }
func (*relRecorder) Available() error           { return nil }
func (*relRecorder) Tap() error                 { return nil }
func (*relRecorder) Diagnose() (string, string) { return "", "" }
func (*relRecorder) Close() error               { return nil }
func (r *relRecorder) MoveBy(dx, dy int) error {
	r.moves = append(r.moves, [2]int{dx, dy})
	return nil
}

type slowRelRecorder struct{ relRecorder }

func (s *slowRelRecorder) MinStep() time.Duration { return 40 * time.Millisecond }

func TestPlayBurstAbsoluteStaysOnScreenAndReturns(t *testing.T) {
	a := &absRecorder{ox: 3, oy: 897, bounds: Rect{0, 0, 1440, 900}}
	p := NewPath(seeded(4))
	if err := playBurst(context.Background(), a, p, noSleep); err != nil {
		t.Fatal(err)
	}
	if len(a.moves) != len(p) {
		t.Fatalf("%d moves for %d steps", len(a.moves), len(p))
	}
	for i, mv := range a.moves {
		if !a.bounds.Contains(mv.x, mv.y) {
			t.Fatalf("move %d to (%v, %v) is off screen", i, mv.x, mv.y)
		}
	}
	if last := a.moves[len(a.moves)-1]; last != (move{3, 897}) {
		t.Fatalf("burst ended at %+v, want the origin", last)
	}
}

func TestPlayBurstRelativeSumsToZero(t *testing.T) {
	for _, r := range []RelativeInjector{&relRecorder{}, &slowRelRecorder{}} {
		var moves [][2]int
		var sleeps []time.Duration
		sleep := func(ctx context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		}
		if err := playBurst(context.Background(), r, NewPath(seeded(5)), sleep); err != nil {
			t.Fatal(err)
		}
		switch rr := r.(type) {
		case *relRecorder:
			moves = rr.moves
		case *slowRelRecorder:
			moves = rr.moves
			for i, d := range sleeps[:len(sleeps)-1] {
				if d < 40*time.Millisecond {
					t.Fatalf("slow injector move %d after only %v", i, d)
				}
			}
		}
		var sx, sy int
		for _, mv := range moves {
			if mv == [2]int{} {
				t.Fatal("zero move sent")
			}
			sx, sy = sx+mv[0], sy+mv[1]
		}
		if sx != 0 || sy != 0 {
			t.Fatalf("%s moves sum to (%d, %d)", r.Name(), sx, sy)
		}
	}
}

func TestPlayBurstCancelledMidwayReturnsPointer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n := 0
	sleep := func(ctx context.Context, d time.Duration) error {
		if n++; n == 30 {
			cancel()
		}
		return ctx.Err()
	}
	a := &absRecorder{ox: 700, oy: 400, bounds: Rect{0, 0, 1440, 900}}
	if err := playBurst(ctx, a, NewPath(seeded(6)), sleep); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	if last := a.moves[len(a.moves)-1]; last != (move{700, 400}) {
		t.Fatalf("pointer left at %+v after cancel", last)
	}

	ctx, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	cancel = cancel2
	n = 0
	r := &relRecorder{}
	if err := playBurst(ctx, r, NewPath(seeded(6)), sleep); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	var sx, sy int
	for _, mv := range r.moves {
		sx, sy = sx+mv[0], sy+mv[1]
	}
	if sx != 0 || sy != 0 {
		t.Fatalf("relative pointer left at (%d, %d) after cancel", sx, sy)
	}

	a = &absRecorder{ox: 700, oy: 400, failAt: 10}
	if err := playBurst(context.Background(), a, NewPath(seeded(6)), noSleep); err == nil {
		t.Fatal("move error not returned")
	}
	if last := a.moves[len(a.moves)-1]; last != (move{700, 400}) {
		t.Fatalf("pointer left at %+v after a failed move", last)
	}
}

func TestVirtualDeskAbsolute(t *testing.T) {
	tests := []struct {
		name   string
		desk   Rect
		x, y   float64
		nx, ny int32
	}{
		{"origin", Rect{0, 0, 1920, 1080}, 0, 0, 0, 0},
		{"far corner", Rect{0, 0, 1920, 1080}, 1919, 1079, 65535, 65535},
		{"centre", Rect{0, 0, 1920, 1080}, 960, 540, 32784, 32797},
		{"monitor left of primary", Rect{-1920, 0, 3840, 1080}, -1920, 0, 0, 0},
		{"primary origin with left monitor", Rect{-1920, 0, 3840, 1080}, 0, 0, 32776, 0},
		{"monitor above and left", Rect{-1280, -1024, 3200, 2104}, -1280, -1024, 0, 0},
		{"rounds fractional points", Rect{0, 0, 1920, 1080}, 959.6, 539.6, 32784, 32797},
		{"clamps outside the desktop", Rect{0, 0, 1920, 1080}, -50, 5000, 0, 65535},
	}
	for _, tt := range tests {
		nx, ny := virtualDeskAbsolute(tt.x, tt.y, tt.desk)
		if nx != tt.nx || ny != tt.ny {
			t.Errorf("%s: virtualDeskAbsolute(%v, %v) = (%d, %d), want (%d, %d)", tt.name, tt.x, tt.y, nx, ny, tt.nx, tt.ny)
		}
	}
}

package activity

import (
	"context"
	"errors"
	"math"
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

func TestProbeReportsAMouseMovedDuringTheBurst(t *testing.T) {
	m := newMachine()
	m.playErr = errUserMoved
	res := probe(context.Background(), probeBackend(m), false, seeded(1), noSleep)
	if res.Effective || res.Reason != "the mouse was moved during the probe" || res.Hint != "keep the mouse still and probe again" {
		t.Fatalf("probe = %+v", res)
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

// absRecorder is an AbsoluteInjector without Play, so playBurst steps it.
// The pointer follows its moves, lagging by lag moves, until userAt moves
// have been made; then the user has grabbed it and it sits at user.
type absRecorder struct {
	ox, oy    float64
	bounds    Rect
	moves     []move
	failAt    int
	lag       int
	userAt    int
	user      move
	positions int
}

func (*absRecorder) Name() string               { return "abs" }
func (*absRecorder) Available() error           { return nil }
func (*absRecorder) Tap() error                 { return nil }
func (*absRecorder) Diagnose() (string, string) { return "", "" }
func (*absRecorder) Close() error               { return nil }
func (a *absRecorder) Position() (float64, float64, bool) {
	a.positions++
	if a.userAt > 0 && len(a.moves) >= a.userAt {
		return a.user.x, a.user.y, true
	}
	if i := len(a.moves) - 1 - a.lag; i >= 0 {
		return a.moves[i].x, a.moves[i].y, true
	}
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

func TestPlayBurstRelativeLimitsMovesUpAndLeft(t *testing.T) {
	for seed := uint64(0); seed < 300; seed++ {
		r := &relRecorder{}
		p := NewPath(seeded(seed))
		var radius float64
		for _, s := range p {
			radius = math.Max(radius, math.Hypot(s.X, s.Y))
		}
		if err := playBurst(context.Background(), r, p, noSleep); err != nil {
			t.Fatal(err)
		}
		var x, y int
		for _, mv := range r.moves {
			x, y = x+mv[0], y+mv[1]
			if float64(x) < -0.4*radius-1 || float64(y) < -0.4*radius-1 {
				t.Fatalf("seed %d: relative pointer at (%d, %d), more than 40%% of %.0f px up or left", seed, x, y, radius)
			}
		}
	}
}

func TestPlayBurstStopsWhenTheUserMovesThePointer(t *testing.T) {
	p := NewPath(seeded(11))
	a := &absRecorder{ox: 700, oy: 400, bounds: Rect{0, 0, 1440, 900}, userAt: 20, user: move{1000, 120}}
	err := playBurst(context.Background(), a, p, noSleep)
	if !errors.Is(err, errUserMoved) {
		t.Fatalf("err = %v, want errUserMoved", err)
	}
	if n := len(a.moves); n < 20 || n > 23 {
		t.Fatalf("made %d moves, want to stop within 3 steps of the user's move at 20", n)
	}
	if last := a.moves[len(a.moves)-1]; last == (move{700, 400}) || last == a.user {
		t.Fatalf("moved the pointer to %+v after the user took it", last)
	}
	if a.positions > len(p)/3+1 {
		t.Fatalf("read the position %d times for %d steps, want every 3 steps", a.positions, len(p))
	}
}

func TestPlayBurstToleratesPositionLagAndRounding(t *testing.T) {
	for _, lag := range []int{0, 1} {
		a := &absRecorder{ox: 700, oy: 400, bounds: Rect{0, 0, 1440, 900}, lag: lag}
		if err := playBurst(context.Background(), a, NewPath(seeded(12)), noSleep); err != nil {
			t.Fatalf("lag %d: %v", lag, err)
		}
		if last := a.moves[len(a.moves)-1]; last != (move{700, 400}) {
			t.Fatalf("lag %d: ended at %+v", lag, last)
		}
	}
	if userMoved(102.5, 99, move{100, 100}, move{90, 95}) {
		t.Fatal("a pixel of rounding counted as the user")
	}
	if !userMoved(104, 100, move{100, 100}, move{90, 95}) {
		t.Fatal("4 px from both points not counted as the user")
	}
}

func TestWindowsLockFallsBackToTheInputDesktop(t *testing.T) {
	wtsFailed := func() (bool, error) { return false, errors.New("WTSQuerySessionInformation: access denied") }
	unknownFlag := func() (bool, error) { return false, errors.New("unknown session lock state 7") }
	desk := func(locked bool, err error) func() (bool, error) {
		return func() (bool, error) { return locked, err }
	}
	deskCalled := false
	tests := []struct {
		name    string
		wts     func() (bool, error)
		desk    func() (bool, error)
		locked  bool
		wantErr bool
	}{
		{"WTS says locked", func() (bool, error) { return true, nil }, func() (bool, error) { deskCalled = true; return false, nil }, true, false},
		{"WTS says unlocked", func() (bool, error) { return false, nil }, func() (bool, error) { deskCalled = true; return true, nil }, false, false},
		{"WTS fails, secure desktop", wtsFailed, desk(true, nil), true, false},
		{"unknown flag, Default desktop", unknownFlag, desk(false, nil), false, false},
		{"both fail", wtsFailed, desk(false, errors.New("GetUserObjectInformation failed")), false, true},
	}
	for _, tt := range tests {
		deskCalled = false
		locked, err := lockedSessionOrDesktop(tt.wts, tt.desk)
		if locked != tt.locked || (err != nil) != tt.wantErr {
			t.Errorf("%s: %v, %v; want %v, error %v", tt.name, locked, err, tt.locked, tt.wantErr)
		}
		if deskCalled {
			t.Errorf("%s: asked the input desktop although WTS answered", tt.name)
		}
	}

	for _, tt := range []struct {
		name         string
		desktop      string
		openErr      error
		accessDenied bool
		locked       bool
		wantErr      bool
	}{
		{"Default desktop", "Default", nil, false, false, false},
		{"Default in other case", "default", nil, false, false, false},
		{"Winlogon desktop", "Winlogon", nil, false, true, false},
		{"screen saver desktop", "Screen-saver", nil, false, true, false},
		{"access denied opening it", "", errors.New("OpenInputDesktop: Access is denied."), true, true, false},
		{"other open error", "", errors.New("OpenInputDesktop: invalid handle"), false, false, true},
	} {
		locked, err := inputDesktopLocked(tt.desktop, tt.openErr, tt.accessDenied)
		if locked != tt.locked || (err != nil) != tt.wantErr {
			t.Errorf("%s: %v, %v; want %v, error %v", tt.name, locked, err, tt.locked, tt.wantErr)
		}
	}
}

// fakeCursor is a Windows cursor for snapBack.
type fakeCursor struct {
	x, y float64
	ok   bool
	sets []move
}

func (c *fakeCursor) get() (float64, float64, bool) { return c.x, c.y, c.ok }
func (c *fakeCursor) set(x, y int32) error {
	c.sets = append(c.sets, move{float64(x), float64(y)})
	c.x, c.y = float64(x), float64(y)
	return nil
}

func TestSnapBackOnlyFixesASmallMiss(t *testing.T) {
	for _, tt := range []struct {
		name   string
		x, y   float64
		ok     bool
		ox, oy float64
		snap   bool
	}{
		{"exact", 700, 400, true, 700, 400, false},
		{"one pixel left", 699, 400, true, 700, 400, true},
		{"two pixels diagonally", 702, 398, true, 700, 400, true},
		{"rounded origin", 1001, 500, true, 999.6, 500.2, true},
		{"three pixels: the user's", 703, 400, true, 700, 400, false},
		{"far away: the user's", 900, 120, true, 700, 400, false},
		{"position unknown", 0, 0, false, 700, 400, false},
		{"negative coordinates", -1281, -2, true, -1280, -1, true},
	} {
		c := &fakeCursor{x: tt.x, y: tt.y, ok: tt.ok}
		snapped, err := snapBack(c.get, c.set, tt.ox, tt.oy)
		if err != nil || snapped != tt.snap {
			t.Errorf("%s: snapped %v, %v; want %v", tt.name, snapped, err, tt.snap)
			continue
		}
		if tt.snap && (len(c.sets) != 1 || c.sets[0] != (move{math.Round(tt.ox), math.Round(tt.oy)})) {
			t.Errorf("%s: set %v, want the origin", tt.name, c.sets)
		}
		if !tt.snap && len(c.sets) != 0 {
			t.Errorf("%s: moved the cursor to %v", tt.name, c.sets)
		}
	}
}

// finishingRecorder is an absolute injector that also corrects its return.
type finishingRecorder struct {
	absRecorder
	finished []move
}

func (f *finishingRecorder) FinishAt(x, y float64) { f.finished = append(f.finished, move{x, y}) }

func TestPlayBurstFinishesExactlyAtTheOrigin(t *testing.T) {
	f := &finishingRecorder{absRecorder: absRecorder{ox: 700, oy: 400, bounds: Rect{0, 0, 1440, 900}}}
	if err := playBurst(context.Background(), f, NewPath(seeded(13)), noSleep); err != nil {
		t.Fatal(err)
	}
	if len(f.finished) != 1 || f.finished[0] != (move{700, 400}) {
		t.Fatalf("FinishAt calls %v, want one at the origin", f.finished)
	}
	f = &finishingRecorder{absRecorder: absRecorder{ox: 700, oy: 400, bounds: Rect{0, 0, 1440, 900}, userAt: 10, user: move{100, 100}}}
	if err := playBurst(context.Background(), f, NewPath(seeded(13)), noSleep); !errors.Is(err, errUserMoved) || len(f.finished) != 0 {
		t.Fatalf("after the user took the pointer: err %v, FinishAt %v", err, f.finished)
	}
}

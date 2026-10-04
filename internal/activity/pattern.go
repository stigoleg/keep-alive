package activity

import (
	"math"
	"math/rand/v2"
	"time"
)

// Burst shape. Strokes aim at 30-120 px from the origin; after curvature and
// overshoot the whole path is scaled to stay within maxPathRadius, which
// leaves room for the ±1 px tremor under 130 px.
const (
	minStrokeRadius = 30.0
	maxStrokeRadius = 120.0
	minStrokeLength = 25.0
	maxPathRadius   = 128.0
	curvature       = 0.35
	overshootChance = 0.4
	minOvershoot    = 0.05
	maxOvershoot    = 0.12

	// Hot corners (GNOME Activities, Plasma Overview, macOS) fire when the
	// pointer reaches a display corner, so bursts keep cornerSize px away
	// from each corner and edgeInset px off every edge.
	edgeInset  = 3.0
	cornerSize = 24.0
	// unknownReach bounds moves up and left, as a share of the burst's
	// radius, when the pointer position is unknown.
	unknownReach = 0.4

	minStepDelay  = 8 * time.Millisecond
	maxStepDelay  = 16 * time.Millisecond
	meanStepDelay = 12 * time.Millisecond
	minBurstTime  = 1100 * time.Millisecond
	maxBurstTime  = 2400 * time.Millisecond
)

// Step is one pointer position, as an offset from where the burst started,
// reached Delay after the previous step.
type Step struct {
	X, Y  float64
	Delay time.Duration
}

// Path is one burst of pointer movement. It always ends exactly at (0, 0).
type Path []Step

// Delta is a relative move for injectors that cannot warp the pointer.
type Delta struct {
	DX, DY int
	Delay  time.Duration
}

type point struct{ x, y float64 }

type stroke struct {
	p0, p1, p2, p3 point
	weight         float64
}

// NewPath builds a human-looking burst: one to three curved strokes away
// from the origin, sometimes overshooting the last target, then a stroke
// back to exactly where it started. Each stroke follows a cubic Bézier curve
// timed with the minimum-jerk velocity profile.
func NewPath(r *rand.Rand) Path {
	var strokes []stroke
	cur := point{}
	n := 1 + r.IntN(3)
	for i := 0; i < n; i++ {
		target := randomTarget(r, cur)
		if i == n-1 && r.Float64() < overshootChance {
			o := minOvershoot + r.Float64()*(maxOvershoot-minOvershoot)
			over := point{target.x + (target.x-cur.x)*o, target.y + (target.y-cur.y)*o}
			strokes = append(strokes, curve(r, cur, over), curve(r, over, target))
		} else {
			strokes = append(strokes, curve(r, cur, target))
		}
		cur = target
	}
	strokes = append(strokes, curve(r, cur, point{}))

	total := minBurstTime + time.Duration(r.Float64()*float64(maxBurstTime-minBurstTime))
	var weights float64
	for _, s := range strokes {
		weights += s.weight
	}
	var p Path
	for _, s := range strokes {
		p = append(p, sample(r, s, time.Duration(float64(total)*s.weight/weights))...)
	}

	var maxR float64
	for _, s := range p {
		maxR = math.Max(maxR, math.Hypot(s.X, s.Y))
	}
	scale := 1.0
	if maxR > maxPathRadius {
		scale = maxPathRadius / maxR
	}
	for i := range p {
		p[i].X *= scale
		p[i].Y *= scale
		if i < len(p)-1 {
			p[i].X += r.Float64()*2 - 1
			p[i].Y += r.Float64()*2 - 1
		}
	}
	return p
}

// randomTarget picks a point 30-120 px from the origin that is far enough
// from cur to be a visible stroke.
func randomTarget(r *rand.Rand, cur point) point {
	var t point
	for range 8 {
		radius := minStrokeRadius + r.Float64()*(maxStrokeRadius-minStrokeRadius)
		angle := r.Float64() * 2 * math.Pi
		t = point{radius * math.Cos(angle), radius * math.Sin(angle)}
		if math.Hypot(t.x-cur.x, t.y-cur.y) >= minStrokeLength {
			break
		}
	}
	return t
}

// curve bends the chord from a to b with control points pushed sideways by
// up to ±35 % of its length. Longer strokes take longer (Fitts-like).
func curve(r *rand.Rand, a, b point) stroke {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	s := stroke{p0: a, p3: b, weight: 0.12 + l/250}
	if l == 0 {
		s.p1, s.p2 = a, b
		return s
	}
	nx, ny := -dy/l, dx/l
	o1 := (r.Float64()*2 - 1) * curvature * l
	o2 := (r.Float64()*2 - 1) * curvature * l
	s.p1 = point{a.x + dx/3 + nx*o1, a.y + dy/3 + ny*o1}
	s.p2 = point{a.x + 2*dx/3 + nx*o2, a.y + 2*dy/3 + ny*o2}
	return s
}

// sample turns a stroke into steps 8-16 ms apart that together take about d.
func sample(r *rand.Rand, s stroke, d time.Duration) []Step {
	k := max(2, int(math.Round(float64(d)/float64(meanStepDelay))))
	raw := make([]float64, k)
	var sum float64
	for i := range raw {
		raw[i] = float64(minStepDelay) + r.Float64()*float64(maxStepDelay-minStepDelay)
		sum += raw[i]
	}
	delays := make([]time.Duration, k)
	var total time.Duration
	for i, v := range raw {
		delays[i] = min(max(time.Duration(v*float64(d)/sum), minStepDelay), maxStepDelay)
		total += delays[i]
	}
	steps := make([]Step, k)
	var elapsed time.Duration
	for i, delay := range delays {
		elapsed += delay
		u := float64(elapsed) / float64(total)
		if i == k-1 {
			u = 1
		}
		pt := s.at(minimumJerk(u))
		steps[i] = Step{X: pt.x, Y: pt.y, Delay: delay}
	}
	return steps
}

// at evaluates the cubic Bézier curve at t in [0, 1]; at(1) is exactly p3.
func (s stroke) at(t float64) point {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return point{
		a*s.p0.x + b*s.p1.x + c*s.p2.x + d*s.p3.x,
		a*s.p0.y + b*s.p1.y + c*s.p2.y + d*s.p3.y,
	}
}

// minimumJerk is the position profile of a minimum-jerk reach: slow start,
// fast middle, slow arrival.
func minimumJerk(t float64) float64 {
	return t * t * t * (10 - 15*t + 6*t*t)
}

// Duration is how long the burst takes.
func (p Path) Duration() time.Duration {
	var d time.Duration
	for _, s := range p {
		d += s.Delay
	}
	return d
}

// Fit keeps the burst on screen when the pointer at (ox, oy) is near an
// edge of b: it mirrors an axis when that fits, otherwise shrinks the path
// (to no less than a quarter) and clamps what still sticks out. It stays
// edgeInset px off every edge and out of the cornerSize square at each
// corner (see awayFromCorners); a pointer already closer than that is not
// moved any closer. Empty bounds or an origin outside them leave the path
// unchanged.
func (p Path) Fit(b Rect, ox, oy float64) Path {
	if len(p) == 0 || b.W <= 0 || b.H <= 0 || !b.Contains(ox, oy) {
		return p
	}
	var minX, maxX, minY, maxY float64
	for _, s := range p {
		minX, maxX = math.Min(minX, s.X), math.Max(maxX, s.X)
		minY, maxY = math.Min(minY, s.Y), math.Max(maxY, s.Y)
	}
	inset := edgeInset
	if b.W <= 2*inset || b.H <= 2*inset {
		inset = 0
	}
	// Room on each side up to the inset, none for a pointer already in it.
	left, right := max(ox-b.X-inset, 0), max(b.X+b.W-1-inset-ox, 0)
	up, down := max(oy-b.Y-inset, 0), max(b.Y+b.H-1-inset-oy, 0)
	sx, kx := orient(minX, maxX, left, right)
	sy, ky := orient(minY, maxY, up, down)
	k := min(max(min(kx, ky), 0.25), 1)
	out := make(Path, len(p))
	for i, s := range p {
		out[i] = Step{X: s.X * sx * k, Y: s.Y * sy * k, Delay: s.Delay}
	}
	out.awayFromCorners(b, ox, oy)
	for i, s := range out {
		out[i].X = min(max(s.X, -left), right)
		out[i].Y = min(max(s.Y, -up), down)
	}
	return out
}

// awayFromCorners folds the path, in place, away from every corner of b it
// would get into: a path from outside a corner's square never enters it,
// and one from inside never moves deeper. Folding an axis (taking the
// absolute value of its offsets) keeps the path continuous and its return
// to the origin exact.
func (p Path) awayFromCorners(b Rect, ox, oy float64) {
	if b.W < 2*cornerSize || b.H < 2*cornerSize {
		return
	}
	right, bottom := b.X+b.W-1, b.Y+b.H-1
	// away is the direction from each corner into the display.
	for _, c := range []struct{ x, y, awayX, awayY float64 }{
		{b.X, b.Y, 1, 1}, {right, b.Y, -1, 1}, {b.X, bottom, 1, -1}, {right, bottom, -1, -1},
	} {
		// Depth is the distance from the corner along each axis.
		odx, ody := (ox-c.x)*c.awayX, (oy-c.y)*c.awayY
		trouble := false
		for _, s := range p {
			dx, dy := odx+s.X*c.awayX, ody+s.Y*c.awayY
			if dx < cornerSize && dy < cornerSize && (odx >= cornerSize || ody >= cornerSize || dx < odx || dy < ody) {
				trouble = true
				break
			}
		}
		if !trouble {
			continue
		}
		// Folding an axis on which the origin lies outside the square keeps
		// the whole path out of it; with both outside, fold away from the
		// nearer edge. Inside the square, fold both.
		foldX, foldY := odx >= cornerSize, ody >= cornerSize
		switch {
		case foldX && foldY:
			foldX = odx <= ody
			foldY = !foldX
		case !foldX && !foldY:
			foldX, foldY = true, true
		}
		for i := range p {
			if foldX {
				p[i].X = math.Abs(p[i].X) * c.awayX
			}
			if foldY {
				p[i].Y = math.Abs(p[i].Y) * c.awayY
			}
		}
	}
}

// ForUnknownPosition adapts a burst for relative input, where the pointer
// position is unknown. Hot corners sit at the top left on GNOME and Plasma,
// so the burst is mirrored to set off right and down, and its moves up and
// left are scaled to at most unknownReach of its radius. The scaling is
// continuous, so the path keeps its shape and still returns to (0, 0).
func (p Path) ForUnknownPosition() Path {
	var r float64
	sx, sy := 1.0, 1.0
	decided := false
	for _, s := range p {
		d := math.Hypot(s.X, s.Y)
		r = math.Max(r, d)
		if !decided && d >= 5 {
			decided = true
			if s.X < 0 {
				sx = -1
			}
			if s.Y < 0 {
				sy = -1
			}
		}
	}
	out := make(Path, len(p))
	var minX, minY float64
	for i, s := range p {
		out[i] = Step{X: s.X * sx, Y: s.Y * sy, Delay: s.Delay}
		minX, minY = math.Min(minX, out[i].X), math.Min(minY, out[i].Y)
	}
	limit := unknownReach * r
	kx, ky := 1.0, 1.0
	if minX < -limit {
		kx = limit / -minX
	}
	if minY < -limit {
		ky = limit / -minY
	}
	for i := range out {
		if out[i].X < 0 {
			out[i].X *= kx
		}
		if out[i].Y < 0 {
			out[i].Y *= ky
		}
	}
	return out
}

// orient picks the direction (1 or -1) for an axis whose path spans
// [lo, hi] with room neg below and pos above the origin, and the scale that
// makes it fit (1 when it already does).
func orient(lo, hi, neg, pos float64) (sign, scale float64) {
	keep, flip := fitScale(lo, hi, neg, pos), fitScale(-hi, -lo, neg, pos)
	if keep >= 1 || keep >= flip {
		return 1, keep
	}
	return -1, flip
}

func fitScale(lo, hi, neg, pos float64) float64 {
	k := 1.0
	if lo < 0 {
		k = math.Min(k, neg/-lo)
	}
	if hi > 0 {
		k = math.Min(k, pos/hi)
	}
	return k
}

// Deltas converts the path to integer relative moves. Rounding each
// absolute position (instead of each delta) carries the error forward, so
// the moves sum to exactly zero. Moves that round to nothing are merged into
// the next one, as are moves closer together than minStep; the final move
// back to the origin is never dropped.
func (p Path) Deltas(minStep time.Duration) []Delta {
	var out []Delta
	var cx, cy int
	var pending time.Duration
	for i, s := range p {
		pending += s.Delay
		if pending < minStep && i < len(p)-1 {
			continue
		}
		tx, ty := int(math.Round(s.X)), int(math.Round(s.Y))
		if tx == cx && ty == cy {
			continue
		}
		out = append(out, Delta{DX: tx - cx, DY: ty - cy, Delay: pending})
		cx, cy, pending = tx, ty, 0
	}
	return out
}

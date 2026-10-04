package activity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/clock"
)

const (
	// tickInterval is how often idle time and the lock state are read.
	tickInterval = 2 * time.Second
	// reprobeInterval is how often a missing injector is looked for again.
	reprobeInterval = 60 * time.Second
	// userReturnTolerance absorbs idle-counter granularity: the user is back
	// when idle+tolerance is less than the time since our last burst.
	userReturnTolerance = 3 * time.Second
	// effectiveIdle is the idle time below which a burst counts as having
	// reset the counter the chat apps read.
	effectiveIdle = 1500 * time.Millisecond
	// settleDelay gives the OS time to account for the burst before it is
	// verified.
	settleDelay = 200 * time.Millisecond
	// ineffectiveLimit consecutive bursts without effect mean degraded.
	ineffectiveLimit = 2
	// cadenceJitter spreads bursts uniformly over Interval ±35 %.
	cadenceJitter = 0.35
)

const noIdleReason = "no idle source on this desktop; simulating on a fixed schedule"

type controllerDeps struct {
	clock clock.Clock
	// idle gates simulation; nil means no idle source (fixed schedule).
	idle IdleSource
	// verify checks bursts; nil means idle. macOS verifies against the
	// counter Electron reads, which is not the one it gates on.
	verify IdleSource
	// lock pauses simulation; nil means never locked.
	lock LockSource
	// open picks an injector; while it fails it is retried every minute.
	open  func() (Injector, error)
	rnd   *rand.Rand
	sleep sleepFunc
	// noIdleHint explains how to get an idle source on this desktop.
	noIdleHint string
}

// controller is the OS-independent state machine behind the Simulator: it
// waits for the user to go idle, bursts at a jittered interval, verifies
// every burst against the idle counter and backs off when the user returns
// or the screen locks.
type controller struct {
	cfg    Config
	deps   controllerDeps
	report func(Status)

	inj       Injector
	nextProbe time.Time
	fixed     bool
	armed     bool
	lastBurst time.Time // end of the last burst since arming
	nextBurst time.Time
	misses    int
	lastErr   error  // error of the last fixed-schedule burst
	status    Status // last published
	armedSt   Status // status between bursts while armed
}

func newController(cfg Config, d controllerDeps, report func(Status)) *controller {
	if d.verify == nil {
		d.verify = d.idle
	}
	if d.sleep == nil {
		d.sleep = realSleep
	}
	return &controller{cfg: cfg, deps: d, report: report}
}

// init decides between idle-gated and fixed-schedule mode. Without an idle
// source the user has just started keepalive, so the first burst waits one
// idle threshold.
func (c *controller) init() {
	now := c.deps.clock.Now()
	c.nextProbe = now
	c.fixed = c.deps.idle == nil
	if !c.fixed {
		if _, err := c.deps.idle.Idle(); err != nil {
			slog.Warn("activity: idle source failed; using a fixed schedule", "source", c.deps.idle.Name(), "err", err)
			c.fixed = true
		}
	}
	if c.fixed {
		c.nextBurst = now.Add(c.cfg.IdleThreshold)
	}
}

func (c *controller) run(ctx context.Context) error {
	// init reads the idle source, which may be a D-Bus call.
	if ctx.Err() != nil {
		return nil
	}
	c.init()
	defer c.close()
	t := c.deps.clock.NewTimer(c.step(ctx))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C():
		}
		if ctx.Err() != nil {
			return nil
		}
		t.Reset(c.step(ctx))
	}
}

func (c *controller) close() {
	if c.inj != nil {
		if err := c.inj.Close(); err != nil {
			slog.Warn("activity: closing injector failed", "method", c.inj.Name(), "err", err)
		}
		c.inj = nil
	}
}

// step runs one evaluation and returns how long to wait before the next.
func (c *controller) step(ctx context.Context) time.Duration {
	now := c.deps.clock.Now()
	if c.inj == nil {
		if now.Before(c.nextProbe) {
			return min(tickInterval, c.nextProbe.Sub(now))
		}
		c.nextProbe = now.Add(reprobeInterval)
		inj, err := c.deps.open()
		if err != nil {
			reason, hint := explain(err)
			c.publish(Status{State: StateDegraded, Reason: reason, Hint: hint})
			return tickInterval
		}
		slog.Info("activity: input method", "method", inj.Name())
		c.inj = inj
	}
	method := c.inj.Name()

	if c.deps.lock != nil {
		if locked, err := c.deps.lock.Locked(); err == nil && locked {
			c.disarm()
			c.publish(Status{State: StatePausedLocked, Method: method, Reason: "the screen is locked"})
			return tickInterval
		}
	}
	if c.fixed {
		return c.fixedStep(ctx, now)
	}

	idle, err := c.deps.idle.Idle()
	if err != nil {
		c.publish(Status{State: StateDegraded, Method: method,
			Reason: fmt.Sprintf("cannot read idle time from %s: %v", c.deps.idle.Name(), err)})
		return tickInterval
	}
	if c.armed && !c.lastBurst.IsZero() && idle+userReturnTolerance < now.Sub(c.lastBurst) {
		c.disarm()
		c.publish(Status{State: StatePausedUser, Method: method, Reason: "you are using the computer", Idle: idle})
		return tickInterval
	}
	if !c.armed {
		if idle < c.cfg.IdleThreshold {
			st := Status{State: StateWaitingIdle, Method: method, Idle: idle,
				Reason: fmt.Sprintf("needs %s without input", c.cfg.IdleThreshold)}
			if c.status.State == StatePausedUser {
				st = c.status
				st.Idle = idle
			}
			c.publish(st)
			return tickInterval
		}
		c.armed = true
		c.nextBurst = now
	}
	if now.Before(c.nextBurst) {
		st := c.armedSt
		st.Idle = idle
		c.publish(st)
		return min(tickInterval, c.nextBurst.Sub(now))
	}
	c.burst(ctx)
	return c.untilNextBurst()
}

func (c *controller) disarm() {
	c.armed = false
	c.lastBurst = time.Time{}
	c.misses = 0
}

// burst plays one burst and verifies that it reset the idle counter.
func (c *controller) burst(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	err := c.play(ctx)
	if ctx.Err() != nil {
		return
	}
	end := c.deps.clock.Now()
	c.lastBurst = end
	c.nextBurst = end.Add(c.jitter())

	var idle time.Duration
	effective := false
	if err == nil {
		if c.deps.sleep(ctx, settleDelay) != nil {
			return
		}
		idle, err = c.deps.verify.Idle()
		effective = err == nil && idle < effectiveIdle
	}
	st := Status{State: StateSimulating, Method: c.inj.Name(), LastBurst: end, Idle: idle}
	if effective {
		c.misses = 0
	} else {
		c.misses++
		slog.Debug("activity: burst had no effect", "method", st.Method, "idle_after", idle, "err", err, "misses", c.misses)
	}
	if c.misses >= ineffectiveLimit {
		if aerr := c.inj.Available(); aerr != nil {
			// The backend went away (daemon stopped, permission revoked):
			// look for one again.
			c.close()
			c.nextProbe = end.Add(reprobeInterval)
			c.disarm()
			reason, hint := explain(aerr)
			c.publish(Status{State: StateDegraded, Reason: reason, Hint: hint})
			return
		}
		st.State = StateDegraded
		st.Reason, st.Hint = c.diagnose(err)
	}
	c.armedSt = st
	c.publish(st)
}

// fixedStep bursts every Interval without idle information.
func (c *controller) fixedStep(ctx context.Context, now time.Time) time.Duration {
	if now.Before(c.nextBurst) {
		c.publish(c.fixedStatus())
		return min(tickInterval, c.nextBurst.Sub(now))
	}
	if ctx.Err() != nil {
		return tickInterval
	}
	err := c.play(ctx)
	if ctx.Err() != nil {
		return tickInterval
	}
	end := c.deps.clock.Now()
	c.lastBurst = end
	c.nextBurst = end.Add(c.cfg.Interval)
	c.lastErr = err
	if err != nil {
		c.misses++
	} else {
		c.misses = 0
	}
	c.publish(c.fixedStatus())
	return c.untilNextBurst()
}

func (c *controller) fixedStatus() Status {
	st := Status{State: StateDegraded, Method: c.inj.Name(), Reason: noIdleReason, Hint: c.deps.noIdleHint, LastBurst: c.lastBurst}
	if c.misses >= ineffectiveLimit {
		st.Reason, st.Hint = c.diagnose(c.lastErr)
	}
	return st
}

func (c *controller) diagnose(err error) (string, string) {
	reason, hint := c.inj.Diagnose()
	if reason == "" {
		reason = "synthetic input did not reset the idle timer"
	}
	if err != nil {
		reason += ": " + err.Error()
	}
	return reason, hint
}

func (c *controller) untilNextBurst() time.Duration {
	d := c.nextBurst.Sub(c.deps.clock.Now())
	if d <= 0 {
		return tickInterval
	}
	return min(tickInterval, d)
}

func (c *controller) play(ctx context.Context) error {
	err := playBurst(ctx, c.inj, NewPath(c.deps.rnd), c.deps.sleep)
	if err == nil && c.cfg.Keys {
		if terr := c.inj.Tap(); terr != nil {
			slog.Debug("activity: key tap failed", "method", c.inj.Name(), "err", terr)
		}
	}
	return err
}

func (c *controller) jitter() time.Duration {
	f := 1 + (c.deps.rnd.Float64()*2-1)*cadenceJitter
	return time.Duration(float64(c.cfg.Interval) * f)
}

func (c *controller) publish(st Status) {
	if st.State != c.status.State || st.Reason != c.status.Reason {
		slog.Info("activity: state", "state", st.State, "method", st.Method, "reason", st.Reason)
	}
	c.status = st
	c.report(st)
}

func explain(err error) (reason, hint string) {
	var u *Unavailable
	if errors.As(err, &u) {
		return u.Reason, u.Hint
	}
	return err.Error(), ""
}

// playBurst moves the pointer along p from where it is now and back. An
// interrupted burst still returns the pointer to its origin.
func playBurst(ctx context.Context, inj Injector, p Path, sleep sleepFunc) error {
	switch in := inj.(type) {
	case AbsoluteInjector:
		ox, oy, ok := in.Position()
		if !ok {
			return errors.New("cannot read the pointer position")
		}
		if b, ok := in.Bounds(); ok {
			p = p.Fit(b, ox, oy)
		}
		if pl, ok := inj.(pathPlayer); ok {
			return pl.Play(ctx, ox, oy, p)
		}
		for _, s := range p {
			if err := sleep(ctx, s.Delay); err != nil {
				_ = in.MoveTo(ox, oy)
				return err
			}
			if err := in.MoveTo(ox+s.X, oy+s.Y); err != nil {
				_ = in.MoveTo(ox, oy)
				return err
			}
		}
		return nil
	case RelativeInjector:
		var minStep time.Duration
		if m, ok := inj.(minStepper); ok {
			minStep = m.MinStep()
		}
		var x, y int
		undo := func() {
			if x != 0 || y != 0 {
				_ = in.MoveBy(-x, -y)
			}
		}
		for _, d := range p.Deltas(minStep) {
			if err := sleep(ctx, d.Delay); err != nil {
				undo()
				return err
			}
			if err := in.MoveBy(d.DX, d.DY); err != nil {
				undo()
				return err
			}
			x, y = x+d.DX, y+d.DY
		}
		return nil
	}
	return fmt.Errorf("%s cannot move the pointer", inj.Name())
}

package activity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/clock"
)

const (
	// tickInterval is how often idle time and the lock state are read.
	tickInterval = 2 * time.Second
	// reprobeInterval is how often a missing injector is looked for again.
	reprobeInterval = 60 * time.Second
	// effectiveIdle is the idle time below which a burst counts as having
	// reset the counter the chat apps read.
	effectiveIdle = 1500 * time.Millisecond
	// settleDelay gives the OS time to account for the burst before it is
	// verified.
	settleDelay = 200 * time.Millisecond
	// ineffectiveLimit consecutive bursts without effect mean degraded.
	ineffectiveLimit = 2
	// failLimit consecutive bursts that an input method fails to play mark
	// it failed; the next available method takes over.
	failLimit = 2
	// cadenceJitter spreads bursts uniformly over Interval ±35 %.
	cadenceJitter = 0.35
)

const userReason = "you are using the computer"

const (
	// userCheckEvery is how many moves an absolute burst makes between
	// checks that the pointer is still where it put it.
	userCheckEvery = 3
	// userMoveTolerance absorbs rounding to whole pixels.
	userMoveTolerance = 3.0
)

// errUserMoved stops a burst when the user resumes any input.
var errUserMoved = errors.New("user input interrupted the burst")

type controllerDeps struct {
	clock clock.Clock
	// idle seeds initial inactivity; simulated input may reset it.
	idle    IdleSource
	monitor func(context.Context) (inputMonitor, error)
	// verify checks bursts; nil means idle. macOS verifies against the
	// counter Electron reads, which is not the one it gates on.
	verify IdleSource
	// lock pauses simulation; nil means never locked.
	lock LockSource
	// open picks an injector; while it fails it is retried every minute.
	open func() (Injector, error)
	// next opens the first available input method that skip does not
	// reject, in the same order as open; nil when the OS has only one.
	next  func(skip func(name string) bool) (Injector, error)
	rnd   *rand.Rand
	sleep sleepFunc
	// noIdleHint explains how to get an idle source on this desktop.
	noIdleHint string
	// secondary is read around every burst for information only (XWayland
	// on a Wayland session); nil when there is none. After it misses two
	// bursts in a row that verify saw, simulating reports secondaryHint.
	secondary     IdleSource
	secondaryHint string
}

// controller is the OS-independent state machine behind the Simulator: it
// waits for the user to go idle, bursts at a jittered interval, verifies
// every burst against the idle counter and backs off when the user returns
// or the screen locks.
type controller struct {
	cfg    Config
	deps   controllerDeps
	report func(Status)

	inj              Injector
	nextProbe        time.Time
	monitor          inputMonitor
	monitorCancel    context.CancelFunc
	nextMonitorProbe time.Time
	monitorOpened    bool
	generation       uint64
	resumeAfter      time.Time
	armed            bool
	lastBurst        time.Time // end of the last burst since arming
	nextBurst        time.Time
	misses           int
	errs             int                  // bursts in a row the injector failed to play
	failed           map[string]time.Time // input methods marked failed, and when
	recheck          time.Time            // when failed methods get another chance
	secMisses        int                  // bursts in a row the secondary counter missed
	secWarned        bool                 // secondaryHint is shown from now on
	status           Status               // last published
	armedSt          Status               // status between bursts while armed
}

func newController(cfg Config, d controllerDeps, report func(Status)) *controller {
	if d.verify == nil {
		d.verify = d.idle
	}
	if d.sleep == nil {
		d.sleep = realSleep
	}
	return &controller{cfg: cfg, deps: d, report: report, failed: map[string]time.Time{}}
}

func (c *controller) init() {
	c.nextProbe = c.deps.clock.Now()
}

func (c *controller) monitorStatus(err error) {
	c.disarm()
	reason, hint := explain(err)
	c.publish(Status{State: StateDegraded, Reason: "user input monitoring unavailable: " + reason, Hint: hint})
}

func (c *controller) closeMonitor() {
	if c.monitor != nil {
		_ = c.monitor.Close()
		c.monitor = nil
		if c.monitorCancel != nil {
			c.monitorCancel()
			c.monitorCancel = nil
		}
	}
}

func (c *controller) ensureMonitor(ctx context.Context, now time.Time) bool {
	if c.monitor != nil {
		if err := c.monitor.Snapshot().Err; err != nil {
			c.monitorStatus(err)
			c.closeMonitor()
			c.nextMonitorProbe = now.Add(reprobeInterval)
		} else {
			return true
		}
	}
	if now.Before(c.nextMonitorProbe) {
		return false
	}
	c.nextMonitorProbe = now.Add(reprobeInterval)
	if c.deps.monitor == nil {
		c.monitorStatus(&Unavailable{Reason: "no user input monitor on this system", Hint: c.deps.noIdleHint})
		return false
	}
	firstAttempt := !c.monitorOpened
	c.monitorOpened = true
	// Keep observing during bounded shutdown cleanup. Cancelling the session
	// must stop playback, but must not blind a possible return to origin.
	monitorCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopStartup := context.AfterFunc(ctx, cancel)
	monitor, err := c.deps.monitor(monitorCtx)
	stopStartup()
	if err != nil {
		cancel()
		c.monitorStatus(err)
		return false
	}
	if ctx.Err() != nil {
		_ = monitor.Close()
		cancel()
		return false
	}
	c.monitor, c.monitorCancel = monitor, cancel
	// Subscribe before reading the OS counter. Never seed after recovery: a
	// gap in observation requires a fresh full idle wait.
	snap := monitor.Snapshot()
	if firstAttempt && c.deps.idle != nil {
		if idle, err := c.deps.idle.Idle(); err == nil {
			monitor.SeedIdle(idle, snap.Generation)
		}
	}
	return c.ensureMonitor(ctx, now)
}

func (c *controller) run(ctx context.Context) error {
	// Avoid all native setup when the run was already cancelled.
	if ctx.Err() != nil {
		return nil
	}
	c.init()
	defer c.close()
	defer c.closeMonitor()
	t := c.deps.clock.NewTimer(c.step(ctx))
	defer t.Stop()
	for {
		var changed <-chan struct{}
		if c.monitor != nil {
			snap := c.monitor.Snapshot()
			changed = snap.Changed
			if c.armed && (snap.Generation != c.generation || snap.Held || snap.Err != nil) {
				t.Reset(c.step(ctx))
				continue
			}
		}
		select {
		case <-changed:
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
	if ctx.Err() != nil {
		return tickInterval
	}
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
		// Create virtual devices without emitting input before observation
		// begins, so our own device arrival does not invalidate X11 coverage
		// on the first burst.
		if p, ok := inj.(interface{ Prepare(context.Context) error }); ok {
			if err := p.Prepare(ctx); err != nil {
				c.close()
				reason, hint := explain(err)
				c.publish(Status{State: StateDegraded, Reason: reason, Hint: hint})
				return tickInterval
			}
		}
	}
	if !c.ensureMonitor(ctx, now) {
		return tickInterval
	}
	if ctx.Err() != nil {
		return tickInterval
	}
	c.recheckFailed(now)
	method := c.inj.Name()

	if c.deps.lock != nil {
		if locked, err := c.deps.lock.Locked(); err == nil && locked {
			c.disarm()
			c.resumeAfter = now.Add(c.cfg.IdleThreshold)
			c.publish(Status{State: StatePausedLocked, Method: method, Reason: "the screen is locked"})
			return tickInterval
		}
	}
	snap := c.monitor.Snapshot()
	if snap.Err != nil {
		c.ensureMonitor(ctx, now)
		return tickInterval
	}
	idle := snap.Idle
	if c.armed && (snap.Generation != c.generation || snap.Held) {
		c.disarm()
		c.publish(Status{State: StatePausedUser, Method: method, Reason: userReason, Idle: idle})
		return tickInterval
	}
	if !c.armed {
		if idle < c.cfg.IdleThreshold || snap.Held || now.Before(c.resumeAfter) {
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
		c.generation = snap.Generation
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
	secBefore, secOK := c.readSecondary()
	err := c.play(ctx)
	if ctx.Err() != nil {
		return
	}
	if errors.Is(err, errUserMoved) {
		// Also reset inactivity when the position check detected takeover
		// before the monitor delivered its event.
		c.monitor.SeedIdle(0, c.generation)
		c.disarm()
		c.publish(Status{State: StatePausedUser, Method: c.inj.Name(), Reason: userReason})
		return
	}
	if isMonitorFailure(err) {
		c.ensureMonitor(ctx, c.deps.clock.Now())
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
		if c.deps.verify != nil {
			idle, err = c.deps.verify.Idle()
			effective = err == nil && idle < effectiveIdle
		} else {
			effective = true // no counter to verify; input itself succeeded
		}
	}
	if guardErr := (inputGuard{c.monitor, c.generation}).check(); guardErr != nil {
		if errors.Is(guardErr, errUserMoved) {
			c.disarm()
			c.publish(Status{State: StatePausedUser, Method: c.inj.Name(), Reason: userReason})
		} else {
			c.ensureMonitor(ctx, c.deps.clock.Now())
		}
		return
	}
	if effective && secOK {
		c.checkSecondary(secBefore)
	}
	st := Status{State: StateSimulating, Method: c.inj.Name(), LastBurst: end, Idle: idle}
	if c.deps.verify == nil {
		st.Hint = "user input is monitored, but this desktop exposes no idle counter to verify simulated input"
	}
	if c.secWarned {
		st.Hint = c.deps.secondaryHint
	}
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

func (c *controller) readSecondary() (time.Duration, bool) {
	if c.deps.secondary == nil {
		return 0, false
	}
	d, err := c.deps.secondary.Idle()
	return d, err == nil
}

// checkSecondary compares the secondary counter after a burst that reset
// the verified one. Input still works, so the state stays simulating; after
// two misses in a row the hint is shown for the rest of the run.
func (c *controller) checkSecondary(before time.Duration) {
	if before < effectiveIdle {
		return // not idle before the burst: nothing to compare
	}
	after, err := c.deps.secondary.Idle()
	if err != nil {
		return
	}
	if after < effectiveIdle {
		c.secMisses = 0
		return
	}
	c.secMisses++
	if c.secMisses >= ineffectiveLimit && !c.secWarned {
		c.secWarned = true
		slog.Warn("activity: simulated input does not reach "+c.deps.secondary.Name(), "idle_after", after)
	}
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

// play plays one burst. When the input method fails to play it, the
// failure is counted, and a method that takes over plays the same burst.
func (c *controller) play(ctx context.Context) error {
	for {
		err := c.playOnce(ctx)
		if err == nil {
			c.errs = 0
			delete(c.failed, c.inj.Name())
			return nil
		}
		if ctx.Err() != nil || errors.Is(err, errUserMoved) || isMonitorFailure(err) || !c.failover() {
			return err
		}
	}
}

// failover counts a failed burst. After failLimit failures in a row (one,
// for a method that failed before) it marks the method failed and switches
// to the next available one; it reports whether it switched. Without
// another method the failed one stays in use.
func (c *controller) failover() bool {
	c.errs++
	name := c.inj.Name()
	_, failedBefore := c.failed[name]
	if c.deps.next == nil || (c.errs < failLimit && !failedBefore) {
		return false
	}
	now := c.deps.clock.Now()
	c.failed[name] = now
	c.recheck = now.Add(reprobeInterval)
	inj, err := c.deps.next(c.recentlyFailed(now))
	if err != nil {
		return false
	}
	slog.Warn("activity: input method failed; trying the next one", "failed", name, "method", inj.Name())
	c.switchTo(inj)
	return true
}

// recheckFailed gives methods that failed a minute ago another chance: the
// first available method that has not failed since takes over if it is
// not the current one. A retried method that fails once more is dropped
// again at once.
func (c *controller) recheckFailed(now time.Time) {
	if c.deps.next == nil || len(c.failed) == 0 || now.Before(c.recheck) {
		return
	}
	c.recheck = now.Add(reprobeInterval)
	inj, err := c.deps.next(c.recentlyFailed(now))
	if err != nil {
		return
	}
	if inj.Name() == c.inj.Name() {
		_ = inj.Close()
		return
	}
	slog.Info("activity: trying an input method again", "method", inj.Name())
	c.switchTo(inj)
}

func (c *controller) recentlyFailed(now time.Time) func(string) bool {
	return func(name string) bool {
		t, ok := c.failed[name]
		return ok && now.Sub(t) < reprobeInterval
	}
}

func (c *controller) switchTo(inj Injector) {
	c.close()
	c.inj = inj
	c.errs, c.misses = 0, 0
	c.armedSt.Method = inj.Name()
}

func (c *controller) playOnce(ctx context.Context) error {
	ctx, stop := watchInput(ctx, c.monitor, c.generation)
	defer stop()
	if err := inputError(ctx); err != nil {
		return err
	}
	err := playBurst(ctx, c.inj, NewPath(c.deps.rnd), c.deps.sleep)
	if interrupted := inputError(ctx); interrupted != nil {
		return interrupted
	}
	if err == nil && c.cfg.Keys {
		if terr := c.inj.Tap(ctx); terr != nil {
			if interrupted := inputError(ctx); interrupted != nil {
				return interrupted
			}
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
// interrupted burst only restores its origin while monitoring confirms that
// the user has not taken over. Absolute position checks provide an additional
// fallback; the input guard protects relative and whole-path playback too.
func playBurst(ctx context.Context, inj Injector, p Path, sleep sleepFunc) error {
	if err := inputError(ctx); err != nil {
		return err
	}
	switch in := inj.(type) {
	case AbsoluteInjector:
		ox, oy, ok := in.Position()
		if !ok {
			return errors.New("cannot read the pointer position")
		}
		if b, ok := in.Bounds(); ok {
			p = p.Fit(b, ox, oy)
		}
		if err := inputError(ctx); err != nil {
			return err
		}
		if pl, ok := inj.(pathPlayer); ok {
			return pl.Play(ctx, ox, oy, p)
		}
		last, prev := move{ox, oy}, move{ox, oy}
		for i, s := range p {
			if err := sleep(ctx, s.Delay); err != nil {
				restore(ctx, func(cleanup context.Context) { _ = in.MoveTo(cleanup, ox, oy) })
				return err
			}
			if err := inputError(ctx); err != nil {
				return err
			}
			if i > 0 && i%userCheckEvery == 0 {
				if x, y, ok := in.Position(); ok && userMoved(x, y, last, prev) {
					return errUserMoved
				}
			}
			x, y := ox+s.X, oy+s.Y
			if err := in.MoveTo(ctx, x, y); err != nil {
				restore(ctx, func(cleanup context.Context) { _ = in.MoveTo(cleanup, ox, oy) })
				return err
			}
			last, prev = move{x, y}, last
		}
		if err := inputError(ctx); err != nil {
			return err
		}
		if f, ok := inj.(finisher); ok {
			f.FinishAt(ctx, ox, oy)
		}
		return nil
	case RelativeInjector:
		p = p.ForUnknownPosition()
		var minStep time.Duration
		if m, ok := inj.(minStepper); ok {
			minStep = m.MinStep()
		}
		var x, y int
		undo := func() {
			if x != 0 || y != 0 {
				restore(ctx, func(cleanup context.Context) { _ = in.MoveBy(cleanup, -x, -y) })
			}
		}
		for _, d := range p.Deltas(minStep) {
			if err := sleep(ctx, d.Delay); err != nil {
				undo()
				return err
			}
			if err := inputError(ctx); err != nil {
				return err
			}
			if err := in.MoveBy(ctx, d.DX, d.DY); err != nil {
				undo()
				return err
			}
			x, y = x+d.DX, y+d.DY
		}
		return nil
	}
	return fmt.Errorf("%s cannot move the pointer", inj.Name())
}

type move struct{ x, y float64 }

// userMoved reports whether the pointer at (x, y) is more than a few pixels
// from the last point a burst set. The point before it is accepted too, in
// case the OS has not applied the last move yet.
func userMoved(x, y float64, last, prev move) bool {
	return math.Hypot(x-last.x, y-last.y) > userMoveTolerance && math.Hypot(x-prev.x, y-prev.y) > userMoveTolerance
}

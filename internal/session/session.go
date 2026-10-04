package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/proc"
)

// Session is a single-use keep-alive session. Create it with New, then call
// Run once; the other methods are safe from any goroutine.
type Session struct {
	cfg  Config
	deps Deps

	stopCh  chan Reason      // buffered(1): the first Stop wins
	cmds    chan func(*loop) // executed by the loop goroutine
	ran     atomic.Bool      // Run was called
	running atomic.Bool      // the loop accepts commands
	done    chan struct{}    // closed when Run returns
	last    atomic.Pointer[Snapshot]
	bus     broadcaster
}

// New prepares a session. A nil Clock means the real clock; a nil Activity
// simulator turns an activity request into a warning; a nil Battery func
// disables battery polling; nil Processes means SystemProcesses().
func New(cfg Config, deps Deps) *Session {
	if deps.Clock == nil {
		deps.Clock = clock.Real()
	}
	if deps.Processes == nil {
		deps.Processes = SystemProcesses()
	}
	if cfg.Activity.IdleThreshold <= 0 {
		cfg.Activity.IdleThreshold = activity.DefaultIdleThreshold
	}
	if cfg.Activity.Interval <= 0 {
		cfg.Activity.Interval = activity.DefaultInterval
	}
	s := &Session{
		cfg:    cfg,
		deps:   deps,
		stopCh: make(chan Reason, 1),
		cmds:   make(chan func(*loop)),
		done:   make(chan struct{}),
	}
	s.bus.init()
	snap := initialSnapshot(cfg)
	s.last.Store(&snap)
	return s
}

// Run starts the session and blocks until it ends. Cancelling ctx stops the
// session with ReasonSignal. Run may be called only once.
func (s *Session) Run(ctx context.Context) Result {
	if !s.ran.CompareAndSwap(false, true) {
		return Result{Reason: ReasonError, Err: errors.New("session: Run called more than once")}
	}
	defer close(s.done)
	l := &loop{
		s:              s,
		ctx:            ctx,
		clk:            s.deps.Clock,
		snap:           initialSnapshot(s.cfg),
		batteryResults: make(chan batteryResult, 1),
		actStatus:      make(chan activityMsg, 16),
		actExit:        make(chan activityExit, 4),
		notified:       map[string]time.Time{},
	}
	res := l.run()
	s.running.Store(false)
	l.notifying.Wait()
	return res
}

// Subscribe returns a stream of events and a func that cancels the
// subscription. The channel is closed when the session ends. A subscriber
// that falls behind loses its oldest undelivered events, never the newest.
func (s *Session) Subscribe() (<-chan Event, func()) { return s.bus.subscribe() }

// Snapshot returns the current state. Outside Run it returns the last state
// the session published.
func (s *Session) Snapshot() Snapshot {
	var snap Snapshot
	if s.do(func(l *loop) { snap = l.snapshot() }) {
		return snap
	}
	return *s.last.Load()
}

// SetActive starts or stops activity simulation. It is a no-op outside Run.
func (s *Session) SetActive(on bool) {
	s.do(func(l *loop) { l.setActive(on) })
}

// Extend moves the end of a timed session by d. A negative d shortens it but
// never below one minute from now. Indefinite sessions ignore it with a
// warning event. It is a no-op outside Run.
func (s *Session) Extend(d time.Duration) {
	s.do(func(l *loop) { l.extend(d) })
}

// Stop ends the session with reason r (ReasonUser when empty). Only the first
// Stop counts; a Stop before Run makes Run return at once without acquiring
// anything.
func (s *Session) Stop(r Reason) {
	if r == "" {
		r = ReasonUser
	}
	select {
	case s.stopCh <- r:
	default:
	}
}

// do runs fn on the loop goroutine and waits for it. It reports false when
// the loop is not running.
func (s *Session) do(fn func(*loop)) bool {
	if !s.running.Load() {
		return false
	}
	ack := make(chan struct{})
	select {
	case s.cmds <- func(l *loop) { fn(l); close(ack) }:
		<-ack
		return true
	case <-s.done:
		return false
	}
}

func initialSnapshot(cfg Config) Snapshot {
	snap := Snapshot{
		Mode:        cfg.mode(),
		Active:      cfg.Active,
		Activity:    activity.Status{State: activity.StateOff},
		Battery:     Battery{Threshold: cfg.BatteryThreshold},
		KeepDisplay: cfg.KeepDisplay,
		InWindow:    cfg.Schedule == nil,
		Watching:    watchDescription(cfg.Command, cfg.WatchPIDs, cfg.WatchProcess),
	}
	if cfg.Schedule != nil {
		snap.Schedule = cfg.Schedule.String()
	}
	if snap.Mode == ModeUntil {
		snap.EndsAt = cfg.Until
	}
	return snap
}

// ---- the loop: everything below runs on the Run goroutine ----

type batteryResult struct {
	status platform.BatteryStatus
	err    error
}

type activityMsg struct {
	gen    int
	status activity.Status
}

type activityExit struct {
	gen int
	err error
}

type activityRun struct {
	gen    int
	cancel context.CancelFunc
}

type loop struct {
	s    *Session
	ctx  context.Context
	clk  clock.Clock
	snap Snapshot
	hold power.Hold // nil outside the work hours
	// stopMsg overrides the stopped event's message.
	stopMsg string

	deadline    clock.Timer
	heartbeat   clock.Ticker
	batteryTick clock.Ticker

	powerLost    <-chan error // the hold's power.Watcher channel; nil when not watched
	powerRetry   clock.Timer  // armed while the hold is lost
	powerRetries int          // re-acquire attempts since the loss

	batteryBusy    bool
	batteryFailing bool
	batteryResults chan batteryResult

	act        *activityRun
	actGen     int
	actRunning int
	actStatus  chan activityMsg
	actExit    chan activityExit

	inWindow   bool        // inside the schedule (always true without one)
	schedTimer clock.Timer // fires at the next schedule change

	watchExit   <-chan proc.Exit
	cancelWatch context.CancelFunc

	notified  map[string]time.Time // last notification per kind
	notifying sync.WaitGroup
}

func (l *loop) run() Result {
	cfg := l.s.cfg
	l.snap.StartedAt = l.clk.Now()

	if err := cfg.validate(); err != nil {
		return l.finish(ReasonError, err)
	}
	select {
	case r := <-l.s.stopCh:
		return l.finish(r, nil)
	default:
	}
	if l.ctx.Err() != nil {
		return l.finish(ReasonSignal, nil)
	}
	if l.s.deps.Power == nil {
		return l.finish(ReasonError, errors.New("session: no power inhibitor"))
	}

	l.s.running.Store(true)
	if err := l.startWatch(); err != nil {
		return l.finish(ReasonError, err)
	}
	l.inWindow = cfg.Schedule == nil || cfg.Schedule.In(l.clk.Now())
	if l.inWindow {
		hold, err := l.s.deps.Power.Acquire(l.ctx, l.powerOptions())
		if err != nil {
			if l.ctx.Err() != nil {
				return l.finish(ReasonSignal, nil)
			}
			return l.finish(ReasonError, fmt.Errorf("keep the system awake: %w", err))
		}
		l.hold = hold
		l.watchPower()
		l.snap.PowerHold = hold.Describe()
	}
	now := l.clk.Now()
	l.snap.StartedAt = now
	l.snap.Running = true
	l.snap.InWindow = l.inWindow
	l.armSchedule(now)
	if cfg.Duration > 0 {
		l.snap.EndsAt = now.Add(cfg.Duration)
	}
	if !l.snap.EndsAt.IsZero() {
		l.deadline = l.clk.NewTimer(l.snap.EndsAt.Sub(now))
	}
	l.heartbeat = l.clk.NewTicker(HeartbeatInterval)
	if cfg.BatteryThreshold > 0 && l.s.deps.Battery != nil {
		l.batteryTick = l.clk.NewTicker(BatteryPollInterval)
	}
	slog.Info("session: started", "mode", l.snap.Mode, "ends_at", l.snap.EndsAt, "hold", l.snap.PowerHold,
		"schedule", l.snap.Schedule, "in_window", l.inWindow, "watching", l.snap.Watching)
	l.emit(EventStarted, "", "")

	if !l.snap.EndsAt.IsZero() && !now.Before(l.snap.EndsAt) {
		return l.stop(l.timedReason(), nil)
	}
	if l.batteryTick != nil {
		l.pollBattery()
	}
	if cfg.Active && l.inWindow {
		l.startActivity()
	}
	return l.loop()
}

func (l *loop) loop() Result {
	for {
		select {
		case <-l.ctx.Done():
			return l.stop(ReasonSignal, nil)
		case r := <-l.s.stopCh:
			return l.stop(r, nil)
		case fn := <-l.s.cmds:
			fn(l)
		case <-timerC(l.deadline):
			if l.deadlinePassed() {
				return l.stop(l.timedReason(), nil)
			}
		case <-tickerC(l.heartbeat):
			if l.deadlinePassed() {
				return l.stop(l.timedReason(), nil)
			}
			l.checkSchedule()
			l.emit(EventSnapshot, "", "")
		case <-timerC(l.schedTimer):
			l.checkSchedule()
		case ex, ok := <-l.watchExit:
			if !ok {
				l.watchExit = nil
				continue
			}
			l.stopMsg = l.exitMessage(ex)
			return l.stop(ReasonProcessExited, nil)
		case <-tickerC(l.batteryTick):
			l.pollBattery()
		case r := <-l.batteryResults:
			if l.handleBattery(r) {
				return l.stop(ReasonBattery, nil)
			}
		case m := <-l.actStatus:
			l.handleActivityStatus(m)
		case e := <-l.actExit:
			l.handleActivityExit(e)
		case err := <-l.powerLost:
			l.handlePowerLost(err)
		case <-timerC(l.powerRetry):
			l.reacquirePower()
		}
	}
}

// deadlinePassed checks the wall clock and re-arms the timer if it fired
// early (e.g. the wall clock was adjusted after start).
func (l *loop) deadlinePassed() bool {
	if l.snap.EndsAt.IsZero() {
		return false
	}
	now := l.clk.Now()
	if !now.Before(l.snap.EndsAt) {
		return true
	}
	l.deadline.Reset(l.snap.EndsAt.Sub(now))
	return false
}

func (l *loop) timedReason() Reason {
	if l.snap.Mode == ModeUntil {
		return ReasonUntil
	}
	return ReasonDuration
}

// stop tears the running session down: simulator first, then the power hold.
func (l *loop) stop(reason Reason, err error) Result {
	l.emit(EventStopping, reason, "")
	if l.deadline != nil {
		l.deadline.Stop()
	}
	l.heartbeat.Stop()
	if l.batteryTick != nil {
		l.batteryTick.Stop()
	}
	if l.powerRetry != nil {
		l.powerRetry.Stop()
	}
	if l.schedTimer != nil {
		l.schedTimer.Stop()
	}
	l.stopActivity()
	l.waitForActivity()

	if rerr := l.releaseHold(); rerr != nil && err == nil {
		err = fmt.Errorf("release power hold: %w", rerr)
	}
	return l.finish(reason, err)
}

// finish publishes the final event and closes the stream.
func (l *loop) finish(reason Reason, err error) Result {
	l.s.running.Store(false)
	l.snap.Running = false
	l.snap.Activity = activity.Status{State: activity.StateOff}
	if l.cancelWatch != nil {
		l.cancelWatch()
	}
	msg := stopMessage(reason, l.snap.Battery)
	if l.stopMsg != "" {
		msg = l.stopMsg
	}
	if reason == ReasonError && err != nil {
		msg = err.Error()
	}
	slog.Info("session: stopped", "reason", reason, "err", err)
	ev := l.emit(EventStopped, reason, msg)
	l.s.bus.close()
	switch reason {
	case ReasonUser, ReasonSignal, ReasonIPC:
	default:
		l.notify(notifyStopped, "Keep-Alive stopped", msg)
	}
	return Result{Reason: reason, Err: err, StartedAt: l.snap.StartedAt, EndedAt: ev.Time}
}

func stopMessage(r Reason, b Battery) string {
	switch r {
	case ReasonUser:
		return "stopped by user"
	case ReasonDuration:
		return "duration reached"
	case ReasonUntil:
		return "end time reached"
	case ReasonBattery:
		return fmt.Sprintf("battery at %d%% (threshold %d%%)", b.Percent, b.Threshold)
	case ReasonSignal:
		return "interrupted"
	case ReasonIPC:
		return `stopped by "keepalive stop"`
	case ReasonCommandExited:
		return "command exited"
	case ReasonProcessExited:
		return "watched process exited"
	case ReasonError:
		return "error"
	default:
		return string(r)
	}
}

func (l *loop) snapshot() Snapshot {
	snap := l.snap
	if snap.Running && !snap.EndsAt.IsZero() {
		snap.Remaining = max(snap.EndsAt.Sub(l.clk.Now()), 0)
	}
	return snap
}

func (l *loop) emit(typ EventType, reason Reason, msg string) Event {
	snap := l.snapshot()
	l.s.last.Store(&snap)
	ev := Event{Time: l.clk.Now(), Type: typ, Snapshot: snap, Message: msg, Reason: reason}
	l.s.bus.publish(ev)
	return ev
}

func (l *loop) warn(msg string) {
	slog.Warn("session: " + msg)
	l.emit(EventWarning, "", msg)
}

// ---- runtime controls ----

func (l *loop) extend(d time.Duration) {
	if l.snap.EndsAt.IsZero() {
		l.warn("extend ignored: the session has no end time")
		return
	}
	now := l.clk.Now()
	end := l.snap.EndsAt.Add(d)
	if d < 0 {
		if floor := now.Add(MinRemainingAfterShorten); end.Before(floor) {
			end = floor
		}
		if end.After(l.snap.EndsAt) {
			end = l.snap.EndsAt
		}
	}
	l.snap.EndsAt = end
	l.deadline.Reset(end.Sub(now))
	l.emit(EventSnapshot, "", "")
}

func (l *loop) setActive(on bool) {
	if on == l.snap.Active && (!on || l.act != nil || !l.inWindow) {
		return
	}
	l.snap.Active = on
	if on {
		if l.inWindow { // outside the work hours it starts on entering them
			l.startActivity()
		}
		l.emit(EventSnapshot, "", "")
		return
	}
	l.stopActivity()
	l.snap.Activity = activity.Status{State: activity.StateOff}
	l.emit(EventActivity, "", "")
	l.emit(EventSnapshot, "", "")
}

// ---- activity ----

func (l *loop) startActivity() {
	if l.act != nil {
		return
	}
	sim := l.s.deps.Activity
	if sim == nil {
		l.snap.Activity = activity.Status{State: activity.StateDegraded, Reason: "activity simulation is not available"}
		l.warn("activity simulation is not available")
		l.notifyDegraded(l.snap.Activity)
		return
	}
	l.actGen++
	gen := l.actGen
	ctx, cancel := context.WithCancel(l.ctx)
	l.act = &activityRun{gen: gen, cancel: cancel}
	l.actRunning++
	cfg, done := l.s.cfg.Activity, l.s.done
	report := func(st activity.Status) {
		select {
		case l.actStatus <- activityMsg{gen: gen, status: st}:
		case <-ctx.Done():
		case <-done:
		}
	}
	go func() {
		err := sim.Run(ctx, cfg, report)
		select {
		case l.actExit <- activityExit{gen: gen, err: err}:
		case <-done:
		}
	}()
}

func (l *loop) stopActivity() {
	if l.act == nil {
		return
	}
	l.act.cancel()
	l.act = nil
}

// waitForActivity waits (bounded) for every simulator goroutine to return so
// none still uses the platform when the power hold is released.
func (l *loop) waitForActivity() {
	if l.actRunning == 0 {
		return
	}
	timeout := time.NewTimer(ActivityStopTimeout)
	defer timeout.Stop()
	for l.actRunning > 0 {
		select {
		case <-l.actExit:
			l.actRunning--
		case <-l.actStatus:
		case <-timeout.C:
			slog.Warn("session: activity simulator did not stop in time", "running", l.actRunning)
			return
		}
	}
}

func (l *loop) handleActivityStatus(m activityMsg) {
	if l.act == nil || m.gen != l.act.gen {
		return
	}
	prev := l.snap.Activity
	l.snap.Activity = m.status
	if sameActivity(prev, m.status) {
		return
	}
	l.emit(EventActivity, "", "")
	if m.status.State == activity.StateDegraded && prev.State != activity.StateDegraded {
		l.notifyDegraded(m.status)
	}
}

func (l *loop) handleActivityExit(e activityExit) {
	l.actRunning--
	if l.act == nil || e.gen != l.act.gen {
		return
	}
	// The simulator returned on its own.
	l.act.cancel()
	l.act = nil
	reason := "activity simulation stopped unexpectedly"
	if e.err != nil {
		reason = fmt.Sprintf("activity simulation failed: %v", e.err)
	}
	l.snap.Activity = activity.Status{State: activity.StateDegraded, Reason: reason}
	l.warn(reason)
	l.emit(EventActivity, "", "")
	l.notifyDegraded(l.snap.Activity)
}

// sameActivity compares statuses ignoring the constantly changing idle time.
func sameActivity(a, b activity.Status) bool {
	return a.State == b.State && a.Method == b.Method && a.Reason == b.Reason &&
		a.Hint == b.Hint && a.LastBurst.Equal(b.LastBurst)
}

// ---- power hold loss ----

// powerRetryBackoff is the wait before each re-acquire attempt after the
// power hold is lost; the last value repeats.
var powerRetryBackoff = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 30 * time.Second}

func (l *loop) powerOptions() power.Options {
	return power.Options{KeepDisplay: l.s.cfg.KeepDisplay, Reason: "keeping the system awake"}
}

// watchPower watches the current hold if the OS can take it away.
func (l *loop) watchPower() {
	l.powerLost = nil
	if w, ok := l.hold.(power.Watcher); ok {
		l.powerLost = w.Lost()
	}
}

// handlePowerLost keeps the lost hold (whatever is left of it stays held
// until a new one replaces it) and starts re-acquiring.
func (l *loop) handlePowerLost(err error) {
	l.powerLost = nil
	l.powerRetries = 0
	l.snap.PowerHold = ""
	l.warn(fmt.Sprintf("power hold lost (%v); re-acquiring", err))
	l.schedulePowerRetry()
}

func (l *loop) schedulePowerRetry() {
	d := powerRetryBackoff[min(l.powerRetries, len(powerRetryBackoff)-1)]
	l.powerRetries++
	if l.powerRetry == nil {
		l.powerRetry = l.clk.NewTimer(d)
		return
	}
	l.powerRetry.Reset(d)
}

func (l *loop) reacquirePower() {
	if !l.inWindow {
		return
	}
	hold, err := l.s.deps.Power.Acquire(l.ctx, l.powerOptions())
	if err != nil {
		slog.Warn("session: re-acquiring the power hold failed", "attempt", l.powerRetries, "err", err)
		if l.powerRetries == 1 {
			l.notify(notifyPower, "Keep-Alive: cannot keep the system awake",
				fmt.Sprintf("the sleep prevention was lost and could not be restored (%v); still retrying", err))
		}
		l.schedulePowerRetry()
		return
	}
	if l.hold != nil {
		if rerr := l.hold.Release(); rerr != nil {
			slog.Warn("session: releasing the lost power hold failed", "err", rerr)
		}
	}
	l.hold = hold
	l.watchPower()
	l.snap.PowerHold = hold.Describe()
	l.warn(fmt.Sprintf("power hold re-acquired (%s)", l.snap.PowerHold))
}

// ---- battery ----

func (l *loop) pollBattery() {
	if l.batteryBusy || l.s.deps.Battery == nil {
		return
	}
	l.batteryBusy = true
	read, done := l.s.deps.Battery, l.s.done
	go func() {
		st, err := read()
		select {
		case l.batteryResults <- batteryResult{status: st, err: err}:
		case <-done:
		}
	}()
}

// handleBattery records a poll result and reports whether the threshold is
// reached.
func (l *loop) handleBattery(r batteryResult) bool {
	l.batteryBusy = false
	if r.err != nil {
		l.snap.Battery.Available = false
		l.emit(EventBattery, "", fmt.Sprintf("battery status unavailable: %v", r.err))
		if !l.batteryFailing {
			l.warn(fmt.Sprintf("battery status unavailable: %v", r.err))
		}
		l.batteryFailing = true
		return false
	}
	l.batteryFailing = false
	l.snap.Battery.Percent = r.status.Percentage
	l.snap.Battery.Available = r.status.Available
	l.emit(EventBattery, "", "")
	return l.snap.Battery.Threshold > 0 && l.snap.Battery.Available && l.snap.Battery.Percent <= l.snap.Battery.Threshold
}

func timerC(t clock.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C()
}

func tickerC(t clock.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C()
}

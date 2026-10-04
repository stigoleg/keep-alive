package activity

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/clock"
)

// backend is what the OS offers the controller. Each OS builds one in
// newBackend.
type backend struct {
	// idle gates simulation; nil when the desktop exposes no idle time.
	idle IdleSource
	// verify checks bursts; nil means idle.
	verify IdleSource
	// sources is every idle source that could be read, for Probe.
	sources []IdleSource
	// secondary is a counter some apps read that bursts may not reach
	// (XWayland's on a Wayland session). It never gates or verifies; when it
	// misses bursts that verify saw, secondaryNote is shown.
	secondary     IdleSource
	secondaryNote string
	lock          LockSource
	// open picks the input backend.
	open func() (Injector, error)
	// noIdleHint explains how to get idle-aware simulation.
	noIdleHint string
	// candidates lists the input methods open tries, unopened, for
	// Diagnose; nil when open has no side effects.
	candidates func() []Injector
	// lockName describes the lock source for Diagnose.
	lockName string
	// env describes the desktop for Diagnose.
	env []string
	// notes warn about the desktop for Diagnose, as "name: text".
	notes []string
	// release frees OS resources (D-Bus connections).
	release func()
}

func (b *backend) Close() {
	if b.release != nil {
		b.release()
	}
}

type simulator struct{}

// Run simulates activity until ctx is done. It never returns early: when no
// input method works it reports degraded and keeps looking for one.
func (simulator) Run(ctx context.Context, cfg Config, report func(Status)) error {
	if cfg.IdleThreshold <= 0 {
		cfg.IdleThreshold = DefaultIdleThreshold
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if ctx.Err() != nil {
		return nil
	}
	b := newBackend(ctx, cfg.Keys)
	defer b.Close()
	c := newController(cfg, controllerDeps{
		clock:         clock.Real(),
		idle:          b.idle,
		verify:        b.verify,
		lock:          b.lock,
		open:          b.open,
		rnd:           newRand(),
		sleep:         realSleep,
		noIdleHint:    b.noIdleHint,
		secondary:     b.secondary,
		secondaryHint: b.secondaryNote,
	}, report)
	return c.run(ctx)
}

func newRand() *rand.Rand { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }

// ProbeResult is what one verified burst showed, for `keepalive doctor
// --probe`.
type ProbeResult struct {
	// Method is the input backend used; empty when none works.
	Method string
	// Reason and Hint explain a missing backend, a failed burst, or a burst
	// that did not reset the idle counter.
	Reason, Hint string
	// Sources holds every idle counter before and after the burst.
	Sources []ProbeReading
	// Verifier names the source the burst is judged by (the one chat apps
	// read); empty when the desktop exposes no idle time.
	Verifier string
	// Locked is the session lock state; LockKnown is false when it cannot
	// be read.
	Locked, LockKnown bool
	// Burst is how long the movement took.
	Burst time.Duration
	// Effective reports that the verifier dropped below 1.5 s.
	Effective bool
}

// ProbeReading is one idle counter around the probe burst.
type ProbeReading struct {
	Source        string
	Before, After time.Duration
	Err           string
	// Note is set when this counter missed a burst the verifier saw and
	// that matters, e.g. for apps running under XWayland.
	Note string
}

// Probe reads every idle source, plays one burst (the pointer visibly
// moves), reads them again and reports whether the burst registered as user
// input.
func Probe(ctx context.Context, keys bool) ProbeResult {
	b := newBackend(ctx, keys)
	defer b.Close()
	return probe(ctx, b, keys, newRand(), realSleep)
}

func probe(ctx context.Context, b *backend, keys bool, r *rand.Rand, sleep sleepFunc) ProbeResult {
	var res ProbeResult
	if b.lock != nil {
		locked, err := b.lock.Locked()
		res.LockKnown = err == nil
		res.Locked = err == nil && locked
	}
	verify := b.verify
	if verify == nil {
		verify = b.idle
	}
	if verify != nil {
		res.Verifier = verify.Name()
	}
	res.Sources = make([]ProbeReading, len(b.sources))
	for i, s := range b.sources {
		res.Sources[i].Source = s.Name()
		d, err := s.Idle()
		res.Sources[i].Before = d
		if err != nil {
			res.Sources[i].Err = err.Error()
		}
	}

	inj, err := b.open()
	if err != nil {
		res.Reason, res.Hint = explain(err)
		return res
	}
	defer inj.Close()
	res.Method = inj.Name()

	start := time.Now()
	err = playBurst(ctx, inj, NewPath(r), sleep)
	if err == nil && keys {
		if terr := inj.Tap(); terr != nil {
			res.Reason = fmt.Sprintf("key tap failed: %v", terr)
		}
	}
	res.Burst = time.Since(start)
	if err != nil {
		_, res.Hint = inj.Diagnose()
		res.Reason = fmt.Sprintf("burst failed: %v", err)
		return res
	}
	_ = sleep(ctx, settleDelay)

	for i, s := range b.sources {
		d, err := s.Idle()
		res.Sources[i].After = d
		if err != nil {
			res.Sources[i].Err = err.Error()
		}
	}
	if verify != nil {
		d, err := verify.Idle()
		res.Effective = err == nil && d < effectiveIdle
		if !res.Effective && res.Reason == "" {
			res.Reason, res.Hint = inj.Diagnose()
		}
	}
	if res.Effective && b.secondary != nil {
		for i := range res.Sources {
			rd := &res.Sources[i]
			if rd.Source == b.secondary.Name() && rd.Err == "" && rd.Before >= effectiveIdle && rd.After >= effectiveIdle {
				rd.Note = b.secondaryNote
			}
		}
	}
	return res
}

package activity

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/stigoleg/keep-alive/v2/internal/legacy"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
)

// legacySimulator drives the v1 per-OS jitter loop through the shared legacy
// object. It is replaced in phase 3; until then the idle threshold, interval
// and key presses are the v1 constants, whatever Config says.
type legacySimulator struct{}

func newLegacy() Simulator { return legacySimulator{} }

func (legacySimulator) Run(ctx context.Context, cfg Config, report func(Status)) error {
	avail := platform.GetActivitySimulationStatus()
	if !avail.Available {
		report(Status{State: StateDegraded, Reason: avail.Message})
		<-ctx.Done()
		return nil
	}

	if err := legacy.Acquire(); err != nil {
		return err
	}
	defer func() {
		if err := legacy.Release(); err != nil {
			slog.Warn("activity: legacy release failed", "err", err)
		}
	}()

	base := Status{Method: avail.Method, Hint: legacyHint(cfg)}
	report(legacyStatus(base, platform.ActivityEvent{Kind: platform.ActivityWaitingIdle}))

	cancel := legacy.Observe(func(ev platform.ActivityEvent) { report(legacyStatus(base, ev)) })
	defer cancel()
	legacy.SetActivity(true)
	defer legacy.SetActivity(false)

	<-ctx.Done()
	return nil
}

func legacyHint(cfg Config) string {
	if cfg.IdleThreshold == platform.IdleThreshold && cfg.Interval == platform.ChatAppActivityInterval && !cfg.Keys {
		return ""
	}
	slog.Warn("activity: legacy engine ignores custom idle/interval/keys settings",
		"idle", cfg.IdleThreshold, "interval", cfg.Interval, "keys", cfg.Keys)
	return fmt.Sprintf("custom --active-idle/--active-interval/--active-keys are not applied yet (using %s idle, %s interval)",
		platform.IdleThreshold, platform.ChatAppActivityInterval)
}

func legacyStatus(base Status, ev platform.ActivityEvent) Status {
	st := base
	st.Idle = ev.Idle
	switch ev.Kind {
	case platform.ActivityIdleUnknown:
		st.State = StateDegraded
		st.Reason = fmt.Sprintf("cannot read idle time: %v", ev.Err)
	case platform.ActivityUserReturned:
		st.State = StatePausedUser
		st.Reason = "you are using the computer"
	case platform.ActivityBurst:
		st.State = StateSimulating
		st.LastBurst = ev.At
	default:
		st.State = StateWaitingIdle
		st.Reason = fmt.Sprintf("needs %s", platform.IdleThreshold)
	}
	return st
}

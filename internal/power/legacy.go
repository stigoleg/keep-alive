package power

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/stigoleg/keep-alive/v2/internal/legacy"
)

// legacyInhibitor adapts the v1 platform keep-alive. It is replaced in phase
// 2; until then the display is always kept on, whatever Options.KeepDisplay
// says.
type legacyInhibitor struct{}

func newLegacy() Inhibitor { return legacyInhibitor{} }

func (legacyInhibitor) Name() string { return "legacy" }

func (legacyInhibitor) Acquire(ctx context.Context, o Options) (Hold, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := legacy.Acquire(); err != nil {
		return nil, err
	}
	desc := legacy.Describe()
	if !o.KeepDisplay {
		slog.Warn("power: legacy engine keeps the display on; keep_display=false is not applied yet")
		desc += " (display kept on)"
	}
	slog.Debug("power: acquired", "mechanism", desc, "reason", o.Reason)
	return &legacyHold{desc: desc}, nil
}

var errReleased = errors.New("power: hold already released")

type legacyHold struct {
	once sync.Once
	desc string
}

func (h *legacyHold) Release() error {
	err := errReleased
	h.once.Do(func() {
		err = legacy.Release()
		slog.Debug("power: released", "mechanism", h.desc, "err", err)
	})
	return err
}

func (h *legacyHold) Describe() string { return h.desc }

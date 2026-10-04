package tui

import (
	"context"
	"sync"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// requestTimeout bounds one request to an attached keepalive.
const requestTimeout = 5 * time.Second

// ipcController follows a keepalive running in another process through its
// control socket.
type ipcController struct {
	c      *ipc.Client
	inst   Instance
	cancel context.CancelFunc
	events chan session.Event

	mu   sync.Mutex
	last session.Snapshot
}

// Attach follows the keepalive behind c. It subscribes before reading the
// status, so no event in between is lost. The subscription lives until ctx
// is cancelled or the controller is closed.
func Attach(ctx context.Context, c *ipc.Client) (Controller, error) {
	sctx, cancel := context.WithCancel(ctx)
	sub, err := c.Subscribe(sctx)
	if err != nil {
		cancel()
		return nil, err
	}
	rctx, rcancel := context.WithTimeout(ctx, requestTimeout)
	defer rcancel()
	st, err := c.Status(rctx)
	if err != nil {
		cancel()
		return nil, err
	}
	ic := &ipcController{
		c:      c,
		inst:   Instance{PID: st.PID, Origin: st.Origin, Version: st.Version},
		cancel: cancel,
		events: make(chan session.Event, 16),
		last:   output.SnapshotFromJSON(st.Snapshot),
	}
	go func() {
		defer close(ic.events)
		for je := range sub {
			ev := output.EventFromJSON(je.JSONEvent)
			ic.setLast(ev.Snapshot)
			select {
			case ic.events <- ev:
			case <-sctx.Done():
				return
			}
		}
	}()
	return ic, nil
}

func (ic *ipcController) cached() session.Snapshot {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return ic.last
}

func (ic *ipcController) setLast(s session.Snapshot) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.last = s
}

// Snapshot asks the instance for its state; on failure it returns the last
// state seen.
func (ic *ipcController) Snapshot() (session.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	st, err := ic.c.Status(ctx)
	if err != nil {
		return ic.cached(), err
	}
	snap := output.SnapshotFromJSON(st.Snapshot)
	ic.setLast(snap)
	return snap, nil
}

func (ic *ipcController) Events() <-chan session.Event { return ic.events }
func (ic *ipcController) Attached() *Instance          { return &ic.inst }

func (ic *ipcController) SetActive(on bool) error {
	return ic.request(func(ctx context.Context) error { return ic.c.SetActive(ctx, on) })
}

func (ic *ipcController) Extend(d time.Duration) error {
	return ic.request(func(ctx context.Context) error { return ic.c.Extend(ctx, d) })
}

func (ic *ipcController) Stop() error {
	return ic.request(func(ctx context.Context) error { return ic.c.Stop(ctx) })
}

// Close detaches; the instance keeps running.
func (ic *ipcController) Close() error {
	ic.cancel()
	return nil
}

func (ic *ipcController) request(fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return fn(ctx)
}

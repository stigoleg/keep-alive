package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/session"
	"github.com/stigoleg/keep-alive/v2/internal/tui"
)

// replaceTimeout bounds how long --replace waits for the other instance.
const replaceTimeout = 10 * time.Second

// claimInstance makes this process the running keepalive by taking the
// single-instance lock and listening on the control socket; call it before
// acquiring power. A nil server without error means `keepalive run` runs
// its command without control (next to another instance, or without a
// socket). Any other keepalive that cannot create the socket does not run:
// "keepalive status" and "keepalive stop" could not reach it.
func (a *App) claimInstance(ctx context.Context, p *Plan) (*ipc.Server, error) {
	info := ipc.ServerInfo{Version: a.Version, Origin: p.Origin}
	srv, err := ipc.Listen(info)
	if err == nil {
		return srv, nil
	}
	if !errors.Is(err, ipc.ErrAlreadyRunning) {
		slog.Warn("control socket unavailable", "err", err)
		switch p.Origin {
		case originRun:
			return nil, nil
		case ipc.OriginService:
			// Restarting cannot fix the directory: say so once and exit 0,
			// so the service manager does not restart us in a loop.
			ue := unreachableErr(err)
			printError(a.Stderr, ue, false)
			msg := failureMessage(ue)
			slog.Error("service: could not start", "err", msg)
			if p.Notify {
				a.notifyService("service-start", "Keep-Alive service could not start", msg)
			}
			return nil, &ExitError{Code: ExitOK}
		}
		return nil, unreachableErr(err)
	}
	if p.Origin == originRun {
		slog.Debug("another keepalive holds the control socket; running without it", "err", err)
		return nil, nil
	}
	other := describeOther(ctx, err)
	if p.Origin == ipc.OriginService {
		// Exit 0 so the service manager does not restart us in a loop.
		slog.Info("service: another keepalive is already running; exiting", "other", other)
		return nil, &ExitError{Code: ExitOK}
	}
	if !p.Replace {
		return nil, &ExitError{
			Code: ExitFailure,
			Err:  fmt.Errorf("another keepalive is already running (%s)", other),
			Hint: `use "keepalive status", "keepalive stop", or start with --replace`,
		}
	}
	return a.replace(ctx, info, other)
}

// unreachableErr is why a keepalive without a control socket does not run.
func unreachableErr(err error) *ExitError {
	return &ExitError{
		Code: ExitFailure,
		Err:  fmt.Errorf(`cannot create the control socket that "keepalive status" and "keepalive stop" use: %w`, err),
		Hint: "fix that directory, or set " + ipc.EnvRuntimeDir + " to a directory only you can use",
	}
}

// describeOther is "pid 123, started by service" for the instance holding the
// lock.
func describeOther(ctx context.Context, err error) string {
	pid := 0
	var are *ipc.AlreadyRunningError
	if errors.As(err, &are) {
		pid = are.PID
	}
	origin := ""
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if c, derr := ipc.Dial(ctx); derr == nil {
		if st, serr := c.Status(ctx); serr == nil {
			pid, origin = st.PID, st.Origin
		}
	}
	switch {
	case pid > 0 && origin != "":
		return fmt.Sprintf("pid %d, started by %s", pid, origin)
	case pid > 0:
		return fmt.Sprintf("pid %d", pid)
	case origin != "":
		return "started by " + origin
	}
	return "pid unknown"
}

// replace asks the running instance to stop and takes the lock once it is
// free.
func (a *App) replace(ctx context.Context, info ipc.ServerInfo, other string) (*ipc.Server, error) {
	if c, err := ipc.Dial(ctx); err == nil {
		if err := c.Stop(ctx); err != nil {
			slog.Warn("replace: stop request failed", "err", err)
		}
	}
	deadline := time.Now().Add(replaceTimeout)
	for {
		srv, err := ipc.Listen(info)
		if err == nil {
			fmt.Fprintf(a.Stderr, "keepalive: replaced the running keepalive (%s)\n", other)
			return srv, nil
		}
		if !errors.Is(err, ipc.ErrAlreadyRunning) {
			return nil, runtimeErr(err, "")
		}
		if time.Now().After(deadline) {
			return nil, runtimeErr(fmt.Errorf("the running keepalive (%s) did not stop within %s", other, replaceTimeout),
				`try "keepalive stop", or end that process`)
		}
		select {
		case <-ctx.Done():
			return nil, runtimeErr(ctx.Err(), "")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// tuiInstance is the interactive UI's claim on being the running keepalive.
// It is taken when the UI starts, or, while another keepalive runs, once
// that one has stopped; until then the UI follows the other one.
type tuiInstance struct {
	ctx  context.Context
	info ipc.ServerInfo
	ctrl *switchController

	mu    sync.Mutex // held for a whole claim, so two never race
	held  bool
	stop  func() // stops serving the control socket
	calls int    // claims so far; only the first, before the UI, may print
}

// adopt takes srv (nil: run without a control socket) as this UI's claim.
func (t *tuiInstance) adopt(srv *ipc.Server) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.adoptLocked(srv)
}

func (t *tuiInstance) adoptLocked(srv *ipc.Server) {
	t.held = true
	if srv != nil {
		t.stop = serve(t.ctx, srv, t.ctrl)
	}
}

// claim makes this process the running keepalive. When another one runs a
// session, it returns a controller attached to it; when another one holds
// the claim without a session, or cannot be reached, an error with a fix.
// Without a control socket the first claim, made before the UI starts,
// returns an *ExitError.
func (t *tuiInstance) claim() (tui.Controller, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.held {
		return nil, nil
	}
	t.calls++
	srv, err := ipc.Listen(t.info)
	if errors.Is(err, ipc.ErrAlreadyRunning) {
		return attachOther(t.ctx, err)
	}
	if err != nil {
		slog.Warn("control socket unavailable", "err", err)
		ue := unreachableErr(err)
		if t.calls == 1 {
			return nil, ue // before the UI: keepalive exits with it
		}
		return nil, tui.Warning{Text: ue.Err.Error(), Fix: ue.Hint}
	}
	t.adoptLocked(srv)
	return nil, nil
}

// attachOther follows the keepalive holding the lock (lockErr) when it runs
// a session.
func attachOther(ctx context.Context, lockErr error) (tui.Controller, error) {
	other := describeOther(ctx, lockErr)
	running := tui.Warning{
		Text: fmt.Sprintf("Another keepalive is already running (%s).", other),
		Fix:  `stop it with "keepalive stop", or start with --replace`,
	}
	dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, err := ipc.Dial(dctx)
	if err != nil {
		return nil, running
	}
	st, err := c.Status(dctx)
	if err != nil {
		return nil, running
	}
	if !st.Snapshot.Running {
		return nil, tui.Warning{
			Text: fmt.Sprintf("Another keepalive is open (%s) and keeps nothing awake.", other),
			Fix:  `start from that one or quit it, then press enter here; or run "keepalive stop"`,
		}
	}
	ctrl, err := tui.Attach(ctx, c)
	if err != nil {
		return nil, running
	}
	return ctrl, nil
}

// close stops serving the control socket.
func (t *tuiInstance) close() {
	t.mu.Lock()
	stop := t.stop
	t.stop = nil
	t.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// serve answers control requests for c until the returned func is called.
func serve(ctx context.Context, srv *ipc.Server, c ipc.Controller) (stop func()) {
	if srv == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(ctx, c); err != nil {
			slog.Warn("control socket stopped", "err", err)
		}
	}()
	return func() {
		if err := srv.Close(); err != nil {
			slog.Debug("closing the control socket", "err", err)
		}
		<-done
	}
}

// switchController is the control socket's view of the TUI: it forwards to
// whichever session the TUI is running. Without one, Stop quits the TUI.
type switchController struct {
	mu   sync.Mutex
	cur  *session.Session
	quit func()
}

func (c *switchController) set(s *session.Session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = s
}

func (c *switchController) get() *session.Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur
}

func (c *switchController) Snapshot() session.Snapshot {
	if s := c.get(); s != nil {
		return s.Snapshot()
	}
	return session.Snapshot{}
}

func (c *switchController) Stop(r session.Reason) {
	if s := c.get(); s != nil {
		s.Stop(r)
		return
	}
	c.mu.Lock()
	quit := c.quit
	c.mu.Unlock()
	if quit != nil {
		quit()
	}
}

func (c *switchController) SetActive(on bool) {
	if s := c.get(); s != nil {
		s.SetActive(on)
	}
}

func (c *switchController) Extend(d time.Duration) {
	if s := c.get(); s != nil {
		s.Extend(d)
	}
}

func (c *switchController) Subscribe() (<-chan session.Event, func()) {
	if s := c.get(); s != nil {
		return s.Subscribe()
	}
	ch := make(chan session.Event)
	close(ch)
	return ch, func() {}
}

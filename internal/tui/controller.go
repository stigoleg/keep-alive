package tui

import (
	"context"
	"sync"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// stopTimeout bounds how long stopping a session waits for the power hold
// to be released.
const stopTimeout = 10 * time.Second

// Controller is the dashboard's handle on a running session: one this
// process runs, or one in another keepalive process (attached). The
// dashboard renders the same way for both.
type Controller interface {
	// Snapshot returns the current state.
	Snapshot() (session.Snapshot, error)
	// Events streams the session's events; it is closed once the session
	// has ended or the connection to it is lost.
	Events() <-chan session.Event
	SetActive(on bool) error
	// Extend moves the end of a timed session; negative shortens it.
	Extend(d time.Duration) error
	// Stop ends the session: a local one is stopped and its power hold
	// released; an attached one is asked to stop.
	Stop() error
	// Close stops following the session: a local session is stopped, an
	// attached one keeps running.
	Close() error
	// Attached describes the other process for an attached session; nil for
	// a local one.
	Attached() *Instance
}

// Instance is a keepalive running in another process.
type Instance struct {
	PID     int
	Origin  string // "terminal", "service" or "run"
	Version string
}

// localController runs a session in this process.
type localController struct {
	sess   *session.Session
	events <-chan session.Event
	unsub  func()
	done   chan struct{}
	res    session.Result
	once   sync.Once
}

func startLocal(ctx context.Context, cfg session.Config, deps session.Deps) *localController {
	s := session.New(cfg, deps)
	events, unsub := s.Subscribe()
	c := &localController{sess: s, events: events, unsub: unsub, done: make(chan struct{})}
	go func() {
		c.res = s.Run(ctx)
		close(c.done)
	}()
	return c
}

func (c *localController) Snapshot() (session.Snapshot, error) { return c.sess.Snapshot(), nil }
func (c *localController) Events() <-chan session.Event        { return c.events }
func (c *localController) Attached() *Instance                 { return nil }

func (c *localController) SetActive(on bool) error {
	c.sess.SetActive(on)
	return nil
}

func (c *localController) Extend(d time.Duration) error {
	c.sess.Extend(d)
	return nil
}

// Stop ends the session and waits until it has released the power hold.
func (c *localController) Stop() error {
	c.sess.Stop(session.ReasonUser)
	select {
	case <-c.done:
		return c.res.Err
	case <-time.After(stopTimeout):
		return context.DeadlineExceeded
	}
}

func (c *localController) Close() error {
	err := c.Stop()
	c.once.Do(c.unsub)
	return err
}

// slot holds the controller the UI currently follows. It is shared by every
// copy of the model, so Shutdown can stop whatever is running when the
// program has exited.
type slot struct {
	mu        sync.Mutex
	cur       Controller
	onSession func(*session.Session) // Options.OnSession
}

func (s *slot) set(c Controller) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = c
	if lc, ok := c.(*localController); ok && s.onSession != nil {
		s.onSession(lc.sess)
	}
}

// release forgets c (if it is current) and closes it.
func (s *slot) release(c Controller) error {
	s.mu.Lock()
	if s.cur == c {
		s.cur = nil
		if _, ok := c.(*localController); ok && s.onSession != nil {
			s.onSession(nil)
		}
	}
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	return c.Close()
}

func (s *slot) current() Controller {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

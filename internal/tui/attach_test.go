package tui

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// ipcFake is the session behind a real control socket.
type ipcFake struct {
	mu     sync.Mutex
	snap   session.Snapshot
	calls  []string
	events chan session.Event
}

func (f *ipcFake) Snapshot() session.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}

func (f *ipcFake) record(c string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
}

func (f *ipcFake) Stop(r session.Reason) { f.record("stop " + string(r)) }
func (f *ipcFake) SetActive(on bool) {
	f.record(map[bool]string{true: "active on", false: "active off"}[on])
}
func (f *ipcFake) Extend(d time.Duration) { f.record("extend " + d.String()) }
func (f *ipcFake) Subscribe() (<-chan session.Event, func()) {
	return f.events, func() {}
}

func TestAttachThroughTheControlSocket(t *testing.T) {
	snap := timedSnap()
	snap.Activity = activity.Status{State: activity.StateSimulating, Method: "CoreGraphics"}
	f := &ipcFake{snap: snap, events: make(chan session.Event, 4)}
	srv, err := ipc.Listen(ipc.ServerInfo{Version: "2.0.0", Origin: ipc.OriginService})
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = srv.Serve(context.Background(), f)
	}()
	t.Cleanup(func() {
		srv.Close()
		<-served
	})

	ctx := context.Background()
	c, err := ipc.Dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctrl, err := Attach(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()

	if inst := ctrl.Attached(); inst == nil || inst.PID != os.Getpid() || inst.Origin != ipc.OriginService || inst.Version != "2.0.0" {
		t.Fatalf("attached %+v", inst)
	}
	got, err := ctrl.Snapshot()
	if err != nil || !got.Running || got.Activity.State != activity.StateSimulating || !got.EndsAt.Equal(snap.EndsAt) {
		t.Fatalf("snapshot %+v, %v", got, err)
	}
	if err := ctrl.SetActive(false); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Extend(15 * time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Stop(); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	calls := append([]string(nil), f.calls...)
	f.mu.Unlock()
	if want := []string{"active off", "extend 15m0s", "stop ipc"}; len(calls) != 3 || calls[0] != want[0] || calls[1] != want[1] || calls[2] != want[2] {
		t.Fatalf("calls %v", calls)
	}

	f.events <- session.Event{Time: testNow, Type: session.EventWarning, Snapshot: snap, Message: "battery status unavailable"}
	stopped := snap
	stopped.Running = false
	f.events <- session.Event{Time: testNow, Type: session.EventStopped, Snapshot: stopped, Reason: session.ReasonDuration, Message: "duration reached"}
	close(f.events)
	var evs []session.Event
	timeout := time.After(3 * time.Second)
	for done := false; !done; {
		select {
		case ev, ok := <-ctrl.Events():
			if !ok {
				done = true
				break
			}
			evs = append(evs, ev)
		case <-timeout:
			t.Fatal("events did not end")
		}
	}
	if len(evs) != 2 || evs[0].Message != "battery status unavailable" || evs[1].Type != session.EventStopped ||
		evs[1].Reason != session.ReasonDuration || evs[1].Message != "duration reached" || evs[1].Snapshot.Running {
		t.Fatalf("events %+v", evs)
	}
}

func TestAttachedCloseDetaches(t *testing.T) {
	f := &ipcFake{snap: timedSnap(), events: make(chan session.Event)}
	srv, err := ipc.Listen(ipc.ServerInfo{Version: "2.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = srv.Serve(context.Background(), f)
	}()
	defer func() {
		srv.Close()
		<-served
	}()
	c, err := ipc.Dial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctrl, err := Attach(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-ctrl.Events():
		if ok {
			t.Fatal("event after close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("events not closed after Close")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 0 {
		t.Fatalf("close sent %v", f.calls)
	}
}

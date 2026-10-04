package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/session"
	"github.com/stigoleg/keep-alive/v2/internal/tui"
)

// fakeInstance serves the control socket for a fake session.
type fakeInstance struct {
	mu      sync.Mutex
	snap    session.Snapshot
	calls   []string
	srv     *ipc.Server
	stopped chan struct{}
}

func (f *fakeInstance) Snapshot() session.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}

func (f *fakeInstance) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeInstance) Stop(r session.Reason) {
	f.record("stop " + string(r))
	go f.srv.Close() // the instance exits
}

func (f *fakeInstance) SetActive(on bool) {
	f.record("active " + map[bool]string{true: "on", false: "off"}[on])
	f.mu.Lock()
	f.snap.Active = on
	f.mu.Unlock()
}

func (f *fakeInstance) Extend(d time.Duration) {
	f.record("extend " + d.String())
	f.mu.Lock()
	f.snap.EndsAt = f.snap.EndsAt.Add(d)
	f.snap.Remaining += d
	f.mu.Unlock()
}

func (f *fakeInstance) Subscribe() (<-chan session.Event, func()) {
	ch := make(chan session.Event, 4)
	ch <- session.Event{Time: time.Now(), Type: session.EventWarning, Snapshot: f.Snapshot(), Message: "test warning"}
	ch <- session.Event{Time: time.Now(), Type: session.EventStopped, Snapshot: f.Snapshot(), Reason: session.ReasonIPC, Message: "stopped"}
	close(ch)
	return ch, func() {}
}

func startInstance(t *testing.T, snap session.Snapshot) *fakeInstance {
	t.Helper()
	srv, err := ipc.Listen(ipc.ServerInfo{Version: "2.0.0", Origin: ipc.OriginService})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeInstance{snap: snap, srv: srv, stopped: make(chan struct{})}
	go func() {
		defer close(f.stopped)
		srv.Serve(context.Background(), f)
	}()
	t.Cleanup(func() {
		srv.Close()
		<-f.stopped
	})
	return f
}

func timedSnap() session.Snapshot {
	start := time.Now().Add(-time.Hour)
	return session.Snapshot{
		Running: true, StartedAt: start, EndsAt: start.Add(3 * time.Hour), Remaining: 2 * time.Hour,
		Mode: session.ModeDuration, Active: true, KeepDisplay: true, InWindow: true, PowerHold: "fake hold",
		Activity: activity.Status{State: activity.StateSimulating, Method: "fake"},
	}
}

func TestControlCommandsWithoutInstance(t *testing.T) {
	for _, args := range [][]string{{"status"}, {"status", "--json"}, {"stop"}, {"active", "on"}, {"extend", "10m"}} {
		ta := newTestApp(t)
		if code := ta.run(args...); code != ExitNoInstance {
			t.Errorf("%v: exit %d, want %d", args, code, ExitNoInstance)
		}
		if !strings.HasPrefix(ta.stderr.String(), "keepalive: error: no keepalive is running\nhint: ") {
			t.Errorf("%v: stderr %q", args, ta.stderr)
		}
	}
}

func TestStatus(t *testing.T) {
	startInstance(t, timedSnap())
	ta := newTestApp(t)
	ta.Now = time.Now
	if code := ta.run("status"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	out := ta.stdout.String()
	for _, want := range []string{"keepalive 2.0.0 · pid ", "started by the login service", "2h0m left", "simulating input via fake", "fake hold"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}

	ta = newTestApp(t)
	if code := ta.run("status", "--json"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	var got struct {
		PID      int    `json:"pid"`
		Origin   string `json:"origin"`
		Snapshot struct {
			Running   bool   `json:"running"`
			Remaining int64  `json:"remaining"`
			PowerHold string `json:"power_hold"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(ta.stdout.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, ta.stdout)
	}
	if got.PID == 0 || got.Origin != "service" || !got.Snapshot.Running || got.Snapshot.Remaining != 7200 || got.Snapshot.PowerHold != "fake hold" {
		t.Fatalf("status --json = %+v", got)
	}
}

func TestStatusFollow(t *testing.T) {
	startInstance(t, timedSnap())
	ta := newTestApp(t)
	if code := ta.run("status", "--follow"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	out := ta.stdout.String()
	if !strings.Contains(out, "warning: test warning") || !strings.Contains(out, "stopped: stopped") {
		t.Fatalf("follow output:\n%s", out)
	}

	ta = newTestApp(t)
	if code := ta.run("status", "--follow", "--json"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	lines := strings.Split(strings.TrimSpace(ta.stdout.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	for _, l := range lines {
		if !json.Valid([]byte(l)) {
			t.Fatalf("invalid JSON line %q", l)
		}
	}
	if !strings.Contains(lines[2], `"type":"stopped"`) || strings.Contains(lines[2], `"v":`) {
		t.Fatalf("last line = %s", lines[2])
	}
}

func TestActiveExtendStop(t *testing.T) {
	f := startInstance(t, timedSnap())
	ta := newTestApp(t)
	ta.Now = time.Now
	for _, args := range [][]string{{"active", "off"}, {"extend", "10m"}, {"extend", "-15m"}, {"extend", "30"}, {"stop"}} {
		ta.stdout.Reset()
		if code := ta.run(args...); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", args, code, ta.stderr)
		}
	}
	want := "active off,extend 10m0s,extend -15m0s,extend 30m0s,stop ipc"
	if got := strings.Join(f.calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
	if !strings.HasPrefix(ta.stdout.String(), "stopped keepalive (pid ") {
		t.Fatalf("stop printed %q", ta.stdout)
	}
}

func TestExtendOutput(t *testing.T) {
	startInstance(t, timedSnap())
	ta := newTestApp(t)
	ta.Now = time.Now
	if code := ta.run("extend", "1h"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if got := ta.stdout.String(); !strings.HasPrefix(got, "keepalive now runs until ") || !strings.Contains(got, "(3h0m left)") {
		t.Fatalf("extend printed %q", got)
	}
}

func TestExtendErrors(t *testing.T) {
	snap := timedSnap()
	snap.Mode, snap.EndsAt, snap.Remaining = session.ModeIndefinite, time.Time{}, 0
	f := startInstance(t, snap)
	ta := newTestApp(t)
	if code := ta.run("extend", "10m"); code != ExitFailure {
		t.Fatalf("indefinite: exit %d", code)
	}
	if !strings.Contains(ta.stderr.String(), "no end time") || !strings.Contains(ta.stderr.String(), "--replace -d 2h") {
		t.Fatalf("stderr = %q", ta.stderr)
	}
	for _, args := range [][]string{{"extend"}, {"extend", "soon"}, {"extend", "30s"}, {"active", "maybe"}} {
		ta := newTestApp(t)
		if code := ta.run(args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("calls = %v", f.calls)
	}
}

type recordingNotifier struct{ sent []string }

func (r *recordingNotifier) Notify(_ context.Context, title, body string) error {
	r.sent = append(r.sent, title+": "+body)
	return errors.New("not shown")
}

func TestServiceUsageErrorExitsZero(t *testing.T) {
	ta := newTestApp(t)
	n := &recordingNotifier{}
	var logged []logging.Options
	ta.Notifier = n
	ta.logSetup = func(o logging.Options) (string, func() error, error) {
		logged = append(logged, o)
		return "", func() error { return nil }, nil
	}
	if err := writeConfig(ta, "battery = 500\n"); err != nil {
		t.Fatal(err)
	}
	if code := ta.run("--plain", "--origin", "service"); code != ExitOK {
		t.Fatalf("exit %d, want 0", code)
	}
	if len(n.sent) != 1 || !strings.HasPrefix(n.sent[0], "Keep-Alive service could not start: ") || !strings.Contains(n.sent[0], "battery") {
		t.Fatalf("notifications = %v", n.sent)
	}
	if len(logged) != 1 || !logged[0].Enabled {
		t.Fatalf("logging = %+v", logged)
	}
	if !strings.Contains(ta.stderr.String(), "keepalive: error: ") {
		t.Fatalf("stderr = %q", ta.stderr)
	}

	// The same failure in a terminal is a usage error.
	ta = newTestApp(t)
	ta.Notifier = n
	writeConfig(ta, "battery = 500\n")
	if code := ta.run("--plain"); code != ExitUsage {
		t.Fatalf("terminal: exit %d", code)
	}
	if len(n.sent) != 1 {
		t.Fatalf("terminal run notified: %v", n.sent)
	}
}

func newTUIInstance() *tuiInstance {
	return &tuiInstance{ctx: context.Background(), info: ipc.ServerInfo{Version: "2.0.0"}, ctrl: &switchController{}, stderr: &strings.Builder{}}
}

func TestTUIAttachesToARunningSession(t *testing.T) {
	startInstance(t, timedSnap())
	inst := newTUIInstance()
	defer inst.close()
	c, err := inst.claim()
	if err != nil || c == nil {
		t.Fatalf("claim = %v, %v; want an attached controller", c, err)
	}
	defer c.Close()
	if a := c.Attached(); a == nil || a.Origin != ipc.OriginService {
		t.Fatalf("attached %+v", a)
	}
	if inst.held {
		t.Fatal("claimed while another keepalive runs")
	}
}

func TestTUIClaimNextToAnIdleUI(t *testing.T) {
	startInstance(t, session.Snapshot{}) // another UI at its menu
	inst := newTUIInstance()
	defer inst.close()
	c, err := inst.claim()
	var w tui.Warning
	if c != nil || !errors.As(err, &w) || !strings.Contains(w.Text, "keeps nothing awake") || w.Fix == "" {
		t.Fatalf("claim = %v, %v", c, err)
	}
}

func TestTUIClaimTakesTheLock(t *testing.T) {
	inst := newTUIInstance()
	c, err := inst.claim()
	if c != nil || err != nil || !inst.held {
		t.Fatalf("claim = %v, %v (held %v)", c, err, inst.held)
	}
	if _, err := ipc.Listen(ipc.ServerInfo{}); !errors.Is(err, ipc.ErrAlreadyRunning) {
		t.Fatalf("second Listen: %v", err)
	}
	if c, err := inst.claim(); c != nil || err != nil {
		t.Fatalf("second claim = %v, %v", c, err)
	}
	inst.close()
	srv, err := ipc.Listen(ipc.ServerInfo{})
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	srv.Close()
}

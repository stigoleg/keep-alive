package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		helperServer()
		return
	}
	os.Exit(m.Run())
}

// fakeController records commands and fans events out like a session.
type fakeController struct {
	mu      sync.Mutex
	snap    session.Snapshot
	stops   []session.Reason
	actives []bool
	extends []time.Duration
	subs    map[chan session.Event]struct{}
	ended   bool
}

func newFake() *fakeController {
	return &fakeController{
		snap: session.Snapshot{
			Running:   true,
			StartedAt: time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC),
			EndsAt:    time.Date(2026, 10, 4, 16, 0, 0, 0, time.UTC),
			Remaining: 8 * time.Hour,
			Mode:      session.ModeUntil,
			Active:    true,
			Activity:  activity.Status{State: activity.StateWaitingIdle, Method: "fake"},
			Battery:   session.Battery{Percent: 80, Available: true},
			PowerHold: "fake hold",
		},
		subs: map[chan session.Event]struct{}{},
	}
}

func (f *fakeController) Snapshot() session.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snap
}

func (f *fakeController) Stop(r session.Reason) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, r)
}

func (f *fakeController) SetActive(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actives = append(f.actives, on)
}

func (f *fakeController) Extend(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extends = append(f.extends, d)
}

func (f *fakeController) Subscribe() (<-chan session.Event, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan session.Event, 64)
	if f.ended {
		close(ch)
		return ch, func() {}
	}
	f.subs[ch] = struct{}{}
	return ch, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.subs[ch]; ok {
			delete(f.subs, ch)
			close(ch)
		}
	}
}

func (f *fakeController) publish(ev session.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for ch := range f.subs {
		ch <- ev
	}
}

func (f *fakeController) end() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ended = true
	for ch := range f.subs {
		close(ch)
	}
	f.subs = map[chan session.Event]struct{}{}
}

func (f *fakeController) subscribers() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

// runtimeDir points KEEPALIVE_RUNTIME_DIR at a short fresh directory (macOS
// limits socket paths to 104 bytes, and t.TempDir can be longer).
func runtimeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ka")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv(EnvRuntimeDir, dir)
	return dir
}

// noLeaks fails the test when goroutines started during it are still alive
// after cleanup.
func noLeaks(t *testing.T) {
	t.Helper()
	base := runtime.NumGoroutine()
	t.Cleanup(func() {
		deadline := time.Now().Add(3 * time.Second)
		for runtime.NumGoroutine() > base {
			if time.Now().After(deadline) {
				buf := make([]byte, 1<<16)
				t.Errorf("goroutine leak: %d running, %d before\n%s", runtime.NumGoroutine(), base, buf[:runtime.Stack(buf, true)])
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

var testInfo = ServerInfo{PID: 4242, Version: "2.0.0-test", Origin: OriginService}

// serve starts a server for c and stops it at cleanup.
func serve(t *testing.T, c Controller) *Server {
	t.Helper()
	srv, err := Listen(testInfo)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), c) }()
	t.Cleanup(func() {
		if err := srv.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Serve did not return after Close")
		}
	})
	return srv
}

func dial(t *testing.T) *Client {
	t.Helper()
	c, err := Dial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func ctxTimeout(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestStatusRoundTrip(t *testing.T) {
	runtimeDir(t)
	noLeaks(t)
	serve(t, newFake())

	st, err := dial(t).Status(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	if st.PID != 4242 || st.Version != "2.0.0-test" || st.Origin != OriginService {
		t.Fatalf("status = %+v", st)
	}
	snap := st.Snapshot
	if !snap.Running || snap.Mode != "until" || snap.Remaining == nil || *snap.Remaining != 8*3600 || !snap.Active ||
		snap.Activity.State != "waiting_idle" || snap.Battery.Percent != 80 || snap.PowerHold != "fake hold" {
		t.Fatalf("snapshot = %+v", snap)
	}
	if snap.EndsAt == nil || *snap.EndsAt != "2026-10-04T16:00:00Z" {
		t.Fatalf("ends_at = %v", snap.EndsAt)
	}
	var raw map[string]any
	if err := json.Unmarshal(st.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["v"] != float64(1) || raw["ok"] != true || raw["origin"] != "service" {
		t.Fatalf("raw = %s", st.Raw)
	}
}

func TestCommands(t *testing.T) {
	runtimeDir(t)
	noLeaks(t)
	f := newFake()
	serve(t, f)
	c, ctx := dial(t), ctxTimeout(t)

	for _, step := range []func() error{
		func() error { return c.SetActive(ctx, false) },
		func() error { return c.SetActive(ctx, true) },
		func() error { return c.Extend(ctx, 15*time.Minute) },
		func() error { return c.Extend(ctx, -5*time.Minute) },
		func() error { return c.Stop(ctx) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.actives) != 2 || f.actives[0] || !f.actives[1] {
		t.Errorf("SetActive calls = %v", f.actives)
	}
	if len(f.extends) != 2 || f.extends[0] != 15*time.Minute || f.extends[1] != -5*time.Minute {
		t.Errorf("Extend calls = %v", f.extends)
	}
	if len(f.stops) != 1 || f.stops[0] != session.ReasonIPC {
		t.Errorf("Stop calls = %v", f.stops)
	}
}

func TestSecondListenIsAlreadyRunning(t *testing.T) {
	runtimeDir(t)
	srv, err := Listen(testInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	_, err = Listen(ServerInfo{Version: "x"})
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Listen = %v, want ErrAlreadyRunning", err)
	}
	var ar *AlreadyRunningError
	if !errors.As(err, &ar) || ar.PID != 4242 {
		t.Fatalf("error %v does not carry pid 4242", err)
	}
	if !strings.Contains(err.Error(), "4242") {
		t.Errorf("message %q lacks the pid", err)
	}
}

func TestCloseReleasesLockAndRemovesSocket(t *testing.T) {
	dir := runtimeDir(t)
	srv, err := Listen(testInfo)
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, SocketName)
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("socket missing while listening: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := os.Stat(sock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket still there after Close: %v", err)
	}
	if _, err := Dial(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Dial after Close = %v, want ErrNotRunning", err)
	}
	srv2, err := Listen(testInfo)
	if err != nil {
		t.Fatalf("Listen after Close: %v", err)
	}
	srv2.Close()
}

func TestStaleSocketIsReplaced(t *testing.T) {
	dir := runtimeDir(t)
	noLeaks(t)
	sock := filepath.Join(dir, SocketName)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("stale socket not left behind: %v", err)
	}

	if _, err := Dial(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Dial on stale socket = %v, want ErrNotRunning", err)
	}
	serve(t, newFake())
	if _, err := dial(t).Status(ctxTimeout(t)); err != nil {
		t.Fatalf("status after stale recovery: %v", err)
	}
}

func TestDialNotRunning(t *testing.T) {
	dir := runtimeDir(t)
	if _, err := Dial(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Dial on empty dir = %v, want ErrNotRunning", err)
	}
	t.Setenv(EnvRuntimeDir, filepath.Join(dir, "missing"))
	if _, err := Dial(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Dial on missing dir = %v, want ErrNotRunning", err)
	}
}

func event(typ session.EventType, msg string) session.Event {
	return session.Event{Time: time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC), Type: typ, Message: msg, Snapshot: session.Snapshot{Running: true, Mode: session.ModeIndefinite}}
}

func waitSubscribers(t *testing.T, f *fakeController, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for f.subscribers() != n {
		if time.Now().After(deadline) {
			t.Fatalf("subscribers = %d, want %d", f.subscribers(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func recv(t *testing.T, ch <-chan Event) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		return ev, ok
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s")
	}
	return Event{}, false
}

func TestSubscribeStreamsUntilSessionEnds(t *testing.T) {
	runtimeDir(t)
	noLeaks(t)
	f := newFake()
	serve(t, f)

	events, err := dial(t).Subscribe(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	waitSubscribers(t, f, 1)
	f.publish(event(session.EventWarning, "low battery"))
	f.publish(event(session.EventStopped, "stopped by user"))

	ev, ok := recv(t, events)
	if !ok || ev.Type != "warning" || ev.Message != "warning: low battery" || !ev.Snapshot.Running {
		t.Fatalf("first event = %+v (ok %v)", ev, ok)
	}
	var raw map[string]any
	if err := json.Unmarshal(ev.Raw, &raw); err != nil || raw["v"] != float64(1) || raw["type"] != "warning" {
		t.Fatalf("raw = %s (%v)", ev.Raw, err)
	}
	if ev, ok = recv(t, events); !ok || ev.Type != "stopped" {
		t.Fatalf("second event = %+v (ok %v)", ev, ok)
	}
	f.end()
	if _, ok := recv(t, events); ok {
		t.Fatal("stream still open after the session ended")
	}
}

func TestSubscribeEndsOnServerClose(t *testing.T) {
	runtimeDir(t)
	noLeaks(t)
	f := newFake()
	srv, err := Listen(testInfo)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), f) }()

	var streams []<-chan Event
	for range 3 {
		ch, err := dial(t).Subscribe(ctxTimeout(t))
		if err != nil {
			t.Fatal(err)
		}
		streams = append(streams, ch)
	}
	waitSubscribers(t, f, 3)
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	for _, ch := range streams {
		if _, ok := recv(t, ch); ok {
			t.Fatal("stream still open after server Close")
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve = %v", err)
	}
	if n := f.subscribers(); n != 0 {
		t.Fatalf("%d subscriptions not cancelled", n)
	}
}

func TestSubscribeClientCancel(t *testing.T) {
	runtimeDir(t)
	noLeaks(t)
	f := newFake()
	serve(t, f)

	ctx, cancel := context.WithCancel(context.Background())
	events, err := dial(t).Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	waitSubscribers(t, f, 1)
	cancel()
	if _, ok := recv(t, events); ok {
		t.Fatal("stream open after cancel")
	}
	waitSubscribers(t, f, 0)
}

func TestServeStopsOnContextCancel(t *testing.T) {
	runtimeDir(t)
	noLeaks(t)
	f := newFake()
	srv, err := Listen(testInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, f) }()

	events, err := dial(t).Subscribe(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	waitSubscribers(t, f, 1)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	if _, ok := recv(t, events); ok {
		t.Fatal("stream open after Serve returned")
	}
}

// rawRequest sends line on a fresh connection and returns the reply line.
func rawRequest(t *testing.T, sock, line string) map[string]any {
	t.Helper()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	reply, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatalf("reading reply to %.40q: %v", line, err)
	}
	var m map[string]any
	if err := json.Unmarshal(reply, &m); err != nil {
		t.Fatalf("reply %q: %v", reply, err)
	}
	return m
}

func TestMalformedRequests(t *testing.T) {
	dir := runtimeDir(t)
	noLeaks(t)
	f := newFake()
	serve(t, f)
	sock := filepath.Join(dir, SocketName)

	tests := map[string]struct{ line, want string }{
		"not json":         {"hello\n", "malformed request"},
		"array":            {"[1,2]\n", "malformed request"},
		"no version":       {`{"cmd":"status"}` + "\n", "unsupported protocol version 0"},
		"future version":   {`{"v":2,"cmd":"status"}` + "\n", "unsupported protocol version 2"},
		"unknown command":  {`{"v":1,"cmd":"reboot"}` + "\n", `unknown command "reboot"`},
		"missing command":  {`{"v":1}` + "\n", `unknown command ""`},
		"active missing":   {`{"v":1,"cmd":"set_active"}` + "\n", `"active"`},
		"seconds missing":  {`{"v":1,"cmd":"extend"}` + "\n", `"seconds"`},
		"seconds overflow": {`{"v":1,"cmd":"extend","seconds":9223372036854775807}` + "\n", "out of range"},
		"seconds float":    {`{"v":1,"cmd":"extend","seconds":1.5}` + "\n", "malformed request"},
		"too large":        {strings.Repeat("x", MaxRequestSize+10) + "\n", "request too large"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := rawRequest(t, sock, tt.line)
			if m["v"] != float64(1) || m["ok"] != false {
				t.Fatalf("reply = %v", m)
			}
			if msg, _ := m["error"].(string); !strings.Contains(msg, tt.want) {
				t.Fatalf("error %q does not contain %q", msg, tt.want)
			}
		})
	}
	// A request without a trailing newline still counts once the client
	// half-closes.
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte(`{"v":1,"cmd":"stop"}`))
	conn.(*net.UnixConn).CloseWrite()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	reply, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil || !strings.Contains(string(reply), `"ok":true`) {
		t.Fatalf("unterminated request: %q, %v", reply, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.stops) != 1 || len(f.extends) != 0 || len(f.actives) != 0 {
		t.Fatalf("malformed requests reached the controller: stops %v extends %v actives %v", f.stops, f.extends, f.actives)
	}
}

func TestRequestReadDeadline(t *testing.T) {
	dir := runtimeDir(t)
	noLeaks(t)
	old := requestTimeout
	requestTimeout = 200 * time.Millisecond
	t.Cleanup(func() { requestTimeout = old })
	serve(t, newFake())

	conn, err := net.Dial("unix", filepath.Join(dir, SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte(`{"v":1,`)) // never finished
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	start := time.Now()
	reply, _ := bufio.NewReader(conn).ReadBytes('\n')
	if time.Since(start) > 2*time.Second {
		t.Fatal("server kept an idle connection open past its deadline")
	}
	if !strings.Contains(string(reply), "timed out") {
		t.Fatalf("reply = %q, want a timeout error", reply)
	}
}

func TestConcurrentClients(t *testing.T) {
	runtimeDir(t)
	noLeaks(t)
	f := newFake()
	serve(t, f)
	c, ctx := dial(t), ctxTimeout(t)

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				_, err := c.Status(ctx)
				errs <- err
				return
			}
			errs <- c.Extend(ctx, time.Minute)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.extends) != 16 {
		t.Fatalf("%d extends, want 16", len(f.extends))
	}
}

func TestRemoteErrorIsTyped(t *testing.T) {
	dir := runtimeDir(t)
	ln, err := net.Listen("unix", filepath.Join(dir, SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				bufio.NewReader(conn).ReadBytes('\n')
				conn.Write([]byte(`{"v":1,"ok":false,"error":"boom"}` + "\n"))
			}()
		}
	}()
	err = dial(t).Stop(ctxTimeout(t))
	var re *RemoteError
	if !errors.As(err, &re) || re.Message != "boom" {
		t.Fatalf("Stop = %v, want RemoteError boom", err)
	}
}

func TestRuntimeDirResolution(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cache := func() (string, error) { return "/home/u/.cache", nil }

	got, err := resolveDir(env(map[string]string{EnvRuntimeDir: "/custom"}), cache)
	if err != nil || got != "/custom" {
		t.Errorf("override: %q, %v", got, err)
	}
	long := "/" + strings.Repeat("d", 120)
	if _, err := resolveDir(env(map[string]string{EnvRuntimeDir: long}), cache); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Errorf("long override: %v, want a too-long error", err)
	}
	if runtime.GOOS == "windows" {
		got, err = resolveDir(env(map[string]string{"LOCALAPPDATA": `C:\Users\u\AppData\Local`}), cache)
		if want := filepath.Join(`C:\Users\u\AppData\Local`, "keepalive"); err != nil || got != want {
			t.Errorf("windows: %q, %v; want %q", got, err, want)
		}
		return
	}
	got, err = resolveDir(env(map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}), cache)
	if err != nil || got != "/run/user/1000/keepalive" {
		t.Errorf("xdg: %q, %v", got, err)
	}
	got, err = resolveDir(env(nil), cache)
	if err != nil || got != "/home/u/.cache/keepalive" {
		t.Errorf("cache: %q, %v", got, err)
	}
	longCache := func() (string, error) { return long, nil }
	got, err = resolveDir(env(nil), longCache)
	if want := "/tmp/keepalive-" + strconv.Itoa(os.Getuid()); err != nil || got != want {
		t.Errorf("long cache dir: %q, %v; want %q", got, err, want)
	}
}

//go:build linux

package activity

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMutterIdleIsTypedMilliseconds(t *testing.T) {
	d, err := mutterIdleFromReply([]any{uint64(64)})
	if err != nil || d != 64*time.Millisecond {
		t.Fatalf("uint64(64) = %v, %v; want 64ms", d, err)
	}
	d, err = mutterIdleFromReply([]any{uint64(185_000)})
	if err != nil || d != 185*time.Second {
		t.Fatalf("uint64(185000) = %v, %v; want 3m5s", d, err)
	}
	for _, body := range [][]any{{"uint64 185000"}, {int32(5)}, {}, {uint64(1), uint64(2)}} {
		if _, err := mutterIdleFromReply(body); err == nil {
			t.Fatalf("reply %#v accepted", body)
		}
	}
}

func TestKDEIdleIsMilliseconds(t *testing.T) {
	d, err := kdeIdleFromReply([]any{uint32(1500)})
	if err != nil || d != 1500*time.Millisecond {
		t.Fatalf("uint32(1500) = %v, %v; want 1.5s", d, err)
	}
	if _, err := kdeIdleFromReply([]any{"1500"}); err == nil {
		t.Fatal("string reply accepted")
	}
}

func TestParseXprintidle(t *testing.T) {
	for out, want := range map[string]time.Duration{"0\n": 0, "6512\n": 6512 * time.Millisecond, " 120000 ": 2 * time.Minute} {
		if got, err := parseXprintidle(out); err != nil || got != want {
			t.Errorf("parseXprintidle(%q) = %v, %v; want %v", out, got, err, want)
		}
	}
	for _, out := range []string{"", "couldn't open display", "-5", "12.5"} {
		if _, err := parseXprintidle(out); err == nil {
			t.Errorf("parseXprintidle(%q) accepted", out)
		}
	}
}

// Captured from ydotool 0.1.8 (Ubuntu 24.04) and 1.0.4 (Fedora 42).
const (
	ydotool018Help = "Usage: ydotool <cmd> <args>\nAvailable commands:\n  type\n  recorder\n  mousemove\n  key\n  click\n"
	ydotool104Help = "Usage: ydotool <cmd> <args>\nAvailable commands:\n  click\n  mousemove\n  type\n  key\n  debug\n  bakers\n" +
		"Use environment variable YDOTOOL_SOCKET to specify daemon socket.\n"
)

func TestYdotoolGeneration(t *testing.T) {
	if g := ydotoolGeneration(ydotool018Help); g != 0 {
		t.Fatalf("0.1.8 help detected as %d", g)
	}
	if g := ydotoolGeneration(ydotool104Help); g != 1 {
		t.Fatalf("1.0.4 help detected as %d", g)
	}
	if g := ydotoolGeneration("sh: ydotool: not found"); g != -1 {
		t.Fatalf("garbage detected as %d", g)
	}
}

func TestYdotoolSocketSearchOrder(t *testing.T) {
	env := map[string]string{"YDOTOOL_SOCKET": "/custom/sock", "XDG_RUNTIME_DIR": "/run/user/1000"}
	getenv := func(k string) string { return env[k] }
	exists := map[string]bool{}
	isSock := func(p string) bool { return exists[p] }

	sock, candidates := ydotoolSocket(getenv, isSock)
	want := []string{"/custom/sock", "/run/user/1000/.ydotool_socket", "/tmp/.ydotool_socket"}
	if sock != "" || !reflect.DeepEqual(candidates, want) {
		t.Fatalf("got %q %v, want none of %v", sock, candidates, want)
	}
	for i := len(want) - 1; i >= 0; i-- {
		exists[want[i]] = true
		if sock, _ := ydotoolSocket(getenv, isSock); sock != want[i] {
			t.Fatalf("with %s present got %q", want[i], sock)
		}
	}
}

type call struct {
	timeout time.Duration
	env     []string
	name    string
	args    []string
}

type fakeRunner struct {
	calls []call
	ctxs  []context.Context
	reply func(c call) (string, string, error)
}

func (f *fakeRunner) run(ctx context.Context, timeout time.Duration, env []string, name string, args ...string) (string, string, error) {
	c := call{timeout, env, name, args}
	f.calls = append(f.calls, c)
	f.ctxs = append(f.ctxs, ctx)
	if f.reply != nil {
		return f.reply(c)
	}
	return "", "", nil
}

func found(string) (string, error)   { return "/usr/bin/x", nil }
func missing(string) (string, error) { return "", errors.New("not found") }

func TestYdotoolAvailability(t *testing.T) {
	newY := func(help string, sockets ...string) (*ydotool, *fakeRunner) {
		r := &fakeRunner{reply: func(c call) (string, string, error) {
			if len(c.args) > 0 && c.args[0] == "help" {
				return help, "", nil
			}
			return "", "", nil
		}}
		set := map[string]bool{}
		for _, s := range sockets {
			set[s] = true
		}
		return &ydotool{run: r.run, lookPath: found, getenv: func(string) string { return "" }, isSocket: func(p string) bool { return set[p] }}, r
	}
	var un *Unavailable

	y, _ := newY(ydotool018Help, "/tmp/.ydotool_socket")
	if err := y.Available(); !errors.As(err, &un) || !strings.Contains(un.Reason, "0.1.x") {
		t.Fatalf("0.1.8: %v", err)
	}
	y, _ = newY(ydotool104Help)
	if err := y.Available(); !errors.As(err, &un) || !strings.Contains(un.Reason, "ydotoold is not running") || !strings.Contains(un.Hint, "systemctl --user") {
		t.Fatalf("no daemon: %v", err)
	}
	y, r := newY(ydotool104Help, "/tmp/.ydotool_socket")
	if err := y.Available(); err != nil {
		t.Fatalf("1.x with daemon: %v", err)
	}
	y.lookPath = missing
	y2 := &ydotool{run: r.run, lookPath: missing}
	if err := y2.Available(); !errors.As(err, &un) || un.Reason != "ydotool is not installed" {
		t.Fatalf("not installed: %v", err)
	}

	r.calls = nil
	if err := y.MoveBy(-5, 3); err != nil {
		t.Fatal(err)
	}
	want := call{cmdTimeout, []string{"YDOTOOL_SOCKET=/tmp/.ydotool_socket"}, "ydotool", []string{"mousemove", "-x", "-5", "-y", "3"}}
	if !reflect.DeepEqual(r.calls, []call{want}) {
		t.Fatalf("MoveBy ran %+v, want %+v", r.calls, want)
	}
	if err := y.Tap(); err != nil {
		t.Fatal(err)
	}
	if got := r.calls[1].args; !reflect.DeepEqual(got, []string{"key", "54:1", "54:0"}) {
		t.Fatalf("Tap ran %v", got)
	}
	if y.MinStep() < 40*time.Millisecond {
		t.Fatal("ydotool must coalesce moves")
	}
}

func TestXdotoolChain(t *testing.T) {
	p := Path{
		{X: 3, Y: -2, Delay: 12 * time.Millisecond},
		{X: 3.2, Y: -2.1, Delay: 10 * time.Millisecond}, // same pixel: merged into the next
		{X: 10.6, Y: 4.4, Delay: 9 * time.Millisecond},
		{X: 0, Y: 0, Delay: 14 * time.Millisecond},
	}
	got := xdotoolChain(100, 200, p)
	want := strings.Fields("sleep 0.012 mousemove -- 103 198 sleep 0.019 mousemove -- 111 204 sleep 0.014 mousemove -- 100 200")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chain\n got %v\nwant %v", got, want)
	}
	// A burst always ends with an explicit move back to the origin.
	full := xdotoolChain(500, 400, NewPath(seeded(9)))
	if tail := strings.Join(full[len(full)-3:], " "); tail != "-- 500 400" {
		t.Fatalf("chain ends with %q", tail)
	}
}

func TestXdotoolParsing(t *testing.T) {
	x, y, ok := parseMouseLocation("X=812\nY=377\nSCREEN=0\nWINDOW=4194310\n")
	if !ok || x != 812 || y != 377 {
		t.Fatalf("parseMouseLocation = %v %v %v", x, y, ok)
	}
	if _, _, ok := parseMouseLocation("Error: Can't open display"); ok {
		t.Fatal("parsed an error")
	}
	r, ok := parseDisplayGeometry("1920 1080\n")
	if !ok || r != (Rect{W: 1920, H: 1080}) {
		t.Fatalf("parseDisplayGeometry = %+v %v", r, ok)
	}
	if _, ok := parseDisplayGeometry("1920"); ok {
		t.Fatal("parsed a broken geometry")
	}
}

func TestXdotoolRefusesWayland(t *testing.T) {
	r := &fakeRunner{}
	x := &xdotool{run: r.run, lookPath: found, env: linuxEnv{display: ":0", wayland: "wayland-0"}}
	var un *Unavailable
	if err := x.Available(); !errors.As(err, &un) || !strings.Contains(un.Reason, "Wayland") {
		t.Fatalf("Wayland: %v", err)
	}
	if len(r.calls) != 0 {
		t.Fatal("ran xdotool on Wayland")
	}
}

func TestXdotoolPlayRunsOneProcess(t *testing.T) {
	r := &fakeRunner{}
	x := &xdotool{run: r.run, lookPath: found, env: linuxEnv{display: ":99"}}
	p := NewPath(seeded(2))
	if err := x.Play(context.Background(), 50, 60, p); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || r.calls[0].name != "xdotool" || r.calls[0].timeout != p.Duration()+cmdTimeout {
		t.Fatalf("calls %+v", r.calls)
	}
	// A failed chain puts the pointer back.
	r.calls = nil
	r.reply = func(c call) (string, string, error) {
		if c.args[0] == "sleep" {
			return "", "", errors.New("killed")
		}
		return "", "", nil
	}
	if err := x.Play(context.Background(), 50, 60, p); err == nil {
		t.Fatal("failure not reported")
	}
	if last := r.calls[len(r.calls)-1]; !reflect.DeepEqual(last.args, []string{"mousemove", "--", "50", "60"}) {
		t.Fatalf("after a failed chain ran %v", last.args)
	}
}

func TestNoIdleHints(t *testing.T) {
	kde := noIdleHint(linuxEnv{wayland: "wayland-0", desktop: "KDE"})
	if !strings.Contains(kde, "Plasma (X11)") {
		t.Fatalf("KDE Wayland hint %q", kde)
	}
	if h := noIdleHint(linuxEnv{wayland: "wayland-1", desktop: "SWAY"}); !strings.Contains(h, "sway") {
		t.Fatalf("wlroots hint %q", h)
	}
	if h := noIdleHint(linuxEnv{display: ":0"}); !strings.Contains(h, "xprintidle") {
		t.Fatalf("X11 hint %q", h)
	}
}

// hungBus accepts D-Bus connections and never answers, like a frozen
// dbus-daemon.
func hungBus(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bus")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return "unix:path=" + path
}

func TestNewBackendGivesUpOnAHungBusWhenCancelled(t *testing.T) {
	addr := hungBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", addr)
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan *backend, 1)
	go func() { done <- newBackend(ctx, false) }()
	select {
	case b := <-done:
		b.Close()
		if b.lock != nil || len(b.sources) != 0 {
			t.Fatalf("a hung bus produced lock %v, sources %v", b.lock, b.sources)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("newBackend still blocked on a hung bus 3s after its context ended")
	}
}

func TestXprintidleRunsUnderTheBackendContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &fakeRunner{reply: func(call) (string, string, error) { return "1000", "", nil }}
	if _, err := (xprintidle{ctx: ctx, run: r.run}).Idle(); err != nil {
		t.Fatal(err)
	}
	if len(r.ctxs) != 1 || r.ctxs[0].Err() == nil {
		t.Fatal("xprintidle did not run under the backend's context")
	}
}

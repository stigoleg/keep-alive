//go:build linux

package activity

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
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
	dial := func(p string) error {
		if exists[p] {
			return nil
		}
		return syscall.ENOENT
	}

	sock, candidates, err := ydotoolSocket(getenv, dial)
	want := []string{"/custom/sock", "/run/user/1000/.ydotool_socket", "/tmp/.ydotool_socket"}
	if sock != "" || err != nil || !reflect.DeepEqual(candidates, want) {
		t.Fatalf("got %q %v %v, want none of %v", sock, candidates, err, want)
	}
	for i := len(want) - 1; i >= 0; i-- {
		exists[want[i]] = true
		if sock, _, _ := ydotoolSocket(getenv, dial); sock != want[i] {
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
		dial := func(p string) error {
			if set[p] {
				return nil
			}
			return syscall.ENOENT
		}
		return &ydotool{run: r.run, lookPath: found, getenv: func(string) string { return "" }, dial: dial}, r
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

type namedIdle string

func (n namedIdle) Name() string               { return string(n) }
func (namedIdle) Idle() (time.Duration, error) { return time.Minute, nil }

func TestPickIdleNeverGatesOnXWayland(t *testing.T) {
	mutter, xp, kde := namedIdle("mutter"), namedIdle("xprintidle"), namedIdle("kde")
	names := func(ss []IdleSource) []string {
		var out []string
		for _, s := range ss {
			out = append(out, s.Name())
		}
		return out
	}
	tests := []struct {
		name            string
		env             linuxEnv
		mutter, xp, kde IdleSource
		gate, xwayland  IdleSource
		sources         []string
	}{
		{"GNOME Wayland with XWayland", linuxEnv{display: ":0", wayland: "wayland-0", desktop: "GNOME"}, mutter, xp, nil, mutter, xp, []string{"mutter", "xprintidle"}},
		{"KDE Wayland with XWayland", linuxEnv{display: ":1", wayland: "wayland-0", desktop: "KDE"}, nil, xp, nil, nil, xp, []string{"xprintidle"}},
		{"sway without XWayland", linuxEnv{wayland: "wayland-1", desktop: "SWAY"}, nil, nil, nil, nil, nil, nil},
		{"GNOME on X11", linuxEnv{display: ":0", desktop: "GNOME"}, mutter, xp, nil, mutter, nil, []string{"mutter", "xprintidle"}},
		{"plain X11", linuxEnv{display: ":0", desktop: "XFCE"}, nil, xp, nil, xp, nil, []string{"xprintidle"}},
		{"KDE X11 without xprintidle", linuxEnv{display: ":0", desktop: "KDE"}, nil, nil, kde, kde, nil, []string{"kde"}},
	}
	for _, tt := range tests {
		got := pickIdle(tt.env, tt.mutter, tt.xp, tt.kde)
		if got.gate != tt.gate || got.xwayland != tt.xwayland || !reflect.DeepEqual(names(got.sources), tt.sources) {
			t.Errorf("%s: gate %v, xwayland %v, sources %v; want %v, %v, %v", tt.name, got.gate, got.xwayland, names(got.sources), tt.gate, tt.xwayland, tt.sources)
		}
	}
}

func TestXWaylandNoteOnlyOnWaylandWithXWayland(t *testing.T) {
	if n := desktopNotes(linuxEnv{display: ":0", wayland: "wayland-0"}); len(n) != 1 || !strings.Contains(n[0], "--ozone-platform=wayland") {
		t.Fatalf("Wayland with XWayland: %v", n)
	}
	for _, env := range []linuxEnv{{display: ":0"}, {wayland: "wayland-0"}} {
		if n := desktopNotes(env); len(n) != 0 {
			t.Fatalf("%+v: %v", env, n)
		}
	}
}

type fakeLockSource struct {
	locked bool
	err    error
}

func (f fakeLockSource) Locked() (bool, error) { return f.locked, f.err }

func TestAnyLockIsLockedWhenAnySourceSaysSo(t *testing.T) {
	notSupported := errors.New("org.freedesktop.DBus.Error.NotSupported")
	src := func(name string, locked bool, err error) namedLock {
		return namedLock{name: name, LockSource: fakeLockSource{locked, err}}
	}
	tests := []struct {
		name    string
		lock    anyLock
		locked  bool
		wantErr bool
	}{
		{"logind unlocked, GNOME locked", anyLock{src("logind", false, nil), src("gnome", true, nil)}, true, false},
		{"logind locked, screensaver unlocked", anyLock{src("logind", true, nil), src("fdo", false, nil)}, true, false},
		{"NotSupported is ignored", anyLock{src("fdo", false, notSupported), src("gnome", false, nil)}, false, false},
		{"NotSupported next to a locked source", anyLock{src("fdo", false, notSupported), src("mate", true, nil)}, true, false},
		{"every source failing is unknown", anyLock{src("logind", false, errors.New("gone")), src("fdo", false, notSupported)}, false, true},
	}
	for _, tt := range tests {
		locked, err := tt.lock.Locked()
		if locked != tt.locked || (err != nil) != tt.wantErr {
			t.Errorf("%s: Locked() = %v, %v; want %v, error %v", tt.name, locked, err, tt.locked, tt.wantErr)
		}
	}
	if got := (anyLock{src("logind LockedHint", false, nil), src("org.gnome.ScreenSaver", false, nil)}).String(); got != "logind LockedHint, org.gnome.ScreenSaver" {
		t.Errorf("String() = %q", got)
	}
}

func TestScreensaverServicesMatchChromium(t *testing.T) {
	want := map[string]dbus.ObjectPath{
		"org.freedesktop.ScreenSaver": "/org/freedesktop/ScreenSaver",
		"org.gnome.ScreenSaver":       "/org/gnome/ScreenSaver",
		"org.mate.ScreenSaver":        "/org/mate/ScreenSaver",
		"org.cinnamon.ScreenSaver":    "/org/cinnamon/ScreenSaver",
		"org.xfce.ScreenSaver":        "/org/xfce/ScreenSaver",
	}
	got := map[string]dbus.ObjectPath{}
	for _, s := range screensaverServices {
		got[s.name] = s.path
		if s.iface == "" {
			t.Errorf("%s has no interface", s.name)
		}
	}
	for name, path := range want {
		if got[name] != path {
			t.Errorf("%s at %q, want %q", name, got[name], path)
		}
	}
}

func TestYdotoolConnectsToTheDaemonSocket(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live")
	daemon, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: live, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer daemon.Close()
	stale := filepath.Join(dir, "stale")
	dead, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: stale, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	_ = dead.Close() // the socket file stays, nobody reads it: ydotoold died

	if err := dialYdotoold(live); err != nil {
		t.Fatalf("live daemon: %v", err)
	}
	if err := dialYdotoold(stale); !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("stale socket: %v, want connection refused", err)
	}
	if err := dialYdotoold(filepath.Join(dir, "missing")); !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("missing socket: %v, want ENOENT", err)
	}

	r := &fakeRunner{reply: func(call) (string, string, error) { return ydotool104Help, "", nil }}
	newY := func(sock string, dial func(string) error) *ydotool {
		env := map[string]string{"YDOTOOL_SOCKET": sock}
		return &ydotool{run: r.run, lookPath: found, getenv: func(k string) string { return env[k] }, dial: dial}
	}
	if err := newY(live, dialYdotoold).Available(); err != nil {
		t.Fatalf("live daemon: %v", err)
	}
	var un *Unavailable
	if err := newY(stale, dialYdotoold).Available(); !errors.As(err, &un) ||
		!strings.Contains(un.Reason, "connection refused") || !strings.Contains(un.Hint, "systemctl --user") {
		t.Fatalf("stale socket: %v", err)
	}
	denied := func(string) error {
		return &net.OpError{Op: "dial", Net: "unixgram", Err: os.NewSyscallError("connect", syscall.EACCES)}
	}
	if err := newY(live, denied).Available(); !errors.As(err, &un) ||
		!strings.Contains(un.Reason, "permission denied") || !strings.Contains(un.Hint, "another user") {
		t.Fatalf("socket of another user: %v", err)
	}
}

func TestUinputProbeOpensTheDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uinput")
	if err := probeUinput(path); !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("missing device: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := probeUinput(path); err != nil {
		t.Fatalf("writable device: %v", err)
	}

	u := newUinput(false)
	var probed []string
	var un *Unavailable
	for errno, want := range map[syscall.Errno]string{
		syscall.EACCES: "no write access",
		syscall.EPERM:  "no write access",
		syscall.ENODEV: "uinput module",
		syscall.EBUSY:  "cannot open /dev/uinput",
	} {
		u.probe = func(p string) error { probed = append(probed, p); return errno }
		if err := u.Available(); !errors.As(err, &un) || !strings.Contains(un.Reason, want) {
			t.Errorf("%v: %v, want a reason with %q", errno, err, want)
		}
	}
	if len(probed) == 0 || probed[0] != "/dev/uinput" {
		t.Fatalf("probed %v", probed)
	}
}

func TestFirstAvailableSkipsFailedMethods(t *testing.T) {
	a := unavailableInjector{err: nil}
	b := unavailableInjector{err: &Unavailable{Reason: "not installed", Hint: "install it"}}
	named := func(name string, u unavailableInjector) Injector { return renamed{u, name} }
	cands := []Injector{named("uinput", a), named("ydotool", b), named("xdotool", a)}

	inj, err := firstAvailable(cands, linuxEnv{}, nil)
	if err != nil || inj.Name() != "uinput" {
		t.Fatalf("no skip: %v, %v", inj, err)
	}
	inj, err = firstAvailable(cands, linuxEnv{}, func(n string) bool { return n == "uinput" })
	if err != nil || inj.Name() != "xdotool" {
		t.Fatalf("uinput skipped: %v, %v", inj, err)
	}
	_, err = firstAvailable(cands, linuxEnv{}, func(n string) bool { return n != "ydotool" })
	var un *Unavailable
	if !errors.As(err, &un) || !strings.Contains(un.Reason, "uinput: failed recently") || !strings.Contains(un.Reason, "ydotool: not installed") {
		t.Fatalf("all skipped or unavailable: %v", err)
	}
}

type renamed struct {
	unavailableInjector
	name string
}

func (r renamed) Name() string { return r.name }

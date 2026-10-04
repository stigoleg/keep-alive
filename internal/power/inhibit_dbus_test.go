package power

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- fake bus ----

type fakeCall struct {
	bus    string
	dest   string
	path   string
	method string
	args   []any
}

// recorder is shared by the fake connections and fds of one test so the
// release order across them can be asserted.
type recorder struct {
	mu     sync.Mutex
	events []string
	calls  []fakeCall
}

func (r *recorder) log(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, fmt.Sprintf(format, a...))
}

func (r *recorder) record(c fakeCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
	r.events = append(r.events, "call "+c.bus+" "+c.method)
}

func (r *recorder) callsTo(method string) []fakeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []fakeCall
	for _, c := range r.calls {
		if c.method == method {
			out = append(out, c)
		}
	}
	return out
}

func (r *recorder) eventList() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type fakeFD struct {
	rec    *recorder
	closed bool
}

func (f *fakeFD) Close() error {
	f.rec.log("close fd")
	f.closed = true
	return nil
}

type fakeConn struct {
	name    string // "system" or "session"
	rec     *recorder
	owners  map[string]bool
	replies map[string]func(path string, args []any) ([]any, error) // by method
	done    chan struct{}
	once    sync.Once
}

func newFakeConn(name string, rec *recorder) *fakeConn {
	return &fakeConn{name: name, rec: rec, owners: map[string]bool{}, replies: map[string]func(string, []any) ([]any, error){}, done: make(chan struct{})}
}

func (c *fakeConn) HasOwner(_ context.Context, name string) (bool, error) {
	return c.owners[name], nil
}

func (c *fakeConn) Call(_ context.Context, dest, path, method string, args ...any) ([]any, error) {
	c.rec.record(fakeCall{bus: c.name, dest: dest, path: path, method: method, args: args})
	if f := c.replies[method]; f != nil {
		return f(path, args)
	}
	return nil, &dbusError{Name: "org.freedesktop.DBus.Error.UnknownMethod"}
}

func (c *fakeConn) Done() <-chan struct{} { return c.done }

func (c *fakeConn) Close() error {
	c.rec.log("close %s", c.name)
	c.disconnect()
	return nil
}

func (c *fakeConn) disconnect() { c.once.Do(func() { close(c.done) }) }

func reply(v ...any) func(string, []any) ([]any, error) {
	return func(string, []any) ([]any, error) { return v, nil }
}

// desktop is a fake system + session bus with logind and every desktop
// service present.
type desktop struct {
	rec     *recorder
	system  *fakeConn
	session *fakeConn
	fd      *fakeFD
	sysErr  error
	sesErr  error
	sysN    int
	sesN    int
}

func newDesktop() *desktop {
	rec := &recorder{}
	d := &desktop{rec: rec, system: newFakeConn("system", rec), session: newFakeConn("session", rec), fd: &fakeFD{rec: rec}}
	d.system.owners[logindName] = true
	d.system.replies[logindInhibit] = reply(d.fd)
	for _, s := range []desktopService{screenSaverService, gnomeSessionService, powerManagementService} {
		d.session.owners[s.name] = true
	}
	d.session.replies["org.freedesktop.ScreenSaver.Inhibit"] = reply(uint32(11))
	d.session.replies["org.gnome.SessionManager.Inhibit"] = reply(uint32(22))
	d.session.replies["org.freedesktop.PowerManagement.Inhibit.Inhibit"] = reply(int32(-33)) // KDE PowerDevil style
	for _, m := range []string{"org.freedesktop.ScreenSaver.UnInhibit", "org.gnome.SessionManager.Uninhibit", "org.freedesktop.PowerManagement.Inhibit.UnInhibit"} {
		d.session.replies[m] = reply()
	}
	return d
}

func (d *desktop) inhibitor() *dbusInhibitor {
	return &dbusInhibitor{
		system: func(context.Context) (dbusConn, error) {
			d.sysN++
			if d.sysErr != nil {
				return nil, d.sysErr
			}
			return d.system, nil
		},
		session: func(context.Context) (dbusConn, error) {
			d.sesN++
			if d.sesErr != nil {
				return nil, d.sesErr
			}
			return d.session, nil
		},
	}
}

func mustAcquire(t *testing.T, in Inhibitor, o Options) *dbusHold {
	t.Helper()
	h, err := in.Acquire(context.Background(), o)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(func() { h.Release() })
	return h.(*dbusHold)
}

func assertArgs(t *testing.T, c fakeCall, want ...any) {
	t.Helper()
	if !reflect.DeepEqual(c.args, want) {
		t.Errorf("%s args = %#v, want %#v", c.method, c.args, want)
	}
}

// ---- tests ----

func TestDBusKeepDisplayTakesEveryInhibitor(t *testing.T) {
	d := newDesktop()
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: true, Reason: "test"})

	want := "logind(sleep:idle) + org.freedesktop.ScreenSaver + org.gnome.SessionManager + org.freedesktop.PowerManagement"
	if got := h.Describe(); got != want {
		t.Errorf("Describe() = %q, want %q", got, want)
	}
	reason := "keepalive: test"
	calls := d.rec.callsTo(logindInhibit)
	if len(calls) != 1 || calls[0].bus != "system" || calls[0].dest != logindName || calls[0].path != logindPath {
		t.Fatalf("logind calls = %+v", calls)
	}
	assertArgs(t, calls[0], "sleep:idle", "keepalive", reason, "block")
	ss := d.rec.callsTo("org.freedesktop.ScreenSaver.Inhibit")
	if len(ss) != 1 || ss[0].path != "/org/freedesktop/ScreenSaver" || ss[0].bus != "session" {
		t.Fatalf("ScreenSaver calls = %+v", ss)
	}
	assertArgs(t, ss[0], "keepalive", reason)
	gs := d.rec.callsTo("org.gnome.SessionManager.Inhibit")
	if len(gs) != 1 {
		t.Fatalf("gnome-session calls = %+v", gs)
	}
	assertArgs(t, gs[0], "keepalive", uint32(0), reason, uint32(12))
	pm := d.rec.callsTo("org.freedesktop.PowerManagement.Inhibit.Inhibit")
	if len(pm) != 1 || pm[0].path != "/org/freedesktop/PowerManagement/Inhibit" {
		t.Fatalf("PowerManagement calls = %+v", pm)
	}
	assertArgs(t, pm[0], "keepalive", reason)
	if d.fd.closed {
		t.Fatal("logind fd closed while held")
	}

	before := len(d.rec.eventList())
	if err := h.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	got := d.rec.eventList()[before:]
	wantOrder := []string{
		"call session org.freedesktop.PowerManagement.Inhibit.UnInhibit",
		"call session org.gnome.SessionManager.Uninhibit",
		"call session org.freedesktop.ScreenSaver.UnInhibit",
		"close fd",
		"close session",
		"close system",
	}
	if !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("release order:\n got %q\nwant %q", got, wantOrder)
	}
	// Cookies go back with the type the service returned.
	assertArgs(t, d.rec.callsTo("org.freedesktop.PowerManagement.Inhibit.UnInhibit")[0], int32(-33))
	assertArgs(t, d.rec.callsTo("org.gnome.SessionManager.Uninhibit")[0], uint32(22))
	assertArgs(t, d.rec.callsTo("org.freedesktop.ScreenSaver.UnInhibit")[0], uint32(11))

	n := len(d.rec.eventList())
	if err := h.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
	if len(d.rec.eventList()) != n {
		t.Fatal("second Release touched the bus")
	}
}

func TestDBusSystemOnlyHoldsSleep(t *testing.T) {
	d := newDesktop()
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: false, Reason: "test"})

	if got, want := h.Describe(), "logind(sleep) + org.gnome.SessionManager"; got != want {
		t.Errorf("Describe() = %q, want %q", got, want)
	}
	assertArgs(t, d.rec.callsTo(logindInhibit)[0], "sleep", "keepalive", "keepalive: test", "block")
	assertArgs(t, d.rec.callsTo("org.gnome.SessionManager.Inhibit")[0], "keepalive", uint32(0), "keepalive: test", uint32(4))
	if n := len(d.rec.callsTo("org.freedesktop.ScreenSaver.Inhibit")) + len(d.rec.callsTo("org.freedesktop.PowerManagement.Inhibit.Inhibit")); n != 0 {
		t.Fatalf("display inhibitors called %d times with KeepDisplay=false", n)
	}
}

func TestDBusSkipsServicesWithoutOwner(t *testing.T) {
	d := newDesktop()
	d.session.owners[gnomeSessionService.name] = false
	d.session.owners[powerManagementService.name] = false
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: true})

	if got, want := h.Describe(), "logind(sleep:idle) + org.freedesktop.ScreenSaver"; got != want {
		t.Errorf("Describe() = %q, want %q", got, want)
	}
	if n := len(d.rec.callsTo("org.gnome.SessionManager.Inhibit")) + len(d.rec.callsTo("org.freedesktop.PowerManagement.Inhibit.Inhibit")); n != 0 {
		t.Fatalf("services without an owner were called %d times", n)
	}
}

func TestDBusScreenSaverFallsBackToKDEPath(t *testing.T) {
	d := newDesktop()
	d.session.replies["org.freedesktop.ScreenSaver.Inhibit"] = func(path string, _ []any) ([]any, error) {
		if path == "/ScreenSaver" {
			return []any{uint32(5)}, nil
		}
		return nil, &dbusError{Name: "org.freedesktop.DBus.Error.UnknownObject"}
	}
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: true})
	h.Release()

	un := d.rec.callsTo("org.freedesktop.ScreenSaver.UnInhibit")
	if len(un) != 1 || un[0].path != "/ScreenSaver" {
		t.Fatalf("UnInhibit calls = %+v, want one on /ScreenSaver", un)
	}
	assertArgs(t, un[0], uint32(5))
}

func TestDBusUnexpectedCookieTypeSkipsService(t *testing.T) {
	d := newDesktop()
	d.session.replies["org.freedesktop.PowerManagement.Inhibit.Inhibit"] = reply("not-a-cookie")
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: true})
	if strings.Contains(h.Describe(), "PowerManagement") {
		t.Fatalf("Describe() = %q lists a service whose cookie could not be decoded", h.Describe())
	}
}

func TestDBusPolkitRefusalUsesFallback(t *testing.T) {
	d := newDesktop()
	d.system.replies[logindInhibit] = func(string, []any) ([]any, error) {
		return nil, &dbusError{Name: "org.freedesktop.DBus.Error.InteractiveAuthorizationRequired"}
	}
	fb := &fakeFallbackHold{rec: d.rec, lost: make(chan error, 1)}
	in := d.inhibitor()
	var gotWhat string
	in.fallback = func(_ context.Context, what, _ string) (Hold, error) {
		gotWhat = what
		return fb, nil
	}
	h := mustAcquire(t, in, Options{KeepDisplay: true})
	if gotWhat != "sleep:idle" {
		t.Errorf("fallback what = %q", gotWhat)
	}
	if got, want := h.Describe(), "systemd-inhibit(sleep:idle) + org.freedesktop.ScreenSaver + org.gnome.SessionManager + org.freedesktop.PowerManagement"; got != want {
		t.Errorf("Describe() = %q, want %q", got, want)
	}
	if !slices.Contains(d.rec.eventList(), "close system") {
		t.Error("system bus connection kept although logind refused")
	}
	h.Release()
	if !fb.released {
		t.Fatal("fallback hold not released")
	}
}

func TestDBusNothingAvailableFailsWithHint(t *testing.T) {
	d := newDesktop()
	d.system.replies[logindInhibit] = func(string, []any) ([]any, error) {
		return nil, &dbusError{Name: "org.freedesktop.DBus.Error.AccessDenied", Message: "Permission denied"}
	}
	for name := range d.session.owners {
		d.session.owners[name] = false
	}
	_, err := d.inhibitor().Acquire(context.Background(), Options{KeepDisplay: true})
	if err == nil {
		t.Fatal("Acquire succeeded with nothing to hold")
	}
	if HintOf(err) != polkitHint {
		t.Errorf("hint = %q, want the polkit hint", HintOf(err))
	}
	if !strings.Contains(err.Error(), "AccessDenied") {
		t.Errorf("error %q does not name the refusal", err)
	}
	ev := d.rec.eventList()
	if !slices.Contains(ev, "close system") || !slices.Contains(ev, "close session") {
		t.Fatalf("connections not closed after failure: %q", ev)
	}
}

func TestDBusNoBuses(t *testing.T) {
	d := newDesktop()
	d.sysErr = errors.New("no such file")
	d.sesErr = errors.New("no session bus")
	_, err := d.inhibitor().Acquire(context.Background(), Options{})
	if err == nil {
		t.Fatal("Acquire succeeded without buses")
	}
	if HintOf(err) != noBusesHint {
		t.Errorf("hint = %q", HintOf(err))
	}
}

func TestDBusSessionOnlyWhenSystemBusMissing(t *testing.T) {
	d := newDesktop()
	d.sysErr = errors.New("no system bus")
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: true})
	if strings.Contains(h.Describe(), "logind") {
		t.Fatalf("Describe() = %q", h.Describe())
	}
	if h.system != nil {
		t.Fatal("hold keeps a system connection")
	}
}

func TestDBusLostOnDisconnect(t *testing.T) {
	d := newDesktop()
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: true})
	d.session.disconnect()
	select {
	case err := <-h.Lost():
		if !strings.Contains(err.Error(), "session bus") {
			t.Errorf("loss error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Lost did not fire on disconnect")
	}
}

func TestDBusNoLossAfterRelease(t *testing.T) {
	d := newDesktop()
	h := mustAcquire(t, d.inhibitor(), Options{KeepDisplay: true})
	h.Release() // closes both fake connections, closing their Done channels
	select {
	case err := <-h.Lost():
		t.Fatalf("Lost fired after Release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestDBusFallbackLossIsForwarded(t *testing.T) {
	d := newDesktop()
	d.system.owners[logindName] = false
	fb := &fakeFallbackHold{rec: d.rec, lost: make(chan error, 1)}
	in := d.inhibitor()
	in.fallback = func(context.Context, string, string) (Hold, error) { return fb, nil }
	h := mustAcquire(t, in, Options{})
	fb.lost <- errors.New("systemd-inhibit exited")
	select {
	case err := <-h.Lost():
		if !strings.Contains(err.Error(), "systemd-inhibit") {
			t.Errorf("loss error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fallback loss not forwarded")
	}
}

func TestDBusCancelledContext(t *testing.T) {
	d := newDesktop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.inhibitor().Acquire(ctx, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if d.sysN+d.sesN != 0 {
		t.Fatal("connected with a cancelled context")
	}
}

func TestDBusMechanisms(t *testing.T) {
	d := newDesktop()
	d.session.owners[powerManagementService.name] = false
	in := d.inhibitor()
	in.fallbackCheck = func() Mechanism { return Mechanism{Name: "systemd-inhibit", Available: true} }
	got := map[string]bool{}
	for _, m := range in.mechanisms(context.Background()) {
		got[m.Name] = m.Available
	}
	want := map[string]bool{
		"system bus": true, "logind": true, "systemd-inhibit": true, "session bus": true,
		"org.freedesktop.ScreenSaver": true, "org.gnome.SessionManager": true, "org.freedesktop.PowerManagement": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mechanisms = %v, want %v", got, want)
	}

	d.sysErr = errors.New("down")
	for _, m := range in.mechanisms(context.Background()) {
		if (m.Name == "system bus" || m.Name == "logind") && m.Available {
			t.Fatalf("%s available without a system bus", m.Name)
		}
	}
}

func TestDecodeCookie(t *testing.T) {
	for _, tc := range []struct {
		body []any
		want any
		ok   bool
	}{
		{[]any{uint32(7)}, uint32(7), true},
		{[]any{int32(-7)}, int32(-7), true},
		{[]any{"7"}, nil, false},
		{nil, nil, false},
	} {
		got, err := decodeCookie(tc.body)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("decodeCookie(%v) = %v, %v", tc.body, got, err)
		}
	}
}

type fakeFallbackHold struct {
	rec      *recorder
	lost     chan error
	released bool
}

func (f *fakeFallbackHold) Release() error {
	f.rec.log("release fallback")
	f.released = true
	return nil
}

func (f *fakeFallbackHold) Describe() string   { return "systemd-inhibit(sleep:idle)" }
func (f *fakeFallbackHold) Lost() <-chan error { return f.lost }

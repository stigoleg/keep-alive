package power

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// TestDBusMock runs the real godbus inhibitor against python-dbusmock. It
// needs the buses and mocks that test/docker/power/mock.py sets up, so it
// only runs with KEEPALIVE_DBUSMOCK=1 (see test/docker/power/run.sh).
func TestDBusMock(t *testing.T) {
	if os.Getenv("KEEPALIVE_DBUSMOCK") == "" {
		t.Skip("needs python-dbusmock; run test/docker/power/run.sh")
	}
	sys, err := dbus.ConnectSystemBus()
	if err != nil {
		t.Fatal(err)
	}
	defer sys.Close()
	ses, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer ses.Close()

	for _, display := range []bool{true, false} {
		t.Run(fmt.Sprintf("keep_display=%v", display), func(t *testing.T) {
			for _, m := range sessionMocks {
				mockObject(ses, m).Call("org.freedesktop.DBus.Mock.ClearCalls", 0)
			}
			h, err := New().Acquire(context.Background(), Options{KeepDisplay: display, Reason: "docker"})
			if err != nil {
				t.Fatalf("Acquire: %v", err)
			}
			dh := h.(*dbusHold)
			want := "logind(sleep) + org.gnome.SessionManager"
			what := "sleep"
			if display {
				want = "logind(sleep:idle) + org.freedesktop.ScreenSaver + org.gnome.SessionManager + org.freedesktop.PowerManagement"
				what = "sleep:idle"
			}
			if got := h.Describe(); got != want {
				t.Fatalf("Describe() = %q, want %q", got, want)
			}

			// The inhibitors stay held: our connections remain on the bus
			// and logind still lists the lock after a pause.
			time.Sleep(1500 * time.Millisecond)
			sysName := dh.system.(godbusConn).c.Names()[0]
			sesName := dh.session.(godbusConn).c.Names()[0]
			if !nameHasOwner(t, sys, sysName) || !nameHasOwner(t, ses, sesName) {
				t.Fatal("a hold connection left the bus while held")
			}
			lock := fmt.Sprintf("%s|keepalive|keepalive: docker|block", what)
			if got := logindInhibitors(t, sys); !slices.Contains(got, lock) {
				t.Fatalf("logind inhibitors = %v, want %q", got, lock)
			}
			gnomeFlags := uint32(4)
			if display {
				gnomeFlags = 12
				assertMockCall(t, ses, sessionMocks[0], "Inhibit", "[keepalive keepalive: docker]")
				assertMockCall(t, ses, sessionMocks[2], "Inhibit", "[keepalive keepalive: docker]")
			} else if n := len(mockCalls(t, ses, sessionMocks[0])); n != 0 {
				t.Fatalf("ScreenSaver called %d times with keep_display=false", n)
			}
			assertMockCall(t, ses, sessionMocks[1], "Inhibit", fmt.Sprintf("[keepalive 0 keepalive: docker %d]", gnomeFlags))

			if err := h.Release(); err != nil {
				t.Fatalf("Release: %v", err)
			}
			if display {
				assertMockCall(t, ses, sessionMocks[0], "UnInhibit", "[11]")
				assertMockCall(t, ses, sessionMocks[2], "UnInhibit", "[-33]") // int32 cookie sent back as int32
			}
			assertMockCall(t, ses, sessionMocks[1], "Uninhibit", "[22]")
			waitFor(t, "logind lock released", func() bool { return !slices.Contains(logindInhibitors(t, sys), lock) })
			waitFor(t, "hold connections closed", func() bool {
				return !nameHasOwner(t, sys, sysName) && !nameHasOwner(t, ses, sesName)
			})
		})
	}

	t.Run("lost", func(t *testing.T) {
		h, err := New().Acquire(context.Background(), Options{KeepDisplay: true})
		if err != nil {
			t.Fatal(err)
		}
		defer h.Release()
		h.(*dbusHold).session.(godbusConn).c.Close() // simulate the session bus going away
		select {
		case err := <-h.(Watcher).Lost():
			t.Logf("lost: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("Lost did not fire")
		}
	})

	t.Run("systemd-inhibit", func(t *testing.T) {
		if _, err := exec.LookPath("systemd-inhibit"); err != nil {
			t.Skip("systemd-inhibit not installed")
		}
		h, err := systemdInhibit(context.Background(), "sleep", "keepalive: fallback")
		if err != nil {
			t.Fatalf("systemdInhibit: %v", err)
		}
		lock := "sleep|keepalive|keepalive: fallback|block"
		if got := logindInhibitors(t, sys); !slices.Contains(got, lock) {
			t.Fatalf("logind inhibitors = %v, want %q", got, lock)
		}
		if err := h.Release(); err != nil {
			t.Fatalf("Release: %v", err)
		}
		waitFor(t, "fallback lock released", func() bool { return !slices.Contains(logindInhibitors(t, sys), lock) })
	})

	t.Run("systemd-inhibit dies with owner", func(t *testing.T) {
		if _, err := exec.LookPath("systemd-inhibit"); err != nil {
			t.Skip("systemd-inhibit not installed")
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestDBusMockHelper$")
		cmd.Env = append(os.Environ(), "KEEPALIVE_DBUSMOCK_HELPER=1")
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer cmd.Wait()
		defer cmd.Process.Kill()
		if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
			t.Fatalf("helper said %q, %v", line, err)
		}
		lock := "sleep:idle|keepalive|keepalive: helper|block"
		if got := logindInhibitors(t, sys); !slices.Contains(got, lock) {
			t.Fatalf("logind inhibitors = %v, want %q", got, lock)
		}
		cmd.Process.Signal(syscall.SIGKILL)
		waitFor(t, "lock released after SIGKILL", func() bool { return !slices.Contains(logindInhibitors(t, sys), lock) })
	})

	t.Run("mechanisms", func(t *testing.T) {
		for _, m := range Mechanisms() {
			if m.Name != "systemd-inhibit" && !m.Available {
				t.Errorf("%s unavailable: %s", m.Name, m.Detail)
			}
		}
	})
}

// TestDBusMockHelper is not a test: re-executed with
// KEEPALIVE_DBUSMOCK_HELPER=1 it takes the systemd-inhibit lock and blocks
// until it is killed.
func TestDBusMockHelper(t *testing.T) {
	if os.Getenv("KEEPALIVE_DBUSMOCK_HELPER") == "" {
		t.Skip("helper process")
	}
	if _, err := systemdInhibit(context.Background(), "sleep:idle", "keepalive: helper"); err != nil {
		fmt.Println("error", err)
		os.Exit(1)
	}
	fmt.Println("ready")
	select {}
}

type mockService struct{ name, path string }

// sessionMocks: ScreenSaver, gnome-session, PowerManagement (as mock.py
// starts them).
var sessionMocks = []mockService{
	{"org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver"},
	{"org.gnome.SessionManager", "/org/gnome/SessionManager"},
	{"org.freedesktop.PowerManagement", "/org/freedesktop/PowerManagement/Inhibit"},
}

func mockObject(c *dbus.Conn, m mockService) dbus.BusObject {
	return c.Object(m.name, dbus.ObjectPath(m.path))
}

type mockCall struct {
	Time   uint64
	Method string
	Args   []dbus.Variant
}

func mockCalls(t *testing.T, c *dbus.Conn, m mockService) []mockCall {
	t.Helper()
	var calls []mockCall
	if err := mockObject(c, m).Call("org.freedesktop.DBus.Mock.GetCalls", 0).Store(&calls); err != nil {
		t.Fatalf("GetCalls(%s): %v", m.name, err)
	}
	return calls
}

// assertMockCall checks that m logged method with args formatted as
// fmt.Sprint of their values.
func assertMockCall(t *testing.T, c *dbus.Conn, m mockService, method, args string) {
	t.Helper()
	var seen []string
	for _, call := range mockCalls(t, c, m) {
		vals := make([]any, len(call.Args))
		for i, a := range call.Args {
			vals[i] = a.Value()
		}
		got := call.Method + fmt.Sprint(vals)
		if got == method+args {
			return
		}
		seen = append(seen, got)
	}
	t.Fatalf("%s: no call %s%s; calls: %v", m.name, method, args, seen)
}

func nameHasOwner(t *testing.T, c *dbus.Conn, name string) bool {
	t.Helper()
	var ok bool
	if err := c.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// logindInhibitors returns the mock's locks as "what|who|why|mode".
func logindInhibitors(t *testing.T, c *dbus.Conn) []string {
	t.Helper()
	var list []struct {
		What, Who, Why, Mode string
		UID, PID             uint32
	}
	if err := c.Object(logindName, logindPath).Call("org.freedesktop.login1.Manager.ListInhibitors", 0).Store(&list); err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(list))
	for i, l := range list {
		out[i] = strings.Join([]string{l.What, l.Who, l.Why, l.Mode}, "|")
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting: %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

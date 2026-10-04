package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/stigoleg/keep-alive/v2/internal/service"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

type fakeManager struct {
	installed   *service.Spec
	uninstalled int
	state       service.State
	err         error
}

func (m *fakeManager) Name() string { return "launchd" }

func (m *fakeManager) Install(s service.Spec) error {
	if m.err != nil {
		return m.err
	}
	m.installed = &s
	m.state = service.State{Installed: true, Running: true, Detail: "running (pid 99)", Path: "/home/u/Library/LaunchAgents/io.github.stigoleg.keepalive.plist"}
	return nil
}

func (m *fakeManager) Uninstall() error {
	if !m.state.Installed {
		return service.ErrNotInstalled
	}
	m.uninstalled++
	m.state = service.State{Detail: "not installed"}
	return nil
}

func (m *fakeManager) Status() (service.State, error) { return m.state, nil }

func newServiceApp(t *testing.T) (*testApp, *fakeManager) {
	t.Helper()
	ta := newTestApp(t)
	m := &fakeManager{state: service.State{Detail: "not installed"}}
	ta.ServiceManager = func() (service.Manager, error) { return m, nil }
	ta.ResolveExecutable = func() (string, string, error) { return "/opt/homebrew/bin/keepalive", "", nil }
	return ta, m
}

func TestServiceInstallSerializesChangedFlags(t *testing.T) {
	ta, m := newServiceApp(t)
	code := ta.run("service", "install", "-d", "2h", "-a", "--schedule", "Mon-Fri 08:00-16:00",
		"--keep-display=false", "--pid", "12", "--pid", "34", "--until", "17:00")
	if code != ExitUsage { // -d and --until together, like the root command
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if m.installed != nil {
		t.Fatal("installed despite a usage error")
	}

	ta, m = newServiceApp(t)
	code = ta.run("service", "install", "-a", "--schedule", "Mon-Fri 08:00-16:00", "--keep-display=false",
		"--pid", "12", "--pid", "34", "--until", "17:00", "--active-idle", "3m", "--log")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	got := strings.Join(m.installed.Args, " ")
	want := "--active --active-idle=3m0s --clock=17:00 --keep-display=false --log --pid=12 --pid=34 --schedule=Mon-Fri 08:00-16:00 --origin service"
	if got != want {
		t.Fatalf("args = %q\nwant   %q", got, want)
	}
	if m.installed.Executable != "/opt/homebrew/bin/keepalive" || m.installed.LogPath == "" {
		t.Fatalf("spec = %+v", m.installed)
	}
	out := ta.stdout.String()
	for _, want := range []string{
		"installed the login service (launchd)",
		"/home/u/Library/LaunchAgents/io.github.stigoleg.keepalive.plist",
		"config file",
		"keepalive service status",
		"keepalive status",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestServiceInstallWithoutFlags(t *testing.T) {
	ta, m := newServiceApp(t)
	if code := ta.run("service", "install"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if got := strings.Join(m.installed.Args, " "); got != "--origin service" {
		t.Fatalf("args = %q", got)
	}
}

func TestServiceInstallValidatesLikeRoot(t *testing.T) {
	for _, args := range [][]string{
		{"-d", "0"},
		{"--schedule", "Mnday 08:00-16:00"},
		{"-b", "100"},
		{"--while", "nothing"},
		{"extra"},
	} {
		ta, m := newServiceApp(t)
		if code := ta.run(append([]string{"service", "install"}, args...)...); code != ExitUsage {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
		if m.installed != nil {
			t.Errorf("%v: installed", args)
		}
	}
}

func TestServiceInstallErrors(t *testing.T) {
	ta, m := newServiceApp(t)
	ta.ResolveExecutable = func() (string, string, error) {
		return "", "", errors.New("service: /tmp/go-build1/exe/keepalive is a temporary build")
	}
	if code := ta.run("service", "install"); code != ExitFailure || m.installed != nil {
		t.Fatalf("exit %d", code)
	}

	ta, m = newServiceApp(t)
	m.err = &service.CommandError{Command: "launchctl bootstrap", Err: errors.New("exit status 5"), Hint: "check plutil -lint"}
	if code := ta.run("service", "install"); code != ExitFailure {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(ta.stderr.String(), "hint: check plutil -lint") {
		t.Fatalf("stderr = %q", ta.stderr)
	}

	ta, _ = newServiceApp(t)
	ta.ResolveExecutable = func() (string, string, error) {
		return `C:\keepalive\keepalive.exe`, "keepalivew.exe was not found", nil
	}
	if code := ta.run("service", "install"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(ta.stderr.String(), "warning: keepalivew.exe was not found") {
		t.Fatalf("stderr = %q", ta.stderr)
	}
}

func TestServiceUninstallStopsTheServiceInstance(t *testing.T) {
	ta, m := newServiceApp(t)
	if code := ta.run("service", "uninstall"); code != ExitOK || !strings.Contains(ta.stdout.String(), "not installed") {
		t.Fatalf("uninstall without a service: exit %d: %s", code, ta.stdout)
	}

	ta.run("service", "install")
	f := startInstance(t, timedSnap()) // origin service
	ta.stdout.Reset()
	if code := ta.run("service", "uninstall"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if m.uninstalled != 1 || len(f.calls) != 1 || f.calls[0] != "stop "+string(session.ReasonIPC) {
		t.Fatalf("uninstalled %d, calls %v", m.uninstalled, f.calls)
	}
	if !strings.Contains(ta.stdout.String(), "removed the login service") {
		t.Fatalf("output = %q", ta.stdout)
	}
}

func TestServiceStatus(t *testing.T) {
	ta, _ := newServiceApp(t)
	if code := ta.run("service", "status"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if out := ta.stdout.String(); !strings.Contains(out, "not installed") || !strings.Contains(out, "keepalive service install") {
		t.Fatalf("output = %q", out)
	}

	ta.run("service", "install")
	startInstance(t, timedSnap())
	ta.stdout.Reset()
	if code := ta.run("service", "status"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	out := ta.stdout.String()
	for _, want := range []string{"launchd", "running (pid 99)", "io.github.stigoleg.keepalive.plist", "started by the login service", "2h0m left"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

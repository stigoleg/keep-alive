package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
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
		"-b", "20", "--active-idle", "3m", "--log")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	got := strings.Join(m.installed.Args, " ")
	want := "--active --active-idle=3m0s --battery=20 --keep-display=false --log --schedule=Mon-Fri 08:00-16:00 --origin service"
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

// TestServiceInstallWarnsAboutConfiguredDuration: the service ignores a
// duration from the config file; install says so.
func TestServiceInstallWarnsAboutConfiguredDuration(t *testing.T) {
	ta, m := newServiceApp(t)
	if err := writeConfig(ta, "duration = \"2h\"\n"); err != nil {
		t.Fatal(err)
	}
	if code := ta.run("service", "install"); code != ExitOK || m.installed == nil {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	want := "keepalive: warning: duration from the config file is ignored by the service; use schedule\n" +
		"hint: remove \"duration\" from " + ta.configPath + "\n"
	if got := ta.stderr.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}

	ta, _ = newServiceApp(t)
	ta.env["KEEPALIVE_DURATION"] = "90" // the service does not inherit the shell's environment
	if code := ta.run("service", "install"); code != ExitOK || ta.stderr.Len() != 0 {
		t.Fatalf("env: exit %d, stderr %q", code, ta.stderr)
	}
}

func TestServiceInstallValidatesLikeRoot(t *testing.T) {
	for _, args := range [][]string{
		{"-d", "0"},
		{"--schedule", "Mnday 08:00-16:00"},
		{"-b", "101"}, // a service may start below the threshold: it pauses
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

func TestServiceInstallMakesLogFileAbsolute(t *testing.T) {
	ta, m := newServiceApp(t)
	if code := ta.run("service", "install", "--log-file", "logs/k.log"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	wd, _ := os.Getwd()
	want := "--log-file=" + filepath.Join(wd, "logs", "k.log")
	if got := strings.Join(m.installed.Args, " "); !strings.Contains(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
	ta, m = newServiceApp(t)
	if code := ta.run("service", "install", "--log-file=~/k.log"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if got := strings.Join(m.installed.Args, " "); !strings.Contains(got, "--log-file=~/k.log") {
		t.Fatalf("args = %q; ~ is expanded when the service starts", got)
	}
}

func TestServiceLogFileRelativeToConfigFile(t *testing.T) {
	for origin, want := range map[string]string{
		"service":  "", // set below: next to the config file
		"terminal": "logs/k.log",
	} {
		ta := newTestApp(t)
		if err := writeConfig(ta, "log_file = \"logs/k.log\"\n"); err != nil {
			t.Fatal(err)
		}
		if want == "" {
			want = filepath.Join(filepath.Dir(ta.configPath), "logs", "k.log")
		}
		if code := ta.run("--plain", "--origin", origin); code != ExitOK {
			t.Fatalf("%s: exit %d: %s", origin, code, ta.stderr)
		}
		if ta.plan.Logging.Path != want {
			t.Errorf("%s: log path %q, want %q", origin, ta.plan.Logging.Path, want)
		}
	}
}

func TestServiceLogFileFallsBackToTheDefault(t *testing.T) {
	ta := newTestApp(t)
	n := &recordingNotifier{}
	ta.Notifier = n
	var tried []string
	ta.logSetup = func(o logging.Options) (string, func() error, error) {
		tried = append(tried, o.Path)
		if o.Path != "" {
			return "", nil, errors.New("permission denied")
		}
		return "/default/keepalive.log", func() error { return nil }, nil
	}
	p := &Plan{Origin: ipc.OriginService, Notify: true, Logging: logging.Options{Enabled: true, Path: "/locked/k.log"}}
	for range 2 { // a restarted service
		closeLog, err := ta.startLogging(p)
		if err != nil {
			t.Fatalf("startLogging = %v; a service must not fail (and restart) over its log", err)
		}
		closeLog()
	}
	if strings.Join(tried, ",") != "/locked/k.log,,/locked/k.log," {
		t.Fatalf("log paths tried: %q", tried)
	}
	if len(n.sent) != 1 || !strings.Contains(n.sent[0], "/locked/k.log") || !strings.Contains(n.sent[0], "/default/keepalive.log") {
		t.Fatalf("notifications = %q, want one naming both files", n.sent)
	}

	// In a terminal it stays an error.
	p.Origin = ipc.OriginTerminal
	if _, err := ta.startLogging(p); err == nil {
		t.Fatal("terminal: no error for an unwritable log file")
	}
}

func TestServiceInstallNextToARunningKeepalive(t *testing.T) {
	const warning = "keepalive: warning: another keepalive is running (pid "
	stopped := func(f *fakeInstance) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return slices.Contains(f.calls, "stop "+string(session.ReasonIPC))
	}
	end := func(f *fakeInstance) { f.srv.Close(); <-f.stopped }

	// Not a terminal: install, warn, leave the other one alone.
	ta, m := newServiceApp(t)
	f := startInstanceFrom(t, timedSnap(), ipc.OriginTerminal)
	if code := ta.run("service", "install"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	errOut := ta.stderr.String()
	if m.installed == nil || stopped(f) || !strings.Contains(errOut, warning) ||
		!strings.Contains(errOut, "the service will not start while it runs") ||
		!strings.Contains(errOut, `"keepalive stop", then "keepalive service install" again`) {
		t.Fatalf("installed %v, stopped %v, stderr %q", m.installed != nil, stopped(f), errOut)
	}
	end(f)

	// --replace stops it first.
	ta, m = newServiceApp(t)
	f = startInstanceFrom(t, timedSnap(), ipc.OriginTerminal)
	if code := ta.run("service", "install", "--replace"); code != ExitOK {
		t.Fatalf("--replace: exit %d: %s", code, ta.stderr)
	}
	if m.installed == nil || !stopped(f) || strings.Contains(ta.stderr.String(), "warning") {
		t.Fatalf("--replace: installed %v, stopped %v, stderr %q", m.installed != nil, stopped(f), ta.stderr)
	}
	if strings.Contains(strings.Join(m.installed.Args, " "), "replace") {
		t.Fatalf("--replace stored in the service: %q", m.installed.Args)
	}
	end(f)

	// On a terminal it asks.
	for answer, wantStop := range map[string]bool{"y\n": true, "\n": false, "no\n": false} {
		ta, m = newServiceApp(t)
		ta.StdinTTY, ta.StdoutTTY = true, true
		ta.Stdin = strings.NewReader(answer)
		f = startInstanceFrom(t, timedSnap(), ipc.OriginTerminal)
		if code := ta.run("service", "install"); code != ExitOK {
			t.Fatalf("%q: exit %d: %s", answer, code, ta.stderr)
		}
		if !strings.Contains(ta.stdout.String(), "Stop the running keepalive now so the service can start? [y/N] ") {
			t.Fatalf("%q: no question in %q", answer, ta.stdout)
		}
		if m.installed == nil || stopped(f) != wantStop || strings.Contains(ta.stderr.String(), warning) == wantStop {
			t.Fatalf("%q: installed %v, stopped %v, stderr %q", answer, m.installed != nil, stopped(f), ta.stderr)
		}
		end(f)
	}

	// The service's own keepalive is replaced by the install itself.
	ta, m = newServiceApp(t)
	ta.StdinTTY, ta.StdoutTTY = true, true
	f = startInstance(t, timedSnap())
	if code := ta.run("service", "install"); code != ExitOK || stopped(f) || ta.stderr.Len() != 0 ||
		strings.Contains(ta.stdout.String(), "[y/N]") {
		t.Fatalf("service instance: exit %d, stopped %v, stderr %q", code, stopped(f), ta.stderr)
	}
}

func TestServiceInstallRefusesOneShotLimits(t *testing.T) {
	for _, args := range [][]string{
		{"-d", "2h"}, {"--duration=30m"}, {"-c", "17:00"}, {"--clock", "17:00"}, {"--until", "17:00"},
		{"--pid", "12"}, {"--while", "zoom"},
	} {
		ta, m := newServiceApp(t)
		if code := ta.run(append([]string{"service", "install"}, args...)...); code != ExitUsage {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
		errOut := ta.stderr.String()
		if m.installed != nil || !strings.Contains(errOut, `cannot be used with "keepalive service install"`) ||
			!strings.Contains(errOut, "hint: a service runs at every login; use --schedule for work hours") {
			t.Errorf("%v: installed %v, stderr %q", args, m.installed != nil, errOut)
		}
	}
}

func TestServiceRunPausesAtTheBatteryThreshold(t *testing.T) {
	// The service starts at login even below the threshold (it pauses
	// then); a terminal run refuses, as before.
	ta := newTestApp(t)
	ta.Battery = func() (platform.BatteryStatus, error) {
		return platform.BatteryStatus{Percentage: 15, Available: true}, nil
	}
	if code := ta.run("--plain", "--origin", "service", "-b", "20"); code != ExitOK {
		t.Fatalf("service: exit %d: %s", code, ta.stderr)
	}
	if !ta.plan.Session.BatteryPause || ta.plan.Session.BatteryThreshold != 20 {
		t.Fatalf("service plan: %+v", ta.plan.Session)
	}
	ta = newTestApp(t)
	ta.Battery = func() (platform.BatteryStatus, error) {
		return platform.BatteryStatus{Percentage: 15, Available: true}, nil
	}
	if code := ta.run("--plain", "-b", "20"); code != ExitUsage {
		t.Fatalf("terminal below the threshold: exit %d", code)
	}
	sa, m := newServiceApp(t)
	sa.Battery = ta.Battery
	if code := sa.run("service", "install", "-b", "20"); code != ExitOK || m.installed == nil {
		t.Fatalf("install below the threshold: exit %d: %s", code, sa.stderr)
	}
	ta = newTestApp(t)
	if code := ta.run("--plain", "-b", "20"); code != ExitOK || ta.plan.Session.BatteryPause {
		t.Fatalf("terminal: exit %d, pause %v", code, ta.plan.Session.BatteryPause)
	}
}

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/proc"
	"github.com/stigoleg/keep-alive/v2/internal/service"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// fakeProcs is a process table for --pid and --while.
type fakeProcs struct {
	alive map[int]bool
	named map[string][]proc.Process
}

func (f fakeProcs) Alive(pid int) (bool, error) { return f.alive[pid], nil }

func (f fakeProcs) FindByName(name string) ([]proc.Process, error) { return f.named[name], nil }

var now = time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

func TestMain(m *testing.M) {
	time.Local = time.UTC // golden files use UTC wall clock
	// Control commands must never reach a real keepalive.
	dir, err := os.MkdirTemp("", "ka-cli")
	if err != nil {
		panic(err)
	}
	os.Setenv("KEEPALIVE_RUNTIME_DIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type testApp struct {
	*App
	stdout, stderr *bytes.Buffer
	plan           *Plan
	argv           []string
	env            map[string]string
	configPath     string
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	stateDir := t.TempDir()
	ta := &testApp{
		stdout:     &bytes.Buffer{},
		stderr:     &bytes.Buffer{},
		env:        map[string]string{},
		configPath: filepath.Join(t.TempDir(), "config.toml"),
	}
	ta.App = &App{
		Version:   "1.5.3",
		Stdin:     strings.NewReader(""),
		Stdout:    ta.stdout,
		Stderr:    ta.stderr,
		LookupEnv: func(k string) (string, bool) { v, ok := ta.env[k]; return v, ok },
		Now:       func() time.Time { return now },
		Battery: func() (platform.BatteryStatus, error) {
			return platform.BatteryStatus{Percentage: 80, Available: true}, nil
		},
		DefaultConfigPath: func() (string, error) { return ta.configPath, nil },
		// Never touch the real login service from tests.
		ServiceManager:    func() (service.Manager, error) { return nil, errors.New("no service manager in tests") },
		ResolveExecutable: func() (string, string, error) { return "", "", errors.New("no executable in tests") },
		stateDir:          func() (string, error) { return stateDir, nil },
		Processes: fakeProcs{
			alive: map[int]bool{12: true, 34: true, 56: true},
			named: map[string][]proc.Process{"zoom": {{PID: 77, Name: "zoom.us"}}},
		},
	}
	ta.App.runSession = func(_ context.Context, p *Plan) error {
		ta.plan = p
		return nil
	}
	ta.App.runChild = func(_ context.Context, p *Plan, argv []string) error {
		ta.plan, ta.argv = p, argv
		return nil
	}
	return ta
}

func (ta *testApp) run(args ...string) int { return ta.Execute(args) }

func writeConfig(ta *testApp, body string) error {
	return os.WriteFile(ta.configPath, []byte(body), 0o600)
}

// TestFlagCompatibility pins the v1.5.3 meaning of every documented flag
// combination.
func TestFlagCompatibility(t *testing.T) {
	at := func(day, hour, minute int) time.Time { return time.Date(2024, 1, day, hour, minute, 0, 0, time.Local) }
	tests := []struct {
		args     []string
		duration time.Duration
		until    time.Time
		battery  int
		active   bool
		log      bool
	}{
		{args: nil},
		{args: []string{"-d", "150"}, duration: 150 * time.Minute},
		{args: []string{"-d", "2h30m"}, duration: 150 * time.Minute},
		{args: []string{"--duration=45m"}, duration: 45 * time.Minute},
		{args: []string{"--duration", "45m"}, duration: 45 * time.Minute},
		{args: []string{"-d20"}, duration: 20 * time.Minute},
		{args: []string{"-c", "22:00"}, until: at(1, 22, 0)},
		{args: []string{"-c", "10:00PM"}, until: at(1, 22, 0)},
		{args: []string{"-c", "10:00 pm"}, until: at(1, 22, 0)},
		{args: []string{"--clock", "22:30"}, until: at(1, 22, 30)},
		{args: []string{"--until", "22:00"}, until: at(1, 22, 0)},
		{args: []string{"-c", "9:45"}, until: at(2, 9, 45)},
		{args: []string{"-c", "10:00"}, until: at(2, 10, 0)}, // the current minute rolls over
		{args: []string{"-b", "20"}, battery: 20},
		{args: []string{"--battery", "30"}, battery: 30},
		{args: []string{"-a"}, active: true},
		{args: []string{"--active"}, active: true},
		{args: []string{"-l"}, log: true},
		{args: []string{"--log"}, log: true},
		{args: []string{"-d", "20", "-b", "65", "-a"}, duration: 20 * time.Minute, battery: 65, active: true},
		{args: []string{"-c", "22:30", "-b", "50"}, until: at(1, 22, 30), battery: 50},
		{args: []string{"-a", "-l", "-d", "1h"}, duration: time.Hour, active: true, log: true},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			ta := newTestApp(t)
			if code := ta.run(tt.args...); code != ExitOK {
				t.Fatalf("exit %d, stderr: %s", code, ta.stderr)
			}
			p := ta.plan
			if p == nil {
				t.Fatal("no session planned")
			}
			s := p.Session
			if s.Duration != tt.duration || !s.Until.Equal(tt.until) || s.BatteryThreshold != tt.battery || s.Active != tt.active {
				t.Fatalf("session = %+v", s)
			}
			if p.Logging.Enabled != tt.log || p.Logging.Debug != tt.log {
				t.Fatalf("logging = %+v, want enabled=%v", p.Logging, tt.log)
			}
			if !s.KeepDisplay || s.Activity.IdleThreshold != 2*time.Minute || s.Activity.Interval != 30*time.Second {
				t.Fatalf("defaults lost: %+v", s)
			}
		})
	}
}

func TestVersionAndHelp(t *testing.T) {
	for _, args := range [][]string{{"-v"}, {"--version"}, {"version"}} {
		ta := newTestApp(t)
		if code := ta.run(args...); code != ExitOK {
			t.Fatalf("%v: exit %d", args, code)
		}
		if got := ta.stdout.String(); got != "keepalive 1.5.3\n" {
			t.Fatalf("%v printed %q", args, got)
		}
		if ta.plan != nil {
			t.Fatalf("%v started a session", args)
		}
	}
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		ta := newTestApp(t)
		if code := ta.run(args...); code != ExitOK {
			t.Fatalf("%v: exit %d", args, code)
		}
		out := ta.stdout.String()
		for _, want := range []string{"Usage:", "--duration", "--clock", "--battery", "--active", "--log", "--version"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v output missing %q", args, want)
			}
		}
		if ta.plan != nil {
			t.Fatalf("%v started a session", args)
		}
	}
}

func TestUsageErrorsExit2(t *testing.T) {
	tests := [][]string{
		{"-d", "0"},
		{"-d", "-5"},
		{"-d", "30s"},
		{"-d", "abc"},
		{"--duration=0m"},
		{"-d", "2h", "-c", "22:00"},
		{"-c", "25:00"},
		{"-c", "noon"},
		{"-b", "0"},
		{"-b", "101"},
		{"-b", "80"}, // current level: threshold must be below it
		{"-b", "100"},
		{"--active-idle", "5s"},
		{"--active-interval", "1s"},
		{"--schedule", ""},
		{"--schedule", "Mnday 08:00-16:00"},
		{"--schedule", "mon-fri 9-17"},
		{"--pid", "999"},
		{"--pid", "0"},
		{"--pid", "-3"},
		{"--while", "nothing"},
		{"--bogus"},
		{"-x"},
		{"--config", "/nonexistent/keepalive.toml"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ta := newTestApp(t)
			if code := ta.run(args...); code != ExitUsage {
				t.Fatalf("exit %d, want %d; stderr: %s", code, ExitUsage, ta.stderr)
			}
			if ta.plan != nil {
				t.Fatal("a session was planned")
			}
			errOut := ta.stderr.String()
			if !strings.HasPrefix(errOut, "keepalive: error: ") {
				t.Fatalf("stderr = %q", errOut)
			}
			if strings.Contains(errOut, "\x1b[") || strings.Contains(errOut, "╭") {
				t.Fatalf("styled error without a TTY: %q", errOut)
			}
		})
	}
}

func TestBatteryWithoutBatteryIsUsageError(t *testing.T) {
	ta := newTestApp(t)
	ta.Battery = func() (platform.BatteryStatus, error) {
		return platform.BatteryStatus{}, errors.New("battery percentage not found")
	}
	if code := ta.run("-b", "20"); code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(ta.stderr.String(), "no battery") {
		t.Fatalf("stderr = %q", ta.stderr)
	}
	if !strings.Contains(ta.stderr.String(), "hint: ") {
		t.Fatalf("no hint: %q", ta.stderr)
	}
}

func TestErrorColorOnlyOnTTYWithoutNoColor(t *testing.T) {
	ta := newTestApp(t)
	ta.StderrTTY = true
	ta.run("-d", "0")
	if !strings.Contains(ta.stderr.String(), "\x1b[") {
		t.Fatalf("no color on a TTY: %q", ta.stderr)
	}

	ta = newTestApp(t)
	ta.StderrTTY = true
	ta.env["NO_COLOR"] = "1"
	ta.run("-d", "0")
	if strings.Contains(ta.stderr.String(), "\x1b[") {
		t.Fatalf("color despite NO_COLOR: %q", ta.stderr)
	}
}

func TestModeSelection(t *testing.T) {
	tests := []struct {
		args       []string
		tty        bool
		tui, json_ bool
	}{
		{nil, true, true, false},
		{nil, false, false, false},
		{[]string{"--plain"}, true, false, false},
		{[]string{"--json"}, true, false, true},
		{[]string{"--json"}, false, false, true},
	}
	for _, tt := range tests {
		ta := newTestApp(t)
		ta.StdinTTY, ta.StdoutTTY = tt.tty, tt.tty
		if code := ta.run(tt.args...); code != ExitOK {
			t.Fatalf("%v: exit %d", tt.args, code)
		}
		if ta.plan.TUI != tt.tui || ta.plan.JSON != tt.json_ {
			t.Errorf("%v tty=%v: tui=%v json=%v", tt.args, tt.tty, ta.plan.TUI, ta.plan.JSON)
		}
	}

	ta := newTestApp(t)
	ta.StdinTTY = true // stdout redirected: headless
	ta.run()
	if ta.plan.TUI {
		t.Error("TUI chosen with stdout redirected")
	}
}

func TestAutoStartMatchesV1(t *testing.T) {
	for args, want := range map[string]bool{
		"":         false,
		"-a":       false,
		"-d 20":    true,
		"-b 20":    true,
		"-c 22:00": true,
	} {
		ta := newTestApp(t)
		ta.run(strings.Fields(args)...)
		if ta.plan.AutoStart != want {
			t.Errorf("%q: AutoStart = %v, want %v", args, ta.plan.AutoStart, want)
		}
	}
}

func TestSessionFlagsAreStored(t *testing.T) {
	ta := newTestApp(t)
	code := ta.run("--pid", "12", "--pid", "34,56", "--while", "zoom", "--schedule", "mon-fri 09:00-17:00", "--notify",
		"--keep-display=false", "--active-keys", "--active-idle", "90s", "--active-interval", "45s")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	s := ta.plan.Session
	if len(s.WatchPIDs) != 3 || s.WatchPIDs[2] != 56 || s.WatchProcess != "zoom" {
		t.Fatalf("session = %+v", s)
	}
	if s.Schedule == nil || s.Schedule.String() != "Mon-Fri 09:00-17:00" {
		t.Fatalf("schedule = %v", s.Schedule)
	}
	if !ta.plan.Notify || s.KeepDisplay || !s.Activity.Keys || s.Activity.IdleThreshold != 90*time.Second || s.Activity.Interval != 45*time.Second {
		t.Fatalf("plan = %+v", ta.plan)
	}
}

func TestWatchErrorsHaveHints(t *testing.T) {
	for args, want := range map[string]string{
		"--while nothing": `no process named "nothing" is running`,
		"--pid 999":       "process 999 is not running",
		"--pid 0":         "--pid: invalid process ID 0",
	} {
		ta := newTestApp(t)
		if code := ta.run(strings.Fields(args)...); code != ExitUsage {
			t.Fatalf("%s: exit %d", args, code)
		}
		errOut := ta.stderr.String()
		if !strings.Contains(errOut, want) || !strings.Contains(errOut, "hint: ") {
			t.Errorf("%s: stderr = %q", args, errOut)
		}
	}
	ta := newTestApp(t)
	ta.run("--while", "nothing")
	if !strings.Contains(ta.stderr.String(), `hint: start the app first, or check the name with "ps"`) {
		t.Errorf("stderr = %q", ta.stderr)
	}
}

func TestScheduleErrorNamesTheFlag(t *testing.T) {
	ta := newTestApp(t)
	ta.run("--schedule", "Mnday 08:00-16:00")
	if got := ta.stderr.String(); !strings.HasPrefix(got, `keepalive: error: --schedule: unknown day "Mnday"`) {
		t.Fatalf("stderr = %q", got)
	}
}

func TestEnvAndFilePrecedence(t *testing.T) {
	ta := newTestApp(t)
	if err := os.WriteFile(ta.configPath, []byte("battery = 20\nactive = true\nduration = \"2h\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ta.env["KEEPALIVE_BATTERY"] = "30"
	if code := ta.run("-c", "22:00"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	s := ta.plan.Session
	if s.BatteryThreshold != 30 || !s.Active {
		t.Fatalf("session = %+v", s)
	}
	// A configured duration yields to an explicit --clock.
	if s.Duration != 0 || s.Until.IsZero() {
		t.Fatalf("duration %v / until %v", s.Duration, s.Until)
	}

	ta = newTestApp(t)
	if err := os.WriteFile(ta.configPath, []byte("duration = 45\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ta.run()
	if ta.plan.Session.Duration != 45*time.Minute {
		t.Fatalf("file duration not applied: %v", ta.plan.Session.Duration)
	}
}

func TestRootRejectsACommand(t *testing.T) {
	ta := newTestApp(t)
	if code := ta.run("-d", "1h", "make", "release"); code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
	want := "keepalive: error: unexpected argument \"make\"\nhint: did you mean \"keepalive run -- make release\"?\n"
	if got := ta.stderr.String(); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

// TestRootCommandTypos: a mistyped command gets a command suggestion; only
// an argument close to no command points to `keepalive run`.
func TestRootCommandTypos(t *testing.T) {
	for typed, want := range map[string]string{
		"statsu":  "status",
		"stpo":    "stop",
		"doctr":   "doctor",
		"servce":  "service",
		"verison": "version",
		"extnd":   "extend",
	} {
		ta := newTestApp(t)
		if code := ta.run(typed); code != ExitUsage {
			t.Errorf("%s: exit %d, want 2", typed, code)
		}
		line := fmt.Sprintf("keepalive: error: unknown command %q — did you mean %q?\n", typed, want)
		if got := ta.stderr.String(); got != line {
			t.Errorf("%s: stderr %q, want %q", typed, got, line)
		}
	}
	ta := newTestApp(t)
	if code := ta.run("backup", "nightly"); code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
	if got, want := ta.stderr.String(), "keepalive: error: unexpected argument \"backup\"\nhint: did you mean \"keepalive run -- backup nightly\"?\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunParsesTheCommand(t *testing.T) {
	ta := newTestApp(t)
	ta.StdinTTY, ta.StdoutTTY = true, true
	if code := ta.run("run", "-d", "2h", "-a", "--", "make", "-j", "8", "--", "x"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if strings.Join(ta.argv, " ") != "make -j 8 -- x" {
		t.Fatalf("argv = %q", ta.argv)
	}
	p := ta.plan
	if p.TUI || p.Session.Duration != 2*time.Hour || !p.Session.Active || p.Session.Command != "make" {
		t.Fatalf("plan = %+v", p)
	}
	if !p.Notify {
		t.Fatal("notifications off by default for run")
	}
}

func TestRunNeedsDashDash(t *testing.T) {
	for _, args := range [][]string{
		{"run"},
		{"run", "make"},
		{"run", "-d", "1h", "make", "--", "x"},
		{"run", "--"},
	} {
		ta := newTestApp(t)
		if code := ta.run(args...); code != ExitUsage {
			t.Fatalf("%v: exit %d", args, code)
		}
		if got := ta.stderr.String(); !strings.Contains(got, "usage: keepalive run [flags] -- command [args…]") {
			t.Fatalf("%v: stderr = %q", args, got)
		}
		if ta.argv != nil {
			t.Fatalf("%v: ran %q", args, ta.argv)
		}
	}
}

func TestNotifyDefaults(t *testing.T) {
	for _, tt := range []struct {
		args []string
		env  map[string]string
		tty  bool
		want bool
	}{
		{nil, nil, true, false},                 // TUI
		{nil, nil, false, true},                 // headless
		{[]string{"--plain"}, nil, true, true},  // headless on a terminal
		{[]string{"--notify"}, nil, true, true}, // explicit wins
		{[]string{"--notify=false"}, nil, false, false},
		{nil, map[string]string{"KEEPALIVE_NOTIFY": "false"}, false, false},
		{[]string{"--origin", "service"}, nil, true, true},
	} {
		ta := newTestApp(t)
		ta.StdinTTY, ta.StdoutTTY = tt.tty, tt.tty
		for k, v := range tt.env {
			ta.env[k] = v
		}
		if code := ta.run(tt.args...); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", tt.args, code, ta.stderr)
		}
		if ta.plan.Notify != tt.want {
			t.Errorf("%v %v tty=%v: Notify = %v, want %v", tt.args, tt.env, tt.tty, ta.plan.Notify, tt.want)
		}
	}
}

func TestServiceOriginDefaults(t *testing.T) {
	ta := newTestApp(t)
	ta.StdinTTY, ta.StdoutTTY = true, true
	if code := ta.run("--origin", "service", "-a"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	p := ta.plan
	if p.Origin != "service" || p.TUI || !p.Logging.Enabled || p.Logging.Debug || !p.Notify {
		t.Fatalf("plan = %+v", p)
	}

	ta = newTestApp(t)
	ta.env["KEEPALIVE_ORIGIN"] = "service"
	ta.run("--log=false")
	if ta.plan.Origin != "service" || ta.plan.Logging.Enabled {
		t.Fatalf("explicit --log=false ignored: %+v", ta.plan.Logging)
	}

	ta = newTestApp(t)
	if code := ta.run("--origin", "cron"); code != ExitUsage {
		t.Fatalf("--origin cron: exit %d", code)
	}
}

// TestServiceIgnoresConfiguredDuration: a service that ends stays stopped
// until the next login, so a duration from the config file or the
// environment does not apply to it.
func TestServiceIgnoresConfiguredDuration(t *testing.T) {
	const fileNote = "duration from the config file is ignored by the service; use schedule"
	ta := newTestApp(t)
	if err := writeConfig(ta, "duration = \"2h\"\n"); err != nil {
		t.Fatal(err)
	}
	if code := ta.run("--plain", "--origin", "service"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if s := ta.plan.Session; s.Duration != 0 || ta.plan.AutoStart || !slices.Equal(s.StartWarnings, []string{fileNote}) {
		t.Fatalf("service plan: duration %v, autostart %v, warnings %q", s.Duration, ta.plan.AutoStart, s.StartWarnings)
	}

	ta = newTestApp(t)
	ta.env["KEEPALIVE_DURATION"] = "90"
	ta.env["KEEPALIVE_ORIGIN"] = "service"
	ta.run("--plain")
	if s := ta.plan.Session; s.Duration != 0 ||
		!slices.Equal(s.StartWarnings, []string{"duration from KEEPALIVE_DURATION is ignored by the service; use schedule"}) {
		t.Fatalf("env: duration %v, warnings %q", s.Duration, s.StartWarnings)
	}

	// An explicit flag still applies (install refuses it), and so does the
	// file outside a service.
	ta = newTestApp(t)
	writeConfig(ta, "duration = \"2h\"\n")
	ta.run("--plain", "--origin", "service", "-d", "1h")
	if s := ta.plan.Session; s.Duration != time.Hour || len(s.StartWarnings) != 0 {
		t.Fatalf("flag: duration %v, warnings %q", s.Duration, s.StartWarnings)
	}
	ta = newTestApp(t)
	writeConfig(ta, "duration = \"2h\"\n")
	ta.run("--plain")
	if s := ta.plan.Session; s.Duration != 2*time.Hour || len(s.StartWarnings) != 0 {
		t.Fatalf("terminal: duration %v, warnings %q", s.Duration, s.StartWarnings)
	}
}

// TestStartWarningsAreLoggedOnce: the plan's warnings go to the log once, at
// info level, when logging starts.
func TestStartWarningsAreLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	ta := newTestApp(t)
	ta.logSetup = func(logging.Options) (string, func() error, error) {
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
		return "", func() error { return nil }, nil
	}
	ta.Command() // wires the defaults
	p := &Plan{Origin: "service", Session: session.Config{StartWarnings: []string{"note one"}}}
	closeLog, err := ta.startLogging(p)
	if err != nil {
		t.Fatal(err)
	}
	closeLog()
	if got := buf.String(); strings.Count(got, "note one") != 1 || !strings.Contains(got, "level=INFO msg=\"note one\"") {
		t.Fatalf("log:\n%s", got)
	}
}

func TestConfigCommands(t *testing.T) {
	ta := newTestApp(t)
	if code := ta.run("config", "path"); code != ExitOK || ta.stdout.String() != ta.configPath+"\n" {
		t.Fatalf("config path: exit %d, %q", code, ta.stdout)
	}

	custom := filepath.Join(t.TempDir(), "other.toml")
	ta = newTestApp(t)
	if code := ta.run("config", "path", "--config", custom); code != ExitOK || ta.stdout.String() != custom+"\n" {
		t.Fatalf("config path --config: exit %d, %q", code, ta.stdout)
	}

	ta = newTestApp(t)
	if code := ta.run("config", "init"); code != ExitOK {
		t.Fatalf("config init: exit %d: %s", code, ta.stderr)
	}
	if _, err := os.Stat(ta.configPath); err != nil {
		t.Fatal(err)
	}
	if code := ta.run("config", "init"); code != ExitFailure || !strings.Contains(ta.stderr.String(), "--force") {
		t.Fatalf("config init twice: exit %d: %s", code, ta.stderr)
	}
	if code := ta.run("config", "init", "--force"); code != ExitOK {
		t.Fatalf("config init --force: exit %d", code)
	}

	ta.stdout.Reset()
	ta.env["KEEPALIVE_ACTIVE"] = "1"
	if code := ta.run("config", "show", "-d", "45m"); code != ExitOK {
		t.Fatalf("config show: exit %d: %s", code, ta.stderr)
	}
	out := ta.stdout.String()
	for _, want := range []string{`duration = "45m0s"`, "# flag", "active = true", "# env", "# default", ta.configPath} {
		if !strings.Contains(out, want) {
			t.Errorf("config show missing %q:\n%s", want, out)
		}
	}
}

func TestCompletion(t *testing.T) {
	for shell, want := range map[string]string{
		"zsh":        "#compdef keepalive",
		"bash":       "keepalive",
		"fish":       "complete -c keepalive",
		"powershell": "Register-ArgumentCompleter",
	} {
		ta := newTestApp(t)
		if code := ta.run("completion", shell); code != ExitOK {
			t.Fatalf("completion %s: exit %d", shell, code)
		}
		if !strings.Contains(ta.stdout.String(), want) {
			t.Errorf("completion %s missing %q", shell, want)
		}
	}
}

func TestRuntimeErrorExit1(t *testing.T) {
	ta := newTestApp(t)
	ta.runSession = func(context.Context, *Plan) error { return errors.New("boom") }
	if code := ta.run(); code != ExitFailure {
		t.Fatalf("exit %d", code)
	}
	if got := ta.stderr.String(); got != "keepalive: error: boom\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestExitErrorCarriesCode(t *testing.T) {
	err := &ExitError{Code: 42}
	if ExitCode(err) != 42 {
		t.Fatal("explicit exit code lost")
	}
	if ExitCode(nil) != ExitOK || ExitCode(errors.New("x")) != ExitUsage {
		t.Fatal("default exit codes wrong")
	}
	if ExitCode(runtimeErr(errors.New("x"), "")) != ExitFailure {
		t.Fatal("runtime error code wrong")
	}
}

func TestPowerErrorHintReachesUser(t *testing.T) {
	perr := &power.Error{Err: errors.New("logind said no"), Hint: "run it from your desktop"}
	err := resultError(session.Result{Reason: session.ReasonError, Err: fmt.Errorf("keep the system awake: %w", perr)})
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != ExitFailure || ee.Hint != "run it from your desktop" {
		t.Fatalf("resultError = %#v", err)
	}
}

func TestResolveVersion(t *testing.T) {
	if got := ResolveVersion("1.5.3"); got != "1.5.3" {
		t.Fatalf("ResolveVersion(1.5.3) = %q", got)
	}
	if got := ResolveVersion("v2.0.0"); got != "2.0.0" {
		t.Fatalf("ResolveVersion(v2.0.0) = %q", got)
	}
	if got := ResolveVersion(""); got == "" {
		t.Fatal("ResolveVersion(\"\") is empty")
	}
}

func TestEveryCommandIsDocumented(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Name() == "help" || (c.HasParent() && c.Parent().Name() == "completion") {
			return
		}
		if c.Short == "" || c.Long == "" || c.Example == "" {
			t.Errorf("%q: Short %q, Long %d bytes, Example %d bytes", c.CommandPath(), c.Short, len(c.Long), len(c.Example))
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newTestApp(t).Command())
}

func TestHelpGroupsFlags(t *testing.T) {
	ta := newTestApp(t)
	ta.run("--help")
	out := ta.stdout.String()
	session, activity, output := strings.Index(out, "Session flags:"), strings.Index(out, "Activity flags:"), strings.Index(out, "Output flags:")
	if session < 0 || activity < session || output < activity {
		t.Fatalf("flag groups missing or out of order:\n%s", out)
	}
	for _, want := range []string{`keepalive --schedule "Mon-Fri 08:00-16:00" -a`, "keepalive run -- make release", "keepalive -c 17:00 -a"} {
		if !strings.Contains(out[:session], want) {
			t.Errorf("root help does not lead with %q", want)
		}
	}
	if strings.Contains(out, "--origin") {
		t.Error("hidden --origin shows in help")
	}
}

func TestRunHelpHidesPlain(t *testing.T) {
	ta := newTestApp(t)
	if code := ta.run("run", "--help"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if out := ta.stdout.String(); strings.Contains(out, "--plain") || !strings.Contains(out, "--json") {
		t.Fatalf("run --help:\n%s", out)
	}
	ta = newTestApp(t)
	if code := ta.run("run", "--plain", "--", "true"); code != ExitOK { // still accepted
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
}

// TestEveryCommandHelp checks that "<command> --help", "-h" and "help
// <command>" show that command's own help, never the root's.
func TestEveryCommandHelp(t *testing.T) {
	var paths [][]string
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		if c.Name() == "help" {
			return
		}
		paths = append(paths, path)
		for _, sub := range c.Commands() {
			walk(sub, append(slices.Clone(path), sub.Name()))
		}
	}
	walk(newTestApp(t).Command(), nil)
	if len(paths) < 15 {
		t.Fatalf("only %d commands found", len(paths))
	}
	for _, path := range paths {
		for _, args := range [][]string{
			append(slices.Clone(path), "--help"),
			append(slices.Clone(path), "-h"),
			append([]string{"help"}, path...),
		} {
			ta := newTestApp(t)
			cmd, _, err := ta.Command().Find(path)
			if err != nil {
				t.Fatal(err)
			}
			if code := ta.run(args...); code != ExitOK {
				t.Errorf("%q: exit %d: %s", args, code, ta.stderr)
				continue
			}
			out := ta.stdout.String()
			long := strings.TrimSpace(cmd.Long)
			if long == "" {
				long = cmd.Short
			}
			first, _, _ := strings.Cut(long, "\n")
			if !strings.HasPrefix(out, first) || !strings.Contains(out, "\nUsage:\n  "+cmd.CommandPath()) {
				t.Errorf("%q does not show the help of %q:\n%s", args, cmd.CommandPath(), out)
			}
		}
	}
}

func TestGroupCommandTyposAreUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		args       []string
		suggestion string // "" when nothing is close
	}{
		{[]string{"service", "instal"}, `did you mean "keepalive service install"?`},
		{[]string{"service", "uninstal"}, `did you mean "keepalive service uninstall"?`},
		{[]string{"config", "instal"}, "use one of: init, path, show"},
		{[]string{"config", "sho"}, `did you mean "keepalive config show"?`},
		{[]string{"completion", "zssh"}, `did you mean "keepalive completion zsh"?`},
		{[]string{"service", "xyzzy", "more"}, ""},
	} {
		ta := newTestApp(t)
		if code := ta.run(tc.args...); code != ExitUsage {
			t.Errorf("%q: exit %d, want 2", tc.args, code)
		}
		errOut := ta.stderr.String()
		want := fmt.Sprintf("keepalive: error: unknown command %q for %q", tc.args[1], "keepalive "+tc.args[0])
		if !strings.HasPrefix(errOut, want) {
			t.Errorf("%q: stderr %q, want %q", tc.args, errOut, want)
		}
		if tc.suggestion != "" && !strings.Contains(errOut, "hint: "+tc.suggestion) {
			t.Errorf("%q: stderr %q, want hint %q", tc.args, errOut, tc.suggestion)
		}
		if ta.stdout.Len() != 0 {
			t.Errorf("%q printed help: %q", tc.args, ta.stdout)
		}
	}
	for _, group := range []string{"service", "config", "completion"} {
		ta := newTestApp(t)
		if code := ta.run(group); code != ExitOK || !strings.Contains(ta.stdout.String(), "Usage:") {
			t.Errorf("%s alone: exit %d, output %q", group, code, ta.stdout)
		}
	}
}

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/platform"
)

var now = time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)

type testApp struct {
	*App
	stdout, stderr *bytes.Buffer
	plan           *Plan
	env            map[string]string
	configPath     string
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
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
	}
	ta.App.runSession = func(_ context.Context, p *Plan) error {
		ta.plan = p
		return nil
	}
	return ta
}

func (ta *testApp) run(args ...string) int { return ta.Execute(args) }

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
		{"--bogus"},
		{"-x"},
		{"extra"},
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

func TestPhase4FlagsAreStored(t *testing.T) {
	ta := newTestApp(t)
	code := ta.run("--pid", "12", "--pid", "34,56", "--while", "zoom", "--schedule", "mon-fri 9-17", "--notify",
		"--keep-display=false", "--active-keys", "--active-idle", "90s", "--active-interval", "45s")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	s := ta.plan.Session
	if len(s.WatchPIDs) != 3 || s.WatchPIDs[2] != 56 || s.WatchProcess != "zoom" || s.ScheduleSpec != "mon-fri 9-17" {
		t.Fatalf("session = %+v", s)
	}
	if !ta.plan.Notify || s.KeepDisplay || !s.Activity.Keys || s.Activity.IdleThreshold != 90*time.Second || s.Activity.Interval != 45*time.Second {
		t.Fatalf("plan = %+v", ta.plan)
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

func TestStubsExit1(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--", "sleep", "1"},
		{"doctor"},
		{"doctor", "--probe", "--json"},
		{"status"},
		{"status", "--json"},
		{"stop"},
		{"service", "install", "-d", "2h"},
		{"service", "uninstall"},
		{"service", "status"},
	} {
		ta := newTestApp(t)
		if code := ta.run(args...); code != ExitFailure {
			t.Errorf("%v: exit %d, want %d", args, code, ExitFailure)
		}
		if !strings.Contains(ta.stderr.String(), "not implemented yet") {
			t.Errorf("%v: stderr %q", args, ta.stderr)
		}
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

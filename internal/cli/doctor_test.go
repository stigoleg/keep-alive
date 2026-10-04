package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/service"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s mismatch\n got:\n%s\nwant:\n%s", name, got, want)
	}
}

var doctorNow = time.Date(2026, 3, 2, 10, 30, 0, 0, time.UTC)

// macFacts is a healthy Mac with a running service instance.
func macFacts() doctorFacts {
	started := doctorNow.Add(-150 * time.Minute)
	snap := session.Snapshot{
		Running: true, StartedAt: started, Mode: session.ModeIndefinite, Active: true, InWindow: true,
		Activity: activity.Status{State: activity.StateSimulating, Method: "CoreGraphics"}, PowerHold: "IOPMAssertion(PreventUserIdleSystemSleep)",
	}
	return doctorFacts{
		Version: "2.0.0", OS: "darwin", Arch: "arm64", Cgo: true,
		ConfigPath: "/Users/jane/.config/keepalive/config.toml", ConfigFound: true,
		LogPath:    "/Users/jane/Library/Caches/keepalive/keepalive.log",
		Mechanisms: []power.Mechanism{{Name: "IOPMAssertion", Available: true, Detail: "IOKit power assertions"}},
		Activity: activity.Diagnostics{
			Injectors: []activity.InjectorCheck{{Name: "CoreGraphics", Available: true, Detail: `Accessibility granted to "Ghostty"`}},
			Idle:      []activity.IdleCheck{{Name: "HIDSystemState", Idle: 3 * time.Second}, {Name: "CombinedSessionState", Idle: 3 * time.Second}},
			Gate:      "CoreGraphics", Verifier: "CombinedSessionState",
			Lock: activity.LockCheck{Method: "session dictionary, polled every 2 s", Known: true},
		},
		Battery:     platform.BatteryStatus{Percentage: 76, Available: true},
		NotifyOK:    true,
		NotifyHow:   "Notification Center via osascript",
		ServiceName: "launchd",
		Service:     service.State{Installed: true, Running: true, Detail: "running (pid 812)", Path: "/Users/jane/Library/LaunchAgents/io.github.stigoleg.keepalive.plist"},
		Instance: &ipc.Status{PID: 812, Version: "2.0.0", Origin: "service",
			Snapshot: output.NewJSONSnapshot(snap)},
		InstanceTime: doctorNow,
	}
}

// brokenLinux has nothing that can keep it awake and no input method.
func brokenLinux() doctorFacts {
	return doctorFacts{
		Version: "2.0.0", OS: "linux", Arch: "amd64", Cgo: false,
		ConfigPath: "/home/jane/.config/keepalive/config.toml",
		ConfigErr:  errors.New("config file /home/jane/.config/keepalive/config.toml: battery: must be 1-100"),
		LogEnabled: true, LogPath: "/home/jane/.cache/keepalive/keepalive.log",
		Mechanisms: []power.Mechanism{
			{Name: "system bus", Detail: "no system bus: dial unix /run/dbus/system_bus_socket: no such file"},
			{Name: "session bus", Detail: "no session bus"},
		},
		Activity: activity.Diagnostics{
			Injectors: []activity.InjectorCheck{
				{Name: "uinput", Reason: "no write access to /dev/uinput", Hint: "give your user access to /dev/uinput"},
				{Name: "ydotool", Reason: "ydotool is not installed", Hint: "install ydotool 1.x"},
			},
			NoIdleHint:  "install xprintidle for idle-aware simulation",
			Environment: []string{"display server: Wayland", "desktop: GNOME"},
		},
		BatteryErr:  errors.New("no battery found"),
		NotifyHow:   "no D-Bus session bus; notify-send not found",
		ServiceName: "XDG autostart",
		Service:     service.State{Detail: "not installed"},
		InstanceErr: ipc.ErrNotRunning,
	}
}

func TestDoctorGolden(t *testing.T) {
	for name, f := range map[string]doctorFacts{"mac": macFacts(), "linux": brokenLinux()} {
		r := buildDoctor(f)
		var uni, ascii bytes.Buffer
		renderDoctor(&uni, r, true, false)
		renderDoctor(&ascii, r, false, false)
		golden(t, "doctor_"+name+".golden", uni.Bytes())
		golden(t, "doctor_"+name+"_ascii.golden", ascii.Bytes())
	}
	if buildDoctor(macFacts()).failed() {
		t.Error("healthy Mac reported a failure")
	}
	if !buildDoctor(brokenLinux()).failed() {
		t.Error("broken Linux reported no failure")
	}
}

func newDoctorApp(t *testing.T, f doctorFacts) *testApp {
	t.Helper()
	ta := newTestApp(t)
	ta.doctorFacts = func(*cobra.Command) doctorFacts { return f }
	ta.probe = func(context.Context) activity.ProbeResult {
		t.Fatal("probe ran")
		return activity.ProbeResult{}
	}
	return ta
}

func TestDoctorExitCodesAndJSON(t *testing.T) {
	ta := newDoctorApp(t, macFacts())
	if code := ta.run("doctor"); code != ExitOK {
		t.Fatalf("healthy: exit %d", code)
	}
	if out := ta.stdout.String(); !strings.Contains(out, "ok   CoreGraphics") || strings.Contains(out, "✓") {
		t.Fatalf("not a TTY: want the ASCII marks:\n%s", out)
	}

	ta = newDoctorApp(t, brokenLinux())
	if code := ta.run("doctor", "--json"); code != ExitFailure {
		t.Fatalf("broken: exit %d", code)
	}
	var doc struct {
		Sections []struct {
			Title  string `json:"title"`
			Checks []struct {
				Name, Status, Detail, Fix string
			} `json:"checks"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(ta.stdout.Bytes(), &doc); err != nil {
		t.Fatalf("%v:\n%s", err, ta.stdout)
	}
	titles := []string{}
	for _, s := range doc.Sections {
		titles = append(titles, s.Title)
	}
	want := "keepalive,Sleep prevention,Activity simulation,Battery,Notifications,Background service,Running instance"
	if got := strings.Join(titles, ","); got != want {
		t.Fatalf("sections = %s", got)
	}
	c := doc.Sections[1].Checks[0]
	if c.Status != "fail" || c.Fix == "" || c.Detail == "" {
		t.Fatalf("sleep prevention check = %+v", c)
	}
}

func TestDoctorUnicodeOnUTF8Terminal(t *testing.T) {
	ta := newDoctorApp(t, macFacts())
	ta.StdoutTTY = true
	ta.env["LANG"] = "en_US.UTF-8"
	ta.run("doctor")
	if !strings.Contains(ta.stdout.String(), "✓") {
		t.Fatalf("no ✓ on a UTF-8 terminal:\n%s", ta.stdout)
	}
	ta = newDoctorApp(t, macFacts())
	ta.StdoutTTY = true
	ta.env["LANG"] = "en_US.UTF-8"
	ta.env["NO_COLOR"] = "1"
	ta.run("doctor")
	if strings.Contains(ta.stdout.String(), "✓") {
		t.Fatalf("✓ despite NO_COLOR:\n%s", ta.stdout)
	}
}

func TestDoctorProbe(t *testing.T) {
	ta := newDoctorApp(t, macFacts())
	if code := ta.run("doctor", "--probe"); code != ExitUsage {
		t.Fatalf("--probe without a terminal: exit %d", code)
	}
	if !strings.Contains(ta.stderr.String(), "--yes") {
		t.Fatalf("stderr = %q", ta.stderr)
	}

	for _, effective := range []bool{true, false} {
		ta = newDoctorApp(t, macFacts())
		ta.probeWait = 0
		ta.probe = func(context.Context) activity.ProbeResult {
			return activity.ProbeResult{
				Method: "CoreGraphics", Verifier: "CombinedSessionState", Effective: effective, Burst: 640 * time.Millisecond,
				LockKnown: true,
				Sources: []activity.ProbeReading{
					{Source: "HIDSystemState", Before: 4200 * time.Millisecond, After: 210 * time.Millisecond},
					{Source: "CombinedSessionState", Before: 4200 * time.Millisecond, After: 210 * time.Millisecond},
				},
				Reason: map[bool]string{false: "macOS ignored the synthetic input"}[effective],
				Hint:   map[bool]string{false: `check that "Ghostty" is switched on under Accessibility`}[effective],
			}
		}
		code := ta.run("doctor", "--probe", "--yes")
		out := ta.stdout.String()
		if !strings.Contains(ta.stderr.String(), "moving the mouse in 3 seconds to verify activity simulation… (Ctrl+C to cancel)") {
			t.Fatalf("stderr = %q", ta.stderr)
		}
		if !strings.Contains(out, "4.2s → 210ms") {
			t.Fatalf("no readings:\n%s", out)
		}
		if effective {
			if code != ExitOK || !strings.Contains(out, "input resets the idle timer that Teams/Slack read") {
				t.Fatalf("effective: exit %d:\n%s", code, out)
			}
		} else if code != ExitFailure || !strings.Contains(out, "CombinedSessionState was not reset: macOS ignored the synthetic input") ||
			!strings.Contains(out, `fix: check that "Ghostty"`) {
			t.Fatalf("ineffective: exit %d:\n%s", code, out)
		}
	}
}

const xwaylandNote = "Apps running under XWayland (some Slack/Teams builds) may not see this activity; start them with --ozone-platform=wayland"

// waylandLinux is GNOME on Wayland with XWayland and a running instance
// whose bursts did not reach XWayland.
func waylandLinux(gate string) doctorFacts {
	f := brokenLinux()
	f.ConfigErr = nil
	f.Activity = activity.Diagnostics{
		Injectors:   []activity.InjectorCheck{{Name: "uinput", Available: true}},
		Idle:        []activity.IdleCheck{{Name: "xprintidle (XWayland)", Idle: 3 * time.Second}},
		Gate:        gate,
		Verifier:    gate,
		Environment: []string{"display server: Wayland (with XWayland)", "desktop: KDE"},
		Notes:       []string{"XWayland: " + xwaylandNote},
	}
	snap := session.Snapshot{
		Running: true, StartedAt: doctorNow.Add(-time.Hour), Mode: session.ModeIndefinite, Active: true, InWindow: true,
		Activity: activity.Status{State: activity.StateSimulating, Method: "uinput", Hint: xwaylandNote},
	}
	f.InstanceErr = nil
	f.Instance = &ipc.Status{PID: 77, Version: "2.0.0", Origin: "terminal", Snapshot: output.NewJSONSnapshot(snap)}
	f.InstanceTime = doctorNow
	return f
}

func TestDoctorShowsXWaylandWarnings(t *testing.T) {
	var buf bytes.Buffer
	renderDoctor(&buf, buildDoctor(waylandLinux("")), false, false)
	out := buf.String()
	for _, want := range []string{
		"warn XWayland",
		xwaylandNote,
		"warn idle time",
		"activity is simulated on a fixed schedule",
		"xprintidle (XWayland) 3s",
		"warn instance",
		"fix: " + xwaylandNote,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}

	buf.Reset()
	f := waylandLinux("Mutter IdleMonitor")
	f.Activity.Idle = append([]activity.IdleCheck{{Name: "Mutter IdleMonitor", Idle: 3 * time.Second}}, f.Activity.Idle...)
	renderDoctor(&buf, buildDoctor(f), false, false)
	if out := buf.String(); !strings.Contains(out, "ok   idle time") || !strings.Contains(out, "checked against Mutter IdleMonitor") {
		t.Errorf("GNOME Wayland idle row:\n%s", out)
	}
}

func TestDoctorProbeShowsReadingNote(t *testing.T) {
	r := &doctorReport{}
	probeSection(r, activity.ProbeResult{
		Method: "uinput", Verifier: "Mutter IdleMonitor", Effective: true, Burst: time.Second,
		Sources: []activity.ProbeReading{
			{Source: "Mutter IdleMonitor", Before: 5 * time.Minute, After: 200 * time.Millisecond},
			{Source: "xprintidle (XWayland)", Before: 5 * time.Minute, After: 5 * time.Minute, Note: xwaylandNote},
		},
	})
	var buf bytes.Buffer
	renderDoctor(&buf, *r, false, false)
	out := buf.String()
	if !strings.Contains(out, "warn xprintidle (XWayland)") || !strings.Contains(out, "fix: "+xwaylandNote) ||
		!strings.Contains(out, "ok   verdict") {
		t.Fatalf("probe section:\n%s", out)
	}
}

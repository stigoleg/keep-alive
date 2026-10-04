package tui

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/schedule"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

var update = flag.Bool("update", false, "rewrite golden files")

var goldenWidths = []int{64, 40}

// golden compares the view at every width with testdata/<name>_<width>.golden
// and checks that no line is wider than the terminal.
func golden(t *testing.T, name string, build func(t *testing.T, width int) *harness) {
	t.Helper()
	for _, w := range goldenWidths {
		h := build(t, w)
		view := h.m.View()
		for i, l := range strings.Split(view, "\n") {
			if lipgloss.Width(l) > w {
				t.Errorf("%s at %d: line %d is %d wide: %q", name, w, i+1, lipgloss.Width(l), l)
			}
		}
		got := []byte(view + "\n")
		path := filepath.Join("testdata", fmt.Sprintf("%s_%d.golden", name, w))
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (run with -update to create)", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s at %d columns:\n got:\n%s\nwant:\n%s", name, w, got, want)
		}
	}
}

func workHours(t *testing.T) *schedule.Schedule {
	t.Helper()
	s, err := schedule.Parse("Mon-Fri 08:00-16:00")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGoldenHome(t *testing.T) {
	golden(t, "home_warning", func(t *testing.T, w int) *harness {
		o := testOptions(&starter{})
		o.Base = session.Config{Active: true, KeepDisplay: true, Duration: 2 * time.Hour,
			Until: time.Date(2026, 10, 5, 17, 0, 0, 0, time.UTC), Schedule: workHours(t)}
		o.ActivityProblem = func() (string, string) {
			return `Accessibility is off for "Ghostty", so activity can't move the pointer`,
				"System Settings → Privacy & Security → Accessibility → Ghostty"
		}
		h := newHarness(t, o, w)
		h.press("up", "up", "up")
		return h
	})
}

func TestGoldenInputErrors(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		open       []string
	}{
		{"input_duration_error", "9x", []string{"down", "enter"}},
		{"input_until_error", "25:00", []string{"down", "down", "enter"}},
		{"input_schedule_error", "Mnday 08:00-16:00", []string{"down", "down", "down", "enter"}},
		{"input_battery_error", "80", []string{"b"}},
	} {
		golden(t, tc.name, func(t *testing.T, w int) *harness {
			h := newHarness(t, testOptions(&starter{}), w)
			h.press(tc.open...)
			h.typeText(tc.text)
			h.press("enter")
			return h
		})
	}
	golden(t, "input_schedule_preview", func(t *testing.T, w int) *harness {
		h := newHarness(t, testOptions(&starter{}), w)
		h.press("down", "down", "down", "enter")
		h.typeText("weekdays 8:00-11:30, 12:00-16:00")
		return h
	})
}

// dashboard starts a session from cfg and shows snap on its dashboard.
func dashboard(t *testing.T, w int, cfg session.Config, snap session.Snapshot) *harness {
	t.Helper()
	st := &starter{}
	o := testOptions(st)
	o.Base, o.Start = cfg, true
	h := newHarness(t, o, w)
	h.update(snapMsg{c: st.last(t), snap: snap})
	return h
}

func indefinite(a activity.Status) session.Snapshot {
	s := runningSnap(session.Config{Active: true, KeepDisplay: true})
	s.Activity = a
	return s
}

func TestGoldenDashboard(t *testing.T) {
	cfg := session.Config{Active: true, KeepDisplay: true}
	full := func() session.Snapshot {
		s := runningSnap(session.Config{Active: true, KeepDisplay: true, BatteryThreshold: 20})
		s.StartedAt, s.EndsAt, s.Mode = testNow.Add(-108*time.Minute), testNow.Add(72*time.Minute), session.ModeUntil
		s.Activity = activity.Status{State: activity.StateSimulating, Method: "CoreGraphics", LastBurst: testNow.Add(-12 * time.Second)}
		s.Schedule, s.NextChange = "Mon-Fri 08:00-16:00", time.Date(2026, 10, 5, 16, 0, 0, 0, time.UTC)
		s.Battery = session.Battery{Percent: 76, Available: true, Threshold: 20}
		s.Watching = "zoom"
		return s
	}
	cases := map[string]session.Snapshot{
		"dash_simulating":    full(),
		"dash_waiting":       indefinite(activity.Status{State: activity.StateWaitingIdle, Idle: 70 * time.Second}),
		"dash_paused_user":   indefinite(activity.Status{State: activity.StatePausedUser, Method: "CoreGraphics"}),
		"dash_paused_locked": indefinite(activity.Status{State: activity.StatePausedLocked, Method: "CoreGraphics"}),
		"dash_degraded": indefinite(activity.Status{State: activity.StateDegraded,
			Reason: "Accessibility permission is missing", Hint: `Grant Accessibility to "Ghostty" in System Settings → Privacy & Security → Accessibility`}),
		"dash_off": func() session.Snapshot {
			s := indefinite(activity.Status{State: activity.StateOff})
			s.Active, s.KeepDisplay = false, false
			return s
		}(),
		"dash_outside_work_hours": func() session.Snapshot {
			s := indefinite(activity.Status{State: activity.StateOff})
			s.Schedule, s.InWindow, s.PowerHold = "Mon-Fri 08:00-16:00", false, ""
			s.NextChange = time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)
			return s
		}(),
		"dash_battery_paused": func() session.Snapshot {
			s := indefinite(activity.Status{State: activity.StateOff})
			s.Battery = session.Battery{Percent: 18, Available: true, Threshold: 20, Pause: true}
			s.PowerHold, s.Paused = "", session.PauseBattery
			return s
		}(),
		"dash_power_lost": func() session.Snapshot {
			s := indefinite(activity.Status{State: activity.StateWaitingIdle, Idle: 5 * time.Second})
			s.PowerHold = ""
			return s
		}(),
	}
	for name, snap := range cases {
		golden(t, name, func(t *testing.T, w int) *harness { return dashboard(t, w, cfg, snap) })
	}

	golden(t, "dash_ended", func(t *testing.T, w int) *harness {
		st := &starter{}
		h := newHarness(t, testOptions(st), w)
		h.press("down", "enter")
		h.typeText("2h")
		h.press("enter")
		c := st.last(t)
		h.update(snapMsg{c: c, snap: full()})
		c.end(session.ReasonUntil, "end time reached")
		h.settle()
		return h
	})
}

func TestGoldenAttached(t *testing.T) {
	golden(t, "dash_attached", func(t *testing.T, w int) *harness {
		h, _ := attachedHarness(t, timedSnap(), w)
		h.update(tickMsgFor(h))
		h.press("s")
		return h
	})
}

func tickMsgFor(h *harness) tickMsg { return tickMsg{gen: h.m.tick} }

func TestGoldenHelp(t *testing.T) {
	golden(t, "help_home", func(t *testing.T, w int) *harness {
		h := newHarness(t, testOptions(&starter{}), w)
		h.press("?")
		return h
	})
	golden(t, "help_dashboard", func(t *testing.T, w int) *harness {
		st := &starter{}
		o := testOptions(st)
		o.LogEnabled = true
		h := newHarness(t, o, w)
		h.press("enter", "?")
		return h
	})
}

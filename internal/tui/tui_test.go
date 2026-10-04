package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/config"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/schedule"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

func TestInputsTakeEveryPrintableKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		open []string
	}{
		{"duration", []string{"down", "enter"}},
		{"until", []string{"down", "down", "enter"}},
		{"work hours", []string{"down", "down", "down", "enter"}},
		{"battery", []string{"b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &starter{}
			h := newHarness(t, testOptions(st), 64)
			h.press(tc.open...)
			if h.m.screen != screenInput {
				t.Fatalf("screen %v, want input", h.m.screen)
			}
			h.typeText("hq?ak d")
			if h.quit || h.m.help || h.m.screen != screenInput {
				t.Fatalf("a key left the input: quit=%v help=%v screen=%v", h.quit, h.m.help, h.m.screen)
			}
			if got := h.m.input.fields[h.m.input.kind].Value(); got != "hq?ak d" {
				t.Fatalf("input = %q", got)
			}
		})
	}
}

func TestDurationWithH(t *testing.T) {
	st := &starter{}
	h := newHarness(t, testOptions(st), 64)
	h.press("down", "enter")
	h.typeText("2h30m")
	h.press("enter")
	if h.m.screen != screenDashboard {
		t.Fatalf("screen %v, want dashboard", h.m.screen)
	}
	if got := st.last(t).cfg.Duration; got != 150*time.Minute {
		t.Fatalf("duration %v", got)
	}
	if h.m.home.lastDuration != 150*time.Minute {
		t.Fatalf("last duration %v", h.m.home.lastDuration)
	}
}

func TestEscOnEachScreen(t *testing.T) {
	st := &starter{}
	h := newHarness(t, testOptions(st), 64)

	h.press("esc") // Home: nothing to leave
	if h.quit || h.m.screen != screenHome {
		t.Fatalf("esc on Home: quit=%v screen=%v", h.quit, h.m.screen)
	}

	h.press("down", "enter") // input: back to Home, text kept
	h.typeText("4")
	h.press("esc")
	if h.m.screen != screenHome {
		t.Fatalf("esc on input: screen %v", h.m.screen)
	}
	h.press("enter")
	if got := h.m.input.fields[inputDuration].Value(); got != "4" || h.m.screen != screenInput {
		t.Fatalf("input after esc = %q (screen %v)", got, h.m.screen)
	}
	h.typeText("5")
	h.press("enter") // 45 minutes

	h.press("?") // help: esc closes it, the session keeps running
	h.press("esc")
	if h.m.help || h.m.screen != screenDashboard {
		t.Fatalf("esc on help: help=%v screen=%v", h.m.help, h.m.screen)
	}

	c := st.last(t)
	h.press("esc") // dashboard: stop, back to Home
	h.settle()
	if !c.has("stop") || h.m.screen != screenHome || h.quit {
		t.Fatalf("esc on dashboard: calls=%v screen=%v quit=%v", c.Calls(), h.m.screen, h.quit)
	}
}

func TestCtrlCQuitsEverywhere(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  []string
		check func(*testing.T, *harness, *starter)
	}{
		{"home", nil, nil},
		{"input", []string{"down", "enter"}, nil},
		{"help", []string{"?"}, nil},
		{"dashboard", []string{"enter"}, func(t *testing.T, h *harness, st *starter) {
			_ = h.m.Shutdown()
			if c := st.last(t); !c.has("close") {
				t.Fatalf("calls %v: the session was not stopped", c.Calls())
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &starter{}
			h := newHarness(t, testOptions(st), 64)
			h.press(tc.keys...)
			h.press("ctrl+c")
			if !h.quit {
				t.Fatal("ctrl+c did not quit")
			}
			if tc.check != nil {
				tc.check(t, h, st)
			}
		})
	}

	t.Run("attached confirm", func(t *testing.T) {
		h, c := attachedHarness(t, timedSnap())
		h.press("s", "ctrl+c")
		if !h.quit {
			t.Fatal("ctrl+c did not quit")
		}
		_ = h.m.Shutdown()
		if c.has("stop") || !c.has("close") {
			t.Fatalf("calls %v: want a detach, not a stop", c.Calls())
		}
	})
}

func TestActivityToggle(t *testing.T) {
	st := &starter{}
	h := newHarness(t, testOptions(st), 64)
	h.press("enter")
	c := st.last(t)
	h.press("a")
	h.settle()
	if !c.has("active true") || !h.m.dash.snap.Active || !h.m.home.opts.active {
		t.Fatalf("a: calls=%v snap.Active=%v option=%v", c.Calls(), h.m.dash.snap.Active, h.m.home.opts.active)
	}
	if !strings.Contains(h.m.View(), "activity simulation on") {
		t.Fatalf("no feedback:\n%s", h.m.View())
	}
	h.press("a")
	h.settle()
	if !c.has("active false") || h.m.home.opts.active {
		t.Fatalf("a again: calls=%v option=%v", c.Calls(), h.m.home.opts.active)
	}
}

func TestExtendAndShorten(t *testing.T) {
	st := &starter{}
	h := newHarness(t, testOptions(st), 64)
	h.press("down", "enter")
	h.typeText("2h")
	h.press("enter")
	c := st.last(t)
	h.press("+")
	h.settle()
	h.press("-")
	h.settle()
	calls := strings.Join(c.Calls(), ",")
	if calls != "extend 15m0s,extend -15m0s" {
		t.Fatalf("calls %s", calls)
	}
	if !strings.Contains(h.m.View(), "now until") {
		t.Fatalf("no feedback:\n%s", h.m.View())
	}

	h.press("s")
	h.settle()
	h.press("up", "enter") // until stopped: + only explains
	c = st.last(t)
	h.press("+")
	h.settle()
	if len(c.Calls()) != 0 {
		t.Fatalf("calls %v on an indefinite session", c.Calls())
	}
	if !strings.Contains(h.m.View(), "+/- only change a session with an end") {
		t.Fatalf("no message:\n%s", h.m.View())
	}
}

func TestStopKeepsOptions(t *testing.T) {
	st := &starter{}
	h := newHarness(t, testOptions(st), 64)
	h.press("a", "d", "k", "b")
	h.typeText("20")
	h.press("enter")
	want := options{active: true, display: false, keys: true, battery: 20}
	if h.m.home.opts != want {
		t.Fatalf("options %+v", h.m.home.opts)
	}
	h.press("enter")
	cfg := st.last(t).cfg
	if !cfg.Active || cfg.KeepDisplay || !cfg.Activity.Keys || cfg.BatteryThreshold != 20 {
		t.Fatalf("session config %+v", cfg)
	}
	h.press("s")
	h.settle()
	if h.m.screen != screenHome || h.m.home.opts != want {
		t.Fatalf("after stop: screen %v options %+v", h.m.screen, h.m.home.opts)
	}
	if v := h.m.View(); !strings.Contains(v, "◉ Stop at battery 20%") {
		t.Fatalf("battery option lost:\n%s", v)
	}
}

func TestSessionEndsOnItsOwn(t *testing.T) {
	t.Run("returns to Home", func(t *testing.T) {
		st := &starter{}
		h := newHarness(t, testOptions(st), 64)
		h.press("down", "enter")
		h.typeText("30")
		h.press("enter")
		c := st.last(t)
		c.end(session.ReasonDuration, "duration reached")
		h.settle()
		if v := h.m.View(); !strings.Contains(v, "Stopped: duration reached at 15:48") {
			t.Fatalf("no final line:\n%s", v)
		}
		h.update(returnMsg{c: c})
		if h.quit || h.m.screen != screenHome {
			t.Fatalf("quit=%v screen=%v", h.quit, h.m.screen)
		}
	})
	t.Run("autostart quits", func(t *testing.T) {
		st := &starter{}
		o := testOptions(st)
		o.Base.Duration, o.Start = 30*time.Minute, true
		h := newHarness(t, o, 64)
		if h.m.screen != screenDashboard {
			t.Fatalf("screen %v", h.m.screen)
		}
		c := st.last(t)
		c.end(session.ReasonDuration, "duration reached")
		h.settle()
		h.update(returnMsg{c: c})
		if !h.quit {
			t.Fatal("did not quit")
		}
		if got := h.m.FinalMessage(); got != "stopped: duration reached at 15:48" {
			t.Fatalf("final message %q", got)
		}
	})
	t.Run("keepalive stop quits", func(t *testing.T) {
		st := &starter{}
		h := newHarness(t, testOptions(st), 64)
		h.press("enter")
		st.last(t).end(session.ReasonIPC, `stopped by "keepalive stop"`)
		h.settle()
		if !h.quit {
			t.Fatal("did not quit")
		}
	})
	t.Run("an error shows on Home", func(t *testing.T) {
		st := &starter{}
		h := newHarness(t, testOptions(st), 64)
		h.press("enter")
		st.last(t).end(session.ReasonError, "keep the system awake: no inhibitor")
		h.settle()
		if h.m.screen != screenHome || h.m.home.problem == nil {
			t.Fatalf("screen %v problem %v", h.m.screen, h.m.home.problem)
		}
		if v := h.m.View(); !strings.Contains(flat(v), "✗ Could not keep awake: keep the system awake: no inhibitor") {
			t.Fatalf("no error on Home:\n%s", v)
		}
	})
}

func timedSnap() session.Snapshot {
	s := runningSnap(session.Config{Active: true, KeepDisplay: true, Duration: 2 * time.Hour})
	s.Activity = activity.Status{State: activity.StateSimulating, Method: "CoreGraphics", LastBurst: testNow.Add(-12 * time.Second)}
	return s
}

func attachedHarness(t *testing.T, snap session.Snapshot, width ...int) (*harness, *fakeCtrl) {
	t.Helper()
	c := newFakeCtrl(snap, &Instance{PID: 812, Origin: "service", Version: "2.0.0"})
	st := &starter{}
	o := testOptions(st)
	o.Attach = c
	o.Claim = func() (Controller, error) { return nil, nil }
	w := 64
	if len(width) > 0 {
		w = width[0]
	}
	h := newHarness(t, o, w)
	h.update(snapMsg{c: c, snap: snap})
	return h, c
}

func TestAttachQuitDetaches(t *testing.T) {
	h, c := attachedHarness(t, timedSnap())
	if v := h.m.View(); !strings.Contains(v, "attached · service pid 812") || !strings.Contains(v, "[q] detach") {
		t.Fatalf("no attach header:\n%s", v)
	}
	h.press("q")
	if !h.quit {
		t.Fatal("q did not quit")
	}
	_ = h.m.Shutdown()
	if c.has("stop") || !c.has("close") {
		t.Fatalf("calls %v: q must detach, not stop", c.Calls())
	}
}

func TestAttachStopAsksFirst(t *testing.T) {
	claims := 0
	c := newFakeCtrl(timedSnap(), &Instance{PID: 812, Origin: "service"})
	st := &starter{}
	o := testOptions(st)
	o.Attach = c
	o.Claim = func() (Controller, error) { claims++; return nil, nil }
	h := newHarness(t, o, 64)

	h.press("s")
	if v := h.m.View(); !strings.Contains(v, "Stop the running keepalive?") || !strings.Contains(v, "[y] stop it") {
		t.Fatalf("no question:\n%s", v)
	}
	h.press("n")
	h.press("esc", "esc") // esc asks too, and esc answers no
	if c.has("stop") || h.m.dash.confirm {
		t.Fatalf("stopped without a yes: %v", c.Calls())
	}
	h.press("s", "y")
	h.settle()
	if !c.has("stop") {
		t.Fatalf("calls %v", c.Calls())
	}
	c.end(session.ReasonIPC, `stopped by "keepalive stop"`)
	h.settle()
	if v := h.m.View(); !strings.Contains(v, `Stopped by "keepalive stop" at 15:48`) {
		t.Fatalf("no final line:\n%s", v)
	}
	h.update(returnMsg{c: c})
	h.settle()
	if h.m.screen != screenHome || !h.m.claimed || claims != 1 {
		t.Fatalf("screen %v claimed %v claims %d", h.m.screen, h.m.claimed, claims)
	}
	h.press("enter") // now this process runs its own session
	if h.m.screen != screenDashboard || st.count() != 1 {
		t.Fatalf("screen %v sessions %d", h.m.screen, st.count())
	}
}

func TestAttachedKeysGoToTheController(t *testing.T) {
	h, c := attachedHarness(t, timedSnap())
	h.press("a", "+", "-")
	h.settle()
	if got := strings.Join(c.Calls(), ","); got != "active false,extend 15m0s,extend -15m0s" {
		t.Fatalf("calls %s", got)
	}
	if h.m.home.opts.active {
		t.Fatal("an attached toggle changed this UI's own option")
	}
}

func TestAttachedInstanceGone(t *testing.T) {
	h, c := attachedHarness(t, timedSnap())
	close(c.events)
	h.settle()
	if v := h.m.View(); !strings.Contains(v, "Stopped: the running keepalive is gone") {
		t.Fatalf("no final line:\n%s", v)
	}
	h.press("enter")
	if h.m.screen != screenHome {
		t.Fatalf("screen %v", h.m.screen)
	}
}

func TestClaimWarningAndAttachOnStart(t *testing.T) {
	other := newFakeCtrl(timedSnap(), &Instance{PID: 812, Origin: "terminal"})
	var answer Controller
	st := &starter{}
	o := testOptions(st)
	o.Warning = Warning{Text: "Another keepalive is open (pid 812) and keeps nothing awake.", Fix: "quit it"}
	o.Claim = func() (Controller, error) {
		if answer != nil {
			return answer, nil
		}
		return nil, Warning{Text: "Another keepalive is open (pid 812) and keeps nothing awake.", Fix: "quit it"}
	}
	h := newHarness(t, o, 64)
	if v := flat(h.m.View()); !strings.Contains(v, "! Another keepalive is open (pid 812)") || !strings.Contains(v, "Fix quit it") {
		t.Fatalf("no warning:\n%s", v)
	}
	h.press("enter")
	if h.m.screen != screenHome || st.count() != 0 {
		t.Fatalf("started despite the claim: screen %v", h.m.screen)
	}
	answer = other // it started a session meanwhile
	h.press("enter")
	if h.m.screen != screenDashboard || h.m.dash.ctrl != other || st.count() != 0 {
		t.Fatalf("screen %v ctrl %v", h.m.screen, h.m.dash.ctrl)
	}
	if v := h.m.View(); !strings.Contains(v, "Not started: another keepalive is already running") {
		t.Fatalf("no explanation:\n%s", v)
	}
}

func TestAutostartWhileAttached(t *testing.T) {
	c := newFakeCtrl(timedSnap(), &Instance{PID: 812, Origin: "service"})
	st := &starter{}
	o := testOptions(st)
	o.Attach, o.Start = c, true
	o.Base.Duration = 30 * time.Minute
	h := newHarness(t, o, 64)
	if st.count() != 0 || h.m.dash.ctrl != c {
		t.Fatal("started a second session")
	}
	if v := h.m.View(); !strings.Contains(v, "Not started: another keepalive is already running") {
		t.Fatalf("no explanation:\n%s", v)
	}
}

func TestDefaultsFromConfig(t *testing.T) {
	sched, err := schedule.Parse("mon-fri 8:00-16:00")
	if err != nil {
		t.Fatal(err)
	}
	st := &starter{}
	o := testOptions(st)
	o.Base = session.Config{Active: true, KeepDisplay: false, Activity: activity.Config{Keys: true}, Duration: 2 * time.Hour,
		Until: time.Date(2026, 10, 5, 17, 0, 0, 0, time.UTC), Schedule: sched}
	h := newHarness(t, o, 64)
	hs := h.m.home
	if hs.opts != (options{active: true, keys: true}) || hs.cursor != modeSchedule {
		t.Fatalf("home %+v", hs)
	}
	v := flat(h.m.View())
	for _, want := range []string{"◉ Simulate activity", "○ Keep display on", "◉ Tap Shift too", "For a duration… 2h", "Until a time… 17:00", "❯ During work hours… Mon–Fri 08:00–16:00"} {
		if !strings.Contains(v, want) {
			t.Errorf("Home lacks %q:\n%s", want, v)
		}
	}
	h.press("enter", "enter") // the configured work hours, as they are
	cfg := st.last(t).cfg
	if cfg.Schedule == nil || cfg.Schedule.String() != "Mon-Fri 08:00-16:00" || cfg.Duration != 0 || !cfg.Until.IsZero() {
		t.Fatalf("session config %+v", cfg)
	}
}

func TestInputErrorsMatchTheCLI(t *testing.T) {
	_, durErr := config.ParseDuration("9x")
	for _, tc := range []struct {
		name, text, want, fix string
		open                  []string
		battery               platform.BatteryStatus
	}{
		{"duration", "9x", durErr.Error(), "", []string{"down", "enter"}, platform.BatteryStatus{}},
		{"short duration", "30s", "duration must be at least 1m (got 30s)", "", []string{"down", "enter"}, platform.BatteryStatus{}},
		{"until", "25:00", `invalid time "25:00": use 24-hour HH:MM (22:00) or 12-hour HH:MM AM/PM (10:00PM)`, "", []string{"down", "down", "enter"}, platform.BatteryStatus{}},
		{"work hours", "Mnday 08:00-16:00", `unknown day "Mnday" (use Mon, Tue, … or weekdays/weekends/daily)`, scheduleHint, []string{"up", "down", "down", "down", "enter"}, platform.BatteryStatus{}},
		{"battery level", "80", "battery threshold must be below the current level (current 76%, threshold 80%)", "choose a lower value", []string{"b"}, platform.BatteryStatus{Percentage: 76, Available: true}},
		{"battery range", "0", "battery threshold must be between 1 and 100 (got 0)", "", []string{"b"}, platform.BatteryStatus{Percentage: 76, Available: true}},
		{"no battery", "20", "battery threshold 20% set, but no battery was found (not available)", `"Stop at battery" only works on machines with a battery`, []string{"b"}, platform.BatteryStatus{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &starter{}
			o := testOptions(st)
			o.Battery = func() (platform.BatteryStatus, error) { return tc.battery, nil }
			h := newHarness(t, o, 64)
			h.m.home.battery = batteryMsg{} // not read yet, so b opens the input
			h.press(tc.open...)
			h.typeText(tc.text)
			h.press("enter")
			w := h.m.input.errs[h.m.input.kind]
			if h.m.screen != screenInput || w == nil {
				t.Fatalf("no error: screen %v", h.m.screen)
			}
			if w.Text != tc.want || w.Fix != tc.fix {
				t.Fatalf("error %q / %q, want %q / %q", w.Text, w.Fix, tc.want, tc.fix)
			}
			if st.count() != 0 {
				t.Fatal("started anyway")
			}
		})
	}
}

func TestBatteryOnADesktop(t *testing.T) {
	st := &starter{}
	o := testOptions(st)
	o.Battery = func() (platform.BatteryStatus, error) { return platform.BatteryStatus{}, errNope }
	h := newHarness(t, o, 64)
	h.press("b")
	if h.m.screen != screenHome || h.m.home.opts.battery != 0 {
		t.Fatalf("screen %v battery %d", h.m.screen, h.m.home.opts.battery)
	}
	if v := flat(h.m.View()); !strings.Contains(v, "! No battery found") || !strings.Contains(v, "no battery") {
		t.Fatalf("no warning:\n%s", v)
	}
}

func TestWorkHoursPreview(t *testing.T) {
	st := &starter{}
	h := newHarness(t, testOptions(st), 64)
	h.press("down", "down", "down", "enter")
	h.typeText("weekdays 9:00-17:00")
	if v := h.m.View(); !strings.Contains(v, "✓ Mon–Fri 09:00–17:00") || !strings.Contains(v, "now: until 17:00") {
		t.Fatalf("no preview:\n%s", v)
	}
	h.press("backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	h.typeText("6:00-07:00")
	if v := h.m.View(); !strings.Contains(v, "next: Tue 06:00–07:00") {
		t.Fatalf("no next window:\n%s", v)
	}
}

func TestEnterAloneUsesTheLastValue(t *testing.T) {
	st := &starter{}
	o := testOptions(st)
	o.Base.Duration = 90 * time.Minute
	h := newHarness(t, o, 64)
	h.press("down", "enter")
	if v := h.m.View(); !strings.Contains(v, "enter alone uses 1h30m") {
		t.Fatalf("no hint:\n%s", v)
	}
	h.press("enter")
	if got := st.last(t).cfg.Duration; got != 90*time.Minute {
		t.Fatalf("duration %v", got)
	}
}

func TestTicksOnlyOnTheDashboard(t *testing.T) {
	st := &starter{}
	h := newHarness(t, testOptions(st), 64)
	h.press("enter")
	gen := h.m.tick
	m2, cmd := h.m.Update(tickMsg{gen: gen})
	if cmd == nil {
		t.Fatal("no next tick on the dashboard")
	}
	h.m = m2.(Model)
	h.press("s")
	h.settle()
	if _, cmd := h.m.Update(tickMsg{gen: gen}); cmd != nil {
		t.Fatal("ticks continue on Home")
	}
}

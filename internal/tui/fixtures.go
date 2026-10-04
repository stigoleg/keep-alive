package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/schedule"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// Fixtures are every screen state with fake data, for the golden tests and
// cmd/tui-preview: both draw them with the UI's own code. Nothing here runs
// in keepalive itself.

// Fixture is one screen state.
type Fixture struct {
	Name string
	// Build returns the UI in this state in a terminal of width × height,
	// drawn by r (nil: plain text) with look.
	Build func(width, height int, r *lipgloss.Renderer, look Look) Model
}

// fixtureNow is a Monday afternoon.
var fixtureNow = time.Date(2026, 10, 5, 15, 48, 0, 0, time.UTC)

// fixtureSnap is what a session started from cfg reports.
func fixtureSnap(cfg session.Config) session.Snapshot {
	s := session.Snapshot{
		Running:     true,
		StartedAt:   fixtureNow.Add(-48 * time.Minute),
		Active:      cfg.Active,
		KeepDisplay: cfg.KeepDisplay,
		PowerHold:   "IOPMAssertion(PreventUserIdleSystemSleep, +2)",
		InWindow:    true,
		Mode:        session.ModeIndefinite,
		Battery:     session.Battery{Threshold: cfg.BatteryThreshold},
		Activity:    activity.Status{State: activity.StateOff},
	}
	if cfg.Active {
		s.Activity = activity.Status{State: activity.StateWaitingIdle, Idle: 70 * time.Second}
	}
	switch {
	case cfg.Duration > 0:
		s.Mode, s.EndsAt = session.ModeDuration, s.StartedAt.Add(cfg.Duration)
	case !cfg.Until.IsZero():
		s.Mode, s.EndsAt = session.ModeUntil, cfg.Until
	}
	if cfg.Schedule != nil {
		s.Schedule = cfg.Schedule.String()
	}
	return s
}

// fixtureCtrl is a session that shows one snapshot and does nothing.
type fixtureCtrl struct {
	snap session.Snapshot
	inst *Instance
}

func (f *fixtureCtrl) Snapshot() (session.Snapshot, error) { return f.snap, nil }
func (f *fixtureCtrl) Events() <-chan session.Event        { return nil }
func (f *fixtureCtrl) SetActive(bool) error                { return nil }
func (f *fixtureCtrl) Extend(time.Duration) error          { return nil }
func (f *fixtureCtrl) Stop() error                         { return nil }
func (f *fixtureCtrl) Close() error                        { return nil }
func (f *fixtureCtrl) Attached() *Instance                 { return f.inst }

// driver feeds a Model messages and keys the way Bubble Tea would, without
// running the commands it returns: a fixture delivers their results itself.
type driver struct{ m Model }

func (d *driver) send(msgs ...tea.Msg) {
	for _, msg := range msgs {
		m, _ := d.m.Update(msg)
		d.m = m.(Model)
	}
}

func (d *driver) press(keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		d.send(msg)
	}
}

func (d *driver) typeText(s string) {
	for _, r := range s {
		d.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// fixtureOptions are the options every fixture starts from: Home with the
// display option on, a battery at 76 % and every session a fixtureCtrl.
func fixtureOptions(r *lipgloss.Renderer, look Look, snap func(session.Config) session.Snapshot) Options {
	if snap == nil {
		snap = fixtureSnap
	}
	return Options{
		Version:         "2.0.0",
		Base:            session.Config{KeepDisplay: true},
		Renderer:        r,
		Look:            look,
		Now:             func() time.Time { return fixtureNow },
		Battery:         func() (platform.BatteryStatus, error) { return fixtureBattery, nil },
		ActivityProblem: func() (string, string) { return "", "" },
		LogPath:         "/home/u/.cache/keepalive/keepalive.log",
		Claim:           func() (Controller, error) { return nil, nil },
		Claimed:         true,
		start:           func(cfg session.Config) Controller { return &fixtureCtrl{snap: snap(cfg)} },
	}
}

var fixtureBattery = platform.BatteryStatus{Percentage: 76, Available: true}

// fixture starts a driver on o in a width × height terminal, with the
// battery and activity checks answered.
func fixture(o Options, width, height int) *driver {
	d := &driver{m: New(o)}
	reason, hint := o.ActivityProblem()
	st, err := o.Battery()
	d.send(tea.WindowSizeMsg{Width: width, Height: height}, diagMsg{reason, hint}, batteryMsg{status: st, err: err, read: true})
	return d
}

func fixtureWorkHours() *schedule.Schedule {
	s, err := schedule.Parse("Mon-Fri 08:00-16:00")
	if err != nil {
		panic(err)
	}
	return s
}

// fixtureFull is a timed session with every row: simulating, work hours,
// battery and a watched process.
func fixtureFull() session.Snapshot {
	s := fixtureSnap(session.Config{Active: true, KeepDisplay: true, BatteryThreshold: 20})
	s.StartedAt, s.EndsAt, s.Mode = fixtureNow.Add(-108*time.Minute), fixtureNow.Add(72*time.Minute), session.ModeUntil
	s.Activity = activity.Status{State: activity.StateSimulating, Method: "CoreGraphics", LastBurst: fixtureNow.Add(-12 * time.Second)}
	s.Schedule, s.NextChange = "Mon-Fri 08:00-16:00", time.Date(2026, 10, 5, 16, 0, 0, 0, time.UTC)
	s.Battery = session.Battery{Percent: 76, Available: true, Threshold: 20}
	s.Watching = "zoom"
	return s
}

// fixtureBursts are six minutes of bursts, oldest bucket first.
var fixtureBursts = []int{0, 1, 0, 2, 0, 0, 3, 0, 1, 0, 0, 2}

// withBursts fills the dashboard's burst history with fixtureBursts.
func (d *driver) withBursts() {
	b := newBurstLog(time.Time{})
	start := fixtureNow.Truncate(sparkBucket).Add(-(sparkBuckets - 1) * sparkBucket)
	for i, n := range fixtureBursts {
		for j := range n {
			b.times = append(b.times, start.Add(time.Duration(i)*sparkBucket+time.Duration(j+1)*time.Second))
		}
	}
	b.last = d.m.dash.snap.Activity.LastBurst
	d.m.dash.bursts = b
}

// dashFixture is the dashboard of a session showing snap.
func dashFixture(name string, snap func() session.Snapshot) Fixture {
	return Fixture{Name: name, Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
		o := fixtureOptions(r, look, nil)
		o.Base, o.Start = session.Config{Active: true, KeepDisplay: true}, true
		d := fixture(o, w, h)
		s := snap()
		d.send(snapMsg{c: d.m.dash.ctrl, snap: s})
		d.withBursts()
		return d.m
	}}
}

func indefiniteFixture(a activity.Status) func() session.Snapshot {
	return func() session.Snapshot {
		s := fixtureSnap(session.Config{Active: true, KeepDisplay: true})
		s.Activity = a
		return s
	}
}

// inputFixture types text into the input that keys open, and submits it
// when submit is set.
func inputFixture(name string, keys []string, text string, submit bool) Fixture {
	return Fixture{Name: name, Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
		d := fixture(fixtureOptions(r, look, nil), w, h)
		d.press(keys...)
		d.typeText(text)
		if submit {
			d.press("enter")
		}
		return d.m
	}}
}

// homeOptions configure activity, the display and last values for every
// way to keep awake (work hours put the cursor on them; the fixtures move
// it up).
func homeOptions(r *lipgloss.Renderer, look Look) Options {
	o := fixtureOptions(r, look, nil)
	o.Base = session.Config{Active: true, KeepDisplay: true, Duration: 2 * time.Hour,
		Until: time.Date(2026, 10, 5, 17, 0, 0, 0, time.UTC), Schedule: fixtureWorkHours()}
	return o
}

// Fixtures returns every screen state, in the order a session goes
// through them.
func Fixtures() []Fixture {
	return []Fixture{
		{Name: "home", Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
			d := fixture(homeOptions(r, look), w, h)
			d.press("up", "up", "up")
			return d.m
		}},
		{Name: "home_warning", Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
			o := homeOptions(r, look)
			o.ActivityProblem = func() (string, string) {
				return `Accessibility is off for "Ghostty", so activity can't move the pointer`,
					"System Settings → Privacy & Security → Accessibility → Ghostty"
			}
			d := fixture(o, w, h)
			d.press("up", "up", "up")
			return d.m
		}},
		inputFixture("input_duration", []string{"down", "enter"}, "", false),
		inputFixture("input_duration_error", []string{"down", "enter"}, "9x", true),
		inputFixture("input_until_error", []string{"down", "down", "enter"}, "25:00", true),
		inputFixture("input_schedule_error", []string{"down", "down", "down", "enter"}, "Mnday 08:00-16:00", true),
		inputFixture("input_schedule_preview", []string{"down", "down", "down", "enter"}, "weekdays 8:00-11:30, 12:00-16:00", false),
		inputFixture("input_battery_error", []string{"b"}, "80", true),
		dashFixture("dash_simulating", fixtureFull),
		dashFixture("dash_waiting", indefiniteFixture(activity.Status{State: activity.StateWaitingIdle, Idle: 70 * time.Second})),
		dashFixture("dash_paused_user", indefiniteFixture(activity.Status{State: activity.StatePausedUser, Method: "CoreGraphics",
			LastBurst: fixtureNow.Add(-3 * time.Minute)})),
		dashFixture("dash_paused_locked", indefiniteFixture(activity.Status{State: activity.StatePausedLocked, Method: "CoreGraphics"})),
		dashFixture("dash_problem", indefiniteFixture(activity.Status{State: activity.StateDegraded,
			Reason: "Accessibility permission is missing", Hint: `Grant Accessibility to "Ghostty" in System Settings → Privacy & Security → Accessibility`})),
		dashFixture("dash_off", func() session.Snapshot {
			s := indefiniteFixture(activity.Status{State: activity.StateOff})()
			s.Active, s.KeepDisplay = false, false
			return s
		}),
		dashFixture("dash_outside_work_hours", func() session.Snapshot {
			s := indefiniteFixture(activity.Status{State: activity.StateOff})()
			s.Schedule, s.InWindow, s.PowerHold, s.Paused = "Mon-Fri 08:00-16:00", false, "", session.PauseSchedule
			s.NextChange = time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC)
			return s
		}),
		dashFixture("dash_battery_paused", func() session.Snapshot {
			s := indefiniteFixture(activity.Status{State: activity.StateOff})()
			s.Battery = session.Battery{Percent: 18, Available: true, Threshold: 20, Pause: true}
			s.PowerHold, s.Paused = "", session.PauseBattery
			return s
		}),
		dashFixture("dash_power_lost", func() session.Snapshot {
			s := indefiniteFixture(activity.Status{State: activity.StateWaitingIdle, Idle: 5 * time.Second})()
			s.PowerHold = ""
			return s
		}),
		{Name: "dash_attached", Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
			snap := fixtureSnap(session.Config{Active: true, KeepDisplay: true, Duration: 2 * time.Hour})
			snap.Activity = activity.Status{State: activity.StateSimulating, Method: "CoreGraphics", LastBurst: fixtureNow.Add(-12 * time.Second)}
			o := fixtureOptions(r, look, nil)
			c := &fixtureCtrl{snap: snap, inst: &Instance{PID: 812, Origin: "service", Version: "2.0.0"}}
			o.Attach = c
			d := fixture(o, w, h)
			d.send(snapMsg{c: c, snap: snap})
			d.withBursts()
			d.press("s")
			return d.m
		}},
		{Name: "dash_ended", Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
			d := fixture(fixtureOptions(r, look, func(session.Config) session.Snapshot { return fixtureFull() }), w, h)
			d.press("down", "enter")
			d.typeText("2h")
			d.press("enter")
			c := d.m.dash.ctrl
			d.send(snapMsg{c: c, snap: fixtureFull()})
			end := fixtureFull()
			end.Running = false
			d.send(eventMsg{c: c, ok: true, ev: session.Event{Time: fixtureNow, Type: session.EventStopped,
				Snapshot: end, Reason: session.ReasonUntil, Message: "end time reached"}})
			return d.m
		}},
		{Name: "help_home", Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
			d := fixture(fixtureOptions(r, look, nil), w, h)
			d.press("?")
			return d.m
		}},
		{Name: "help_dashboard", Build: func(w, h int, r *lipgloss.Renderer, look Look) Model {
			o := fixtureOptions(r, look, nil)
			o.LogEnabled = true
			d := fixture(o, w, h)
			d.press("enter", "?")
			return d.m
		}},
	}
}

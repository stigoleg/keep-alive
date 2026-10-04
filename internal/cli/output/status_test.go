package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

func TestStatusGolden(t *testing.T) {
	now := start.Add(48 * time.Minute)

	timed := runningSnap()
	timed.Remaining = time.Hour + 12*time.Minute
	timed.Activity = activity.Status{State: activity.StateSimulating, Method: "CoreGraphics"}

	paused := runningSnap()
	paused.Mode, paused.EndsAt, paused.Remaining = session.ModeIndefinite, time.Time{}, 0
	paused.Battery = session.Battery{}
	paused.Schedule, paused.InWindow, paused.PowerHold, paused.Paused = "Mon-Fri 08:00-16:00", false, "", session.PauseSchedule
	paused.NextChange = time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)
	paused.Watching = "zoom"

	degraded := runningSnap()
	degraded.Mode, degraded.EndsAt, degraded.Remaining = session.ModeIndefinite, time.Time{}, 0
	degraded.Battery = session.Battery{Percent: 64, Available: true}
	degraded.Schedule, degraded.NextChange = "daily 08:00-16:00", time.Date(2026, 3, 1, 16, 0, 0, 0, time.UTC)
	degraded.Activity = activity.Status{State: activity.StateDegraded, Reason: "Accessibility permission is missing", Hint: `Grant Accessibility to "Ghostty"`}

	var buf bytes.Buffer
	for _, c := range []struct {
		origin string
		snap   session.Snapshot
	}{
		{"terminal", timed},
		{"service", paused},
		{"run", degraded},
		{"terminal", session.Snapshot{}},
	} {
		buf.WriteString(StatusText(StatusInfo{PID: 4242, Version: "2.0.0", Origin: c.origin, Snapshot: NewJSONSnapshot(c.snap)}, now))
		buf.WriteString("--\n")
	}
	golden(t, "status.golden", buf.Bytes())
}

func TestFollowPrinter(t *testing.T) {
	f := fixtures()
	var buf bytes.Buffer
	p := NewFollow(&buf, false)
	repeat := f["burst"]
	repeat.Time = repeat.Time.Add(30 * time.Second)
	for _, name := range []string{"started", "battery", "battery", "activity", "snapshot", "burst", "stopping", "stopped"} {
		if err := p.Print(NewJSONEvent(f[name])); err != nil {
			t.Fatal(err)
		}
		if name == "burst" {
			p.Print(NewJSONEvent(repeat))
		}
	}
	var human bytes.Buffer
	h := NewHuman(&human, false)
	for _, name := range []string{"started", "battery", "battery", "activity", "snapshot", "burst", "stopping", "stopped"} {
		h.Print(f[name])
	}
	if buf.String() != human.String() {
		t.Fatalf("follow output differs from the human printer:\n%s\nvs\n%s", buf.String(), human.String())
	}
	if strings.Count(buf.String(), "simulating input") != 1 {
		t.Fatalf("repeated burst printed:\n%s", buf.String())
	}
}

// batteryPaused is a login service paused below its battery threshold.
func batteryPaused() session.Snapshot {
	snap := runningSnap()
	snap.Mode, snap.EndsAt, snap.Remaining = session.ModeIndefinite, time.Time{}, 0
	snap.Battery = session.Battery{Percent: 18, Available: true, Threshold: 20, Pause: true}
	snap.PowerHold, snap.Paused = "", session.PauseBattery
	snap.Activity = activity.Status{State: activity.StateOff}
	return snap
}

func TestStatusBatteryPause(t *testing.T) {
	out := StatusText(StatusInfo{PID: 4242, Version: "2.0.0", Origin: "service", Snapshot: NewJSONSnapshot(batteryPaused())}, start.Add(time.Hour))
	for _, want := range []string{
		"  activity    on; resumes when the battery recovers\n",
		"  power       released while the battery is low\n",
		"  battery     18% · pauses at 20%, resumes at 25% or when charging\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status lacks %q:\n%s", want, out)
		}
	}
}

// TestStatusBatteryPauseAtTheTop: -b 100 resumes only on external power.
func TestStatusBatteryPauseAtTheTop(t *testing.T) {
	snap := batteryPaused()
	snap.Battery.Percent, snap.Battery.Threshold = 97, 100
	out := StatusText(StatusInfo{PID: 4242, Version: "2.0.0", Origin: "service", Snapshot: NewJSONSnapshot(snap)}, start.Add(time.Hour))
	if want := "  battery     97% · pauses at 100%, resumes when charging\n"; !strings.Contains(out, want) {
		t.Errorf("status lacks %q:\n%s", want, out)
	}
}

// TestBatteryReadingWhenItPauses: a reading in a session that pauses at the
// threshold does not say it stops there.
func TestBatteryReadingWhenItPauses(t *testing.T) {
	snap := runningSnap()
	snap.Battery = session.Battery{Percent: 70, Available: true, Threshold: 100, Pause: true}
	ev := session.Event{Time: start, Type: session.EventBattery, Snapshot: snap}
	if got, want := Text(ev), "battery 70% (pauses at 100%)"; got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if got := NewJSONEvent(ev); got.Message != "battery 70% (pauses at 100%)" || !got.Snapshot.Battery.Pause || got.Snapshot.Battery.Threshold != 100 {
		t.Errorf("JSON event = %+v", got)
	}
	snap.Battery.Pause = false
	if got, want := Text(session.Event{Type: session.EventBattery, Snapshot: snap}), "battery 70% (stops at 100%)"; got != want {
		t.Errorf("foreground: Text = %q, want %q", got, want)
	}
}

func TestBatteryPauseEvents(t *testing.T) {
	running := runningSnap()
	running.Battery = session.Battery{Percent: 30, Available: true, Threshold: 20, Pause: true}
	paused := batteryPaused()
	resumed := running
	resumed.Battery.Percent = 25
	events := []session.Event{
		{Time: start, Type: session.EventStarted, Snapshot: running},
		{Time: start.Add(time.Minute), Type: session.EventBattery, Snapshot: paused, Message: "battery at 18%: paused until it is back at 25% or charging"},
		{Time: start.Add(2 * time.Minute), Type: session.EventBattery, Snapshot: paused},
		{Time: start.Add(3 * time.Minute), Type: session.EventBattery, Snapshot: resumed, Message: "battery at 25%: keeping awake again"},
	}
	var human, follow bytes.Buffer
	h, f := NewHuman(&human, false), NewFollow(&follow, false)
	for _, ev := range events {
		h.Print(ev)
		f.Print(NewJSONEvent(ev))
	}
	want := "10:02:11 keeping system and display awake for 2h0m (until 12:02), pausing at 20% battery, simulating activity\n" +
		"10:03:11 battery at 18%: paused until it is back at 25% or charging\n" +
		"10:05:11 battery at 25%: keeping awake again\n"
	if human.String() != want {
		t.Errorf("human:\n%s\nwant:\n%s", human.String(), want)
	}
	if follow.String() != want {
		t.Errorf("follow:\n%s\nwant:\n%s", follow.String(), want)
	}

	js := NewJSONSnapshot(paused)
	if js.Paused != "battery" || !js.Battery.Pause {
		t.Fatalf("JSON snapshot: %+v", js)
	}
	if back := SnapshotFromJSON(js); back.Paused != session.PauseBattery || !back.Battery.Pause {
		t.Fatalf("round trip: %+v", back)
	}
}

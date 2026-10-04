package output

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMain(m *testing.M) {
	time.Local = time.UTC // golden files use UTC wall clock
	os.Exit(m.Run())
}

var (
	start = time.Date(2026, 3, 1, 10, 2, 11, 0, time.UTC)
	ends  = start.Add(2 * time.Hour)
)

func runningSnap() session.Snapshot {
	return session.Snapshot{
		Running:     true,
		StartedAt:   start,
		EndsAt:      ends,
		Remaining:   2 * time.Hour,
		Mode:        session.ModeDuration,
		Active:      true,
		Activity:    activity.Status{State: activity.StateOff},
		Battery:     session.Battery{Percent: 80, Available: true, Threshold: 20},
		KeepDisplay: true,
		PowerHold:   "caffeinate -dims",
		InWindow:    true,
	}
}

// fixtures returns one event of every type, in session order.
func fixtures() map[string]session.Event {
	snap := runningSnap()

	waiting := snap
	waiting.Remaining = 2*time.Hour - 2*time.Minute
	waiting.Activity = activity.Status{State: activity.StateWaitingIdle, Method: "CoreGraphics mouse events", Reason: "needs 2m0s", Idle: 5 * time.Second}

	heartbeat := waiting
	heartbeat.Remaining = 2*time.Hour - 150*time.Second

	burst := waiting
	burst.Activity = activity.Status{State: activity.StateSimulating, Method: "CoreGraphics mouse events", LastBurst: start.Add(5 * time.Minute)}

	paused := snap
	paused.Schedule, paused.InWindow, paused.PowerHold, paused.Paused = "Mon-Fri 08:00-16:00", false, "", session.PauseSchedule
	paused.NextChange = time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)

	stopped := snap
	stopped.Running = false
	stopped.Remaining = 0
	stopped.Activity = activity.Status{State: activity.StateOff}

	return map[string]session.Event{
		"started":  {Time: start, Type: session.EventStarted, Snapshot: snap},
		"activity": {Time: start.Add(2 * time.Minute), Type: session.EventActivity, Snapshot: waiting},
		"burst":    {Time: start.Add(5 * time.Minute), Type: session.EventActivity, Snapshot: burst},
		"battery":  {Time: start.Add(30 * time.Second), Type: session.EventBattery, Snapshot: snap},
		"snapshot": {Time: start.Add(150 * time.Second), Type: session.EventSnapshot, Snapshot: heartbeat},
		"warning":  {Time: start.Add(time.Minute), Type: session.EventWarning, Snapshot: snap, Message: "battery status unavailable: no battery"},
		"schedule": {Time: start.Add(3 * time.Minute), Type: session.EventSchedule, Snapshot: paused, Message: "outside work hours until Mon 08:00"},
		"stopping": {Time: ends, Type: session.EventStopping, Snapshot: snap, Reason: session.ReasonDuration},
		"stopped":  {Time: ends, Type: session.EventStopped, Snapshot: stopped, Reason: session.ReasonDuration, Message: "duration reached"},
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
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
		t.Errorf("%s mismatch\n got: %s\nwant: %s", name, got, want)
	}
}

func TestJSONGolden(t *testing.T) {
	for name, ev := range fixtures() {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := NewJSON(&buf).Print(ev); err != nil {
				t.Fatal(err)
			}
			if !json.Valid(buf.Bytes()) || strings.Count(buf.String(), "\n") != 1 {
				t.Fatalf("not a single NDJSON line: %q", buf.String())
			}
			golden(t, "json_"+name+".golden", buf.Bytes())
		})
	}
}

func TestJSONIndefiniteHasNullEnd(t *testing.T) {
	snap := runningSnap()
	snap.Mode, snap.EndsAt, snap.Remaining = session.ModeIndefinite, time.Time{}, 0
	var buf bytes.Buffer
	if err := NewJSON(&buf).Print(session.Event{Time: start, Type: session.EventStarted, Snapshot: snap}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Snapshot map[string]any `json:"snapshot"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ends_at", "remaining"} {
		if v, ok := got.Snapshot[k]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, ok)
		}
	}
}

func TestHumanGolden(t *testing.T) {
	f := fixtures()
	var buf bytes.Buffer
	p := NewHuman(&buf, false)
	for _, name := range []string{"started", "battery", "warning", "activity", "schedule", "snapshot", "burst", "stopping", "stopped"} {
		if err := p.Print(f[name]); err != nil {
			t.Fatal(err)
		}
	}
	golden(t, "human.golden", buf.Bytes())
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("ANSI escapes without color")
	}
}

func TestHumanSkipsRepeatedBursts(t *testing.T) {
	f := fixtures()
	next := f["burst"]
	next.Time = next.Time.Add(30 * time.Second)
	next.Snapshot.Activity.LastBurst = next.Time
	var buf bytes.Buffer
	p := NewHuman(&buf, false)
	for _, ev := range []session.Event{f["started"], f["burst"], next} {
		p.Print(ev)
	}
	if n := strings.Count(buf.String(), "simulating input"); n != 1 {
		t.Fatalf("burst printed %d times:\n%s", n, buf.String())
	}
}

func TestHumanColor(t *testing.T) {
	var buf bytes.Buffer
	NewHuman(&buf, true).Print(fixtures()["warning"])
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("no ANSI escapes with color on")
	}
}

func TestHumanStartedVariants(t *testing.T) {
	snap := runningSnap()
	snap.Active = false
	snap.Battery = session.Battery{}

	indefinite := snap
	indefinite.Mode, indefinite.EndsAt, indefinite.Remaining = session.ModeIndefinite, time.Time{}, 0

	until := snap
	until.Mode, until.Remaining = session.ModeUntil, 11*time.Hour+58*time.Minute
	until.EndsAt = time.Date(2026, 3, 1, 22, 0, 0, 0, time.UTC)

	systemOnly := indefinite
	systemOnly.KeepDisplay = false

	paused := indefinite
	paused.Schedule, paused.InWindow, paused.PowerHold = "Mon-Fri 08:00-16:00", false, ""
	paused.NextChange = time.Date(2026, 3, 2, 8, 0, 0, 0, time.UTC)

	working := snap
	working.Schedule, working.NextChange = "daily 09:00-17:00", time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC)

	watching := indefinite
	watching.Watching = "zoom"

	watchingTimed := snap
	watchingTimed.Watching = "pid 42"

	tests := map[string]struct {
		snap session.Snapshot
		want string
	}{
		"duration":         {snap, "keeping system and display awake for 2h0m (until 12:02)"},
		"indefinite":       {indefinite, "keeping system and display awake indefinitely"},
		"until":            {until, "keeping system and display awake until 22:00 (11h58m)"},
		"system only":      {systemOnly, "keeping system awake indefinitely"},
		"outside schedule": {paused, "keeping system and display awake during work hours (Mon-Fri 08:00-16:00); outside work hours until Mon 08:00"},
		"inside schedule":  {working, "keeping system and display awake for 2h0m (until 12:02), during work hours (daily 09:00-17:00)"},
		"watching":         {watching, "keeping system and display awake while zoom runs"},
		"watching, timed":  {watchingTimed, "keeping system and display awake for 2h0m (until 12:02), while pid 42 runs"},
	}
	for name, tt := range tests {
		got := Text(session.Event{Time: start, Type: session.EventStarted, Snapshot: tt.snap})
		if got != tt.want {
			t.Errorf("%s: %q, want %q", name, got, tt.want)
		}
	}
}

func TestJSONScheduleFields(t *testing.T) {
	var buf bytes.Buffer
	if err := NewJSON(&buf).Print(fixtures()["schedule"]); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Type     string `json:"type"`
		Snapshot struct {
			Schedule   string  `json:"schedule"`
			InWindow   bool    `json:"in_window"`
			NextChange *string `json:"next_change"`
			Watching   string  `json:"watching"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "schedule" || got.Snapshot.Schedule != "Mon-Fri 08:00-16:00" || got.Snapshot.InWindow ||
		got.Snapshot.NextChange == nil || *got.Snapshot.NextChange != "2026-03-02T08:00:00Z" {
		t.Fatalf("decoded = %+v", got)
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		2 * time.Hour:                        "2h0m",
		45 * time.Minute:                     "45m",
		90 * time.Second:                     "1m30s",
		30 * time.Second:                     "30s",
		0:                                    "0s",
		2*time.Minute + 400*time.Millisecond: "2m",
	} {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestRunPrinterIsQuiet(t *testing.T) {
	f := fixtures()
	degraded := f["activity"]
	degraded.Snapshot.Activity = activity.Status{State: activity.StateDegraded, Reason: "no input method works"}
	commandDone := f["stopped"]
	commandDone.Reason, commandDone.Message = session.ReasonCommandExited, "command exited"
	started := f["started"]
	started.Snapshot.Watching = "make"
	started.Snapshot.Mode, started.Snapshot.EndsAt, started.Snapshot.Battery.Threshold = session.ModeIndefinite, time.Time{}, 0

	var buf bytes.Buffer
	p := NewRun(&buf, false)
	for _, ev := range []session.Event{started, f["battery"], f["activity"], f["warning"], degraded, degraded, f["snapshot"], f["burst"], f["stopping"], commandDone} {
		if err := p.Print(ev); err != nil {
			t.Fatal(err)
		}
	}
	want := "keepalive: keeping system and display awake while make runs, simulating activity\n" +
		"keepalive: warning: battery status unavailable: no battery\n" +
		"keepalive: active: unavailable (no input method works)\n"
	if got := buf.String(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	buf.Reset()
	p = NewRun(&buf, false)
	p.Print(f["started"])
	p.Print(f["stopped"])
	if got := buf.String(); !strings.HasSuffix(got, "\nkeepalive: stopped: duration reached\n") {
		t.Fatalf("unusual stop = %q", got)
	}

	// A session that could not start is the command's error, not a line.
	failed := f["stopped"]
	failed.Reason, failed.Message = session.ReasonError, "keep the system awake: denied"
	buf.Reset()
	NewRun(&buf, false).Print(failed)
	if buf.Len() != 0 {
		t.Fatalf("start failure printed %q", buf.String())
	}
}

func TestJSONRoundTrip(t *testing.T) {
	for name, ev := range fixtures() {
		ev.Snapshot.Watching = "zoom"
		ev.Snapshot.Activity.Idle = ev.Snapshot.Activity.Idle.Truncate(time.Second)
		got := EventFromJSON(NewJSONEvent(ev))
		want := ev
		if !want.Snapshot.EndsAt.IsZero() {
			want.Snapshot.Remaining = want.Snapshot.Remaining.Truncate(time.Second)
		} else {
			want.Snapshot.Remaining = 0
		}
		switch ev.Type {
		case session.EventStarted, session.EventSnapshot, session.EventActivity, session.EventBattery, session.EventStopping:
			want.Message = "" // these messages are rendered, not carried
		}
		if !got.Time.Equal(want.Time) {
			t.Errorf("%s: time %v, want %v", name, got.Time, want.Time)
		}
		got.Time, want.Time = time.Time{}, time.Time{}
		if !equalSnapshots(got.Snapshot, want.Snapshot) {
			t.Errorf("%s: snapshot\n got %+v\nwant %+v", name, got.Snapshot, want.Snapshot)
		}
		got.Snapshot, want.Snapshot = session.Snapshot{}, session.Snapshot{}
		if got != want {
			t.Errorf("%s: event %+v, want %+v", name, got, want)
		}
	}
}

func equalSnapshots(a, b session.Snapshot) bool {
	sameTime := func(x, y time.Time) bool { return x.Equal(y) }
	ok := sameTime(a.StartedAt, b.StartedAt) && sameTime(a.EndsAt, b.EndsAt) && sameTime(a.NextChange, b.NextChange) &&
		sameTime(a.Activity.LastBurst, b.Activity.LastBurst)
	a.StartedAt, a.EndsAt, a.NextChange, a.Activity.LastBurst = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	b.StartedAt, b.EndsAt, b.NextChange, b.Activity.LastBurst = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	return ok && a == b
}

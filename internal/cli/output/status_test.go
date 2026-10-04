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
	paused.Schedule, paused.InWindow, paused.PowerHold = "Mon-Fri 08:00-16:00", false, ""
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

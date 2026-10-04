package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// StatusInfo is what `keepalive status` shows about the running instance.
type StatusInfo struct {
	PID      int          `json:"pid"`
	Version  string       `json:"version"`
	Origin   string       `json:"origin"`
	Snapshot JSONSnapshot `json:"snapshot"`
}

// StatusText renders a compact summary of the running instance, e.g.
//
//	keepalive 2.0.0 · pid 4242 · started by the login service at 08:00
//	  session     1h12m left (until 17:00)
//	  work hours  Mon-Fri 08:00-16:00 · in work hours until 16:00
//	  activity    simulating input via CoreGraphics
//	  power       IOPMAssertion(PreventUserIdleSystemSleep)
//	  battery     76% · stops at 20%
func StatusText(st StatusInfo, now time.Time) string {
	snap := st.Snapshot
	var b strings.Builder
	fmt.Fprintf(&b, "keepalive %s · pid %d · started %s", st.Version, st.PID, originName(st.Origin))
	if t, ok := parseTime(snap.StartedAt); ok && snap.Running {
		fmt.Fprintf(&b, " at %s", session.ClockText(now.Local(), t.Local()))
	}
	b.WriteString("\n")
	row := func(label, value string) { fmt.Fprintf(&b, "  %-11s %s\n", label, value) }

	if !snap.Running {
		row("session", "none (the interactive UI is open; nothing is kept awake)")
		return b.String()
	}
	row("session", sessionLine(snap, now))
	if snap.Schedule != "" {
		row("work hours", scheduleLine(snap, now))
	}
	if snap.Watching != "" {
		row("watching", snap.Watching)
	}
	act := activity.Status{
		State:  activity.State(snap.Activity.State),
		Method: snap.Activity.Method,
		Reason: snap.Activity.Reason,
		Hint:   snap.Activity.Hint,
	}
	switch {
	case !snap.Active:
		row("activity", "off")
	case !snap.InWindow:
		row("activity", "on; resumes with the work hours")
	case snap.Paused == string(session.PauseBattery):
		row("activity", "on; resumes when the battery recovers")
	default:
		row("activity", ActivityText(act, false))
		if act.Hint != "" && act.State == activity.StateDegraded {
			row("", "fix: "+act.Hint)
		}
	}
	switch {
	case !snap.InWindow:
		row("power", "released while outside the work hours")
	case snap.Paused == string(session.PauseBattery):
		row("power", "released while the battery is low")
	case snap.PowerHold == "":
		row("power", "lost; re-acquiring")
	default:
		row("power", snap.PowerHold)
	}
	if bat := snap.Battery; bat.Available || bat.Threshold > 0 {
		v := "unavailable"
		if bat.Available {
			v = fmt.Sprintf("%d%%", bat.Percent)
		}
		switch {
		case bat.Threshold > 0 && bat.Pause:
			v += fmt.Sprintf(" · pauses at %d%%, resumes at %d%% or when charging", bat.Threshold, bat.Threshold+session.BatteryResumeMargin)
		case bat.Threshold > 0:
			v += fmt.Sprintf(" · stops at %d%%", bat.Threshold)
		}
		row("battery", v)
	}
	return b.String()
}

func originName(o string) string {
	switch o {
	case "service":
		return "by the login service"
	case "run":
		return "by keepalive run"
	case "terminal", "":
		return "in a terminal"
	}
	return "by " + o
}

func sessionLine(snap JSONSnapshot, now time.Time) string {
	keeps := "keeps the system awake"
	if snap.KeepDisplay {
		keeps = "keeps the system and display awake"
	}
	if end, ok := parseTime(snap.EndsAt); ok && snap.Remaining != nil {
		left := FormatDuration(time.Duration(*snap.Remaining) * time.Second)
		return fmt.Sprintf("%s left (until %s); %s", left, session.ClockText(now.Local(), end.Local()), keeps)
	}
	if start, ok := parseTime(snap.StartedAt); ok {
		return fmt.Sprintf("until stopped (running for %s); %s", FormatDuration(now.Sub(start).Truncate(time.Minute)), keeps)
	}
	return "until stopped; " + keeps
}

func scheduleLine(snap JSONSnapshot, now time.Time) string {
	next, ok := parseTime(snap.NextChange)
	switch {
	case !ok:
		return snap.Schedule
	case snap.InWindow:
		return fmt.Sprintf("%s · in work hours until %s", snap.Schedule, session.ClockText(now.Local(), next.Local()))
	default:
		return fmt.Sprintf("%s · paused until %s", snap.Schedule, session.ClockText(now.Local(), next.Local()))
	}
}

func parseTime(s *string) (time.Time, bool) {
	if s == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	return t, err == nil
}

// Follow prints events received from a running instance (`keepalive status
// --follow`) the way the human printer prints local ones.
type Follow struct {
	w     io.Writer
	color bool

	started  bool
	endsAt   string
	battery  JSONBattery
	paused   string
	activity JSONActivity
}

// NewFollow returns a human printer for streamed events.
func NewFollow(w io.Writer, color bool) *Follow { return &Follow{w: w, color: color} }

func (f *Follow) Print(ev JSONEvent) error {
	if !f.meaningful(ev) {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, ev.Time)
	if err != nil {
		t = time.Now()
	}
	ts, text := t.Local().Format("15:04:05"), ev.Message
	if f.color {
		ts = dim + ts + reset
		if ev.Type == string(session.EventWarning) {
			text = yellow + text + reset
		}
	}
	_, err = fmt.Fprintf(f.w, "%s %s\n", ts, text)
	return err
}

func (f *Follow) meaningful(ev JSONEvent) bool {
	snap := ev.Snapshot
	endsAt := ""
	if snap.EndsAt != nil {
		endsAt = *snap.EndsAt
	}
	pausedChanged := snap.Paused != f.paused
	f.paused = snap.Paused
	switch session.EventType(ev.Type) {
	case session.EventStarted:
		f.started, f.endsAt, f.battery = true, endsAt, snap.Battery
		return true
	case session.EventSnapshot:
		changed := f.started && endsAt != f.endsAt
		f.endsAt, f.started = endsAt, true
		return changed
	case session.EventBattery: // like Human
		changed := snap.Battery != f.battery || pausedChanged
		f.battery = snap.Battery
		return changed
	case session.EventActivity:
		a, b := snap.Activity, f.activity
		f.activity = a
		return a.State != b.State || a.Reason != b.Reason || a.Method != b.Method || a.Hint != b.Hint
	case session.EventStopping:
		return false
	}
	return true
}

// JSONLines writes values as NDJSON, like the --json printer.
type JSONLines struct{ enc *json.Encoder }

// NewJSONLines returns an NDJSON writer.
func NewJSONLines(w io.Writer) *JSONLines {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &JSONLines{enc: enc}
}

// Write encodes v as one line.
func (j *JSONLines) Write(v any) error { return j.enc.Encode(v) }

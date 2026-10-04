// Package output renders session events for headless mode: one human line per
// meaningful event, or NDJSON with one object per event.
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

// Printer writes session events.
type Printer interface {
	Print(ev session.Event) error
}

// Human prints one line per meaningful event, prefixed with the local time.
// Heartbeats and unchanged readings are skipped.
type Human struct {
	w     io.Writer
	color bool

	lastBattery  session.Battery
	lastEndsAt   time.Time
	lastActivity activity.Status
}

// NewHuman returns a human printer; color enables ANSI styling.
func NewHuman(w io.Writer, color bool) *Human { return &Human{w: w, color: color} }

func (h *Human) Print(ev session.Event) error {
	if !h.meaningful(ev) {
		return nil
	}
	ts := ev.Time.Local().Format("15:04:05")
	text := Text(ev)
	if h.color {
		ts = dim + ts + reset
		if ev.Type == session.EventWarning {
			text = yellow + text + reset
		}
	}
	_, err := fmt.Fprintf(h.w, "%s %s\n", ts, text)
	return err
}

func (h *Human) meaningful(ev session.Event) bool {
	snap := ev.Snapshot
	switch ev.Type {
	case session.EventStarted:
		h.lastEndsAt = snap.EndsAt
		h.lastBattery = snap.Battery
		return true
	case session.EventSnapshot:
		changed := !snap.EndsAt.Equal(h.lastEndsAt)
		h.lastEndsAt = snap.EndsAt
		return changed
	case session.EventBattery:
		changed := snap.Battery != h.lastBattery
		h.lastBattery = snap.Battery
		return changed
	case session.EventActivity:
		// Every burst updates LastBurst; print only real state changes.
		a, b := snap.Activity, h.lastActivity
		h.lastActivity = a
		return a.State != b.State || a.Reason != b.Reason || a.Method != b.Method || a.Hint != b.Hint
	case session.EventStopping:
		return false
	}
	return true
}

// Run prints the few lines `keepalive run` writes to stderr next to the
// command's own output: the start, warnings, work-hour changes, activity
// simulation failing, and a stop for any reason other than the command
// exiting. Each line starts with "keepalive: ".
type Run struct {
	w        io.Writer
	color    bool
	degraded bool
	started  bool
}

// NewRun returns the quiet printer for `keepalive run`.
func NewRun(w io.Writer, color bool) *Run { return &Run{w: w, color: color} }

func (r *Run) Print(ev session.Event) error {
	switch ev.Type {
	case session.EventStarted:
		r.started = true
	case session.EventWarning, session.EventSchedule:
	case session.EventActivity:
		was := r.degraded
		r.degraded = ev.Snapshot.Activity.State == activity.StateDegraded
		if !r.degraded || was {
			return nil
		}
	case session.EventStopped:
		// A session that never started is reported as the command's error.
		if ev.Reason == session.ReasonCommandExited || !r.started {
			return nil
		}
	default:
		return nil
	}
	text := Text(ev)
	if r.color && (ev.Type == session.EventWarning || ev.Type == session.EventActivity) {
		text = yellow + text + reset
	}
	_, err := fmt.Fprintf(r.w, "keepalive: %s\n", text)
	return err
}

const (
	dim    = "\x1b[2m"
	yellow = "\x1b[33m"
	reset  = "\x1b[0m"
)

// Text is the human description of an event, without the timestamp.
func Text(ev session.Event) string {
	snap := ev.Snapshot
	switch ev.Type {
	case session.EventStarted:
		return startedText(ev.Time, snap)
	case session.EventSnapshot:
		return remainingText(snap)
	case session.EventActivity:
		return activityText(snap)
	case session.EventBattery:
		if !snap.Battery.Available {
			if ev.Message != "" {
				return ev.Message
			}
			return "battery status unavailable"
		}
		if snap.Battery.Threshold > 0 {
			return fmt.Sprintf("battery %d%% (stops at %d%%)", snap.Battery.Percent, snap.Battery.Threshold)
		}
		return fmt.Sprintf("battery %d%%", snap.Battery.Percent)
	case session.EventWarning:
		return "warning: " + ev.Message
	case session.EventStopping:
		return fmt.Sprintf("stopping (%s)", ev.Reason)
	case session.EventStopped:
		if ev.Reason == session.ReasonError {
			return "stopped: error: " + ev.Message
		}
		return "stopped: " + ev.Message
	}
	return ev.Message
}

func startedText(now time.Time, snap session.Snapshot) string {
	var b strings.Builder
	b.WriteString("keeping system ")
	if snap.KeepDisplay {
		b.WriteString("and display ")
	}
	b.WriteString("awake")
	var limits []string
	switch snap.Mode {
	case session.ModeDuration:
		limits = append(limits, fmt.Sprintf("for %s (until %s)", FormatDuration(snap.Remaining), clockText(snap.EndsAt)))
	case session.ModeUntil:
		limits = append(limits, fmt.Sprintf("until %s (%s)", clockText(snap.EndsAt), FormatDuration(snap.Remaining)))
	}
	if snap.Schedule != "" {
		limits = append(limits, fmt.Sprintf("during work hours (%s)", snap.Schedule))
	}
	if snap.Watching != "" {
		limits = append(limits, fmt.Sprintf("while %s runs", snap.Watching))
	}
	if len(limits) == 0 {
		limits = append(limits, "indefinitely")
	}
	b.WriteString(" " + strings.Join(limits, ", "))
	if snap.Battery.Threshold > 0 {
		fmt.Fprintf(&b, ", stopping at %d%% battery", snap.Battery.Threshold)
	}
	if snap.Active {
		b.WriteString(", simulating activity")
	}
	if snap.Schedule != "" && !snap.InWindow && !snap.NextChange.IsZero() {
		fmt.Fprintf(&b, "; outside work hours until %s", session.ClockText(now.Local(), snap.NextChange.Local()))
	}
	return b.String()
}

func remainingText(snap session.Snapshot) string {
	if snap.EndsAt.IsZero() {
		return "running indefinitely"
	}
	return fmt.Sprintf("%s left (until %s)", FormatDuration(snap.Remaining), clockText(snap.EndsAt))
}

func activityText(snap session.Snapshot) string {
	return "active: " + ActivityText(snap.Activity, true)
}

// ActivityText describes an activity state, e.g. "simulating input via
// CoreGraphics" or "unavailable (reason)"; withFix appends "; hint".
func ActivityText(st activity.Status, withFix bool) string {
	var s string
	switch st.State {
	case activity.StateOff:
		return "off"
	case activity.StateWaitingIdle:
		s = "waiting for idle"
	case activity.StateSimulating:
		s = "simulating input"
		if st.Method != "" {
			s += " via " + st.Method
		}
		return withHint(s, st, withFix)
	case activity.StatePausedUser:
		return withHint("paused while you use the computer", st, withFix)
	case activity.StatePausedLocked:
		return withHint("paused while the screen is locked", st, withFix)
	case activity.StateDegraded:
		s = "unavailable"
	default:
		s = string(st.State)
	}
	if st.Reason != "" {
		s += " (" + st.Reason + ")"
	}
	return withHint(s, st, withFix)
}

func withHint(s string, st activity.Status, withFix bool) string {
	if withFix && st.Hint != "" {
		s += "; " + st.Hint
	}
	return s
}

func clockText(t time.Time) string { return t.Local().Format("15:04") }

// FormatDuration renders d compactly at second precision, dropping a zero
// seconds part from minute-or-longer values ("2h0m", "45m", "1m30s", "30s").
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	s := d.String()
	if d >= time.Minute && strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	return s
}

// JSON writes one JSON object per event (NDJSON).
type JSON struct{ enc *json.Encoder }

// NewJSON returns an NDJSON printer.
func NewJSON(w io.Writer) *JSON {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &JSON{enc: enc}
}

func (j *JSON) Print(ev session.Event) error { return j.enc.Encode(NewJSONEvent(ev)) }

// JSONEvent is the NDJSON object for one event. Field order is part of the
// output format; other packages (the control socket) reuse it.
type JSONEvent struct {
	Time     string       `json:"time"`
	Type     string       `json:"type"`
	Snapshot JSONSnapshot `json:"snapshot"`
	Message  string       `json:"message"`
	Reason   string       `json:"reason"`
}

// JSONSnapshot is the JSON form of a session.Snapshot. Times are RFC 3339
// strings or null; durations are whole seconds.
type JSONSnapshot struct {
	Running       bool         `json:"running"`
	StartedAt     *string      `json:"started_at"`
	EndsAt        *string      `json:"ends_at"`
	Remaining     *int64       `json:"remaining"`
	RemainingText string       `json:"remaining_text"`
	Mode          string       `json:"mode"`
	Active        bool         `json:"active"`
	Activity      JSONActivity `json:"activity"`
	Battery       JSONBattery  `json:"battery"`
	KeepDisplay   bool         `json:"keep_display"`
	PowerHold     string       `json:"power_hold"`
	Schedule      string       `json:"schedule"`
	InWindow      bool         `json:"in_window"`
	NextChange    *string      `json:"next_change"`
	Watching      string       `json:"watching"`
}

// JSONActivity is the activity part of a JSONSnapshot.
type JSONActivity struct {
	State     string  `json:"state"`
	Method    string  `json:"method"`
	Reason    string  `json:"reason"`
	Hint      string  `json:"hint"`
	LastBurst *string `json:"last_burst"`
	Idle      int64   `json:"idle"`
	IdleText  string  `json:"idle_text"`
}

// JSONBattery is the battery part of a JSONSnapshot.
type JSONBattery struct {
	Percent   int  `json:"percent"`
	Available bool `json:"available"`
	Threshold int  `json:"threshold"`
}

// NewJSONEvent converts ev to the object the JSON printer writes; its
// message is Text(ev).
func NewJSONEvent(ev session.Event) JSONEvent {
	return JSONEvent{
		Time:     ev.Time.Format(time.RFC3339Nano),
		Type:     string(ev.Type),
		Snapshot: NewJSONSnapshot(ev.Snapshot),
		Message:  Text(ev),
		Reason:   string(ev.Reason),
	}
}

// NewJSONSnapshot converts snap to its JSON form.
func NewJSONSnapshot(snap session.Snapshot) JSONSnapshot {
	js := JSONSnapshot{
		Running:   snap.Running,
		StartedAt: timePtr(snap.StartedAt),
		EndsAt:    timePtr(snap.EndsAt),
		Mode:      string(snap.Mode),
		Active:    snap.Active,
		Activity: JSONActivity{
			State:     string(snap.Activity.State),
			Method:    snap.Activity.Method,
			Reason:    snap.Activity.Reason,
			Hint:      snap.Activity.Hint,
			LastBurst: timePtr(snap.Activity.LastBurst),
			Idle:      seconds(snap.Activity.Idle),
			IdleText:  FormatDuration(snap.Activity.Idle),
		},
		Battery:     JSONBattery{Percent: snap.Battery.Percent, Available: snap.Battery.Available, Threshold: snap.Battery.Threshold},
		KeepDisplay: snap.KeepDisplay,
		PowerHold:   snap.PowerHold,
		Schedule:    snap.Schedule,
		InWindow:    snap.InWindow,
		NextChange:  timePtr(snap.NextChange),
		Watching:    snap.Watching,
	}
	if !snap.EndsAt.IsZero() {
		r := seconds(snap.Remaining)
		js.Remaining = &r
		js.RemainingText = FormatDuration(snap.Remaining)
	}
	return js
}

func timePtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.Format(time.RFC3339Nano)
	return &s
}

func seconds(d time.Duration) int64 { return int64(d / time.Second) }

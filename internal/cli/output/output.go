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

	lastBattery session.Battery
	lastEndsAt  time.Time
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
	case session.EventStopping:
		return false
	}
	return true
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
		return startedText(snap)
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

func startedText(snap session.Snapshot) string {
	var b strings.Builder
	b.WriteString("keeping system ")
	if snap.KeepDisplay {
		b.WriteString("and display ")
	}
	b.WriteString("awake ")
	switch snap.Mode {
	case session.ModeDuration:
		fmt.Fprintf(&b, "for %s (until %s)", FormatDuration(snap.Remaining), clockText(snap.EndsAt))
	case session.ModeUntil:
		fmt.Fprintf(&b, "until %s (%s)", clockText(snap.EndsAt), FormatDuration(snap.Remaining))
	default:
		b.WriteString("indefinitely")
	}
	if snap.Battery.Threshold > 0 {
		fmt.Fprintf(&b, ", stopping at %d%% battery", snap.Battery.Threshold)
	}
	if snap.Active {
		b.WriteString(", simulating activity")
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
	st := snap.Activity
	var s string
	switch st.State {
	case activity.StateOff:
		return "active: off"
	case activity.StateWaitingIdle:
		s = "active: waiting for idle"
	case activity.StateSimulating:
		s = "active: simulating input"
		if st.Method != "" {
			s += " via " + st.Method
		}
		return withHint(s, st)
	case activity.StatePausedUser:
		s = "active: paused while you use the computer"
		return withHint(s, st)
	case activity.StatePausedLocked:
		s = "active: paused while the screen is locked"
		return withHint(s, st)
	case activity.StateDegraded:
		s = "active: unavailable"
	default:
		s = "active: " + string(st.State)
	}
	if st.Reason != "" {
		s += " (" + st.Reason + ")"
	}
	return withHint(s, st)
}

func withHint(s string, st activity.Status) string {
	if st.Hint != "" {
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

func (j *JSON) Print(ev session.Event) error { return j.enc.Encode(newJSONEvent(ev)) }

type jsonEvent struct {
	Time     string       `json:"time"`
	Type     string       `json:"type"`
	Snapshot jsonSnapshot `json:"snapshot"`
	Message  string       `json:"message"`
	Reason   string       `json:"reason"`
}

type jsonSnapshot struct {
	Running       bool         `json:"running"`
	StartedAt     *string      `json:"started_at"`
	EndsAt        *string      `json:"ends_at"`
	Remaining     *int64       `json:"remaining"`
	RemainingText string       `json:"remaining_text"`
	Mode          string       `json:"mode"`
	Active        bool         `json:"active"`
	Activity      jsonActivity `json:"activity"`
	Battery       jsonBattery  `json:"battery"`
	KeepDisplay   bool         `json:"keep_display"`
	PowerHold     string       `json:"power_hold"`
}

type jsonActivity struct {
	State     string  `json:"state"`
	Method    string  `json:"method"`
	Reason    string  `json:"reason"`
	Hint      string  `json:"hint"`
	LastBurst *string `json:"last_burst"`
	Idle      int64   `json:"idle"`
	IdleText  string  `json:"idle_text"`
}

type jsonBattery struct {
	Percent   int  `json:"percent"`
	Available bool `json:"available"`
	Threshold int  `json:"threshold"`
}

func newJSONEvent(ev session.Event) jsonEvent {
	snap := ev.Snapshot
	js := jsonSnapshot{
		Running:   snap.Running,
		StartedAt: timePtr(snap.StartedAt),
		EndsAt:    timePtr(snap.EndsAt),
		Mode:      string(snap.Mode),
		Active:    snap.Active,
		Activity: jsonActivity{
			State:     string(snap.Activity.State),
			Method:    snap.Activity.Method,
			Reason:    snap.Activity.Reason,
			Hint:      snap.Activity.Hint,
			LastBurst: timePtr(snap.Activity.LastBurst),
			Idle:      seconds(snap.Activity.Idle),
			IdleText:  FormatDuration(snap.Activity.Idle),
		},
		Battery:     jsonBattery{Percent: snap.Battery.Percent, Available: snap.Battery.Available, Threshold: snap.Battery.Threshold},
		KeepDisplay: snap.KeepDisplay,
		PowerHold:   snap.PowerHold,
	}
	if !snap.EndsAt.IsZero() {
		r := seconds(snap.Remaining)
		js.Remaining = &r
		js.RemainingText = FormatDuration(snap.Remaining)
	}
	return jsonEvent{
		Time:     ev.Time.Format(time.RFC3339Nano),
		Type:     string(ev.Type),
		Snapshot: js,
		Message:  Text(ev),
		Reason:   string(ev.Reason),
	}
}

func timePtr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.Format(time.RFC3339Nano)
	return &s
}

func seconds(d time.Duration) int64 { return int64(d / time.Second) }

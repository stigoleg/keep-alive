// Package schedule parses weekly work-hour schedules such as
// "Mon-Fri 08:00-16:00" and answers whether a time falls inside one and when
// the next transition is.
//
// # Grammar
//
// Case-insensitive; whitespace around tokens is ignored.
//
//	schedule = rule { ";" rule }              ; empty rules (a trailing ";") are skipped
//	rule     = days windows
//	days     = dayitem { "," dayitem }
//	dayitem  = day | day "-" day | "daily" | "weekdays" | "weekends"
//	day      = Mon | Tue | Wed | Thu | Fri | Sat | Sun, or the full name (Monday ...)
//	windows  = window { "," window }
//	window   = time "-" time
//	time     = H:MM or HH:MM, 00:00 to 23:59; 24:00 is allowed as an end only
//
// A day range may wrap around the week: "Fri-Mon" is Fri, Sat, Sun, Mon.
// "weekdays" is Mon-Fri, "weekends" is Sat and Sun, "daily" is every day.
// A window whose end is earlier than its start spans midnight and belongs to
// its start day: "Fri 22:00-02:00" runs from Friday 22:00 to Saturday 02:00.
// A window whose end equals its start is an error; "00:00-24:00" is a whole
// day. Windows that overlap or touch, also across midnight, behave as one.
//
// Examples: "Mon-Fri 08:00-16:00", "weekdays 08:00-11:30,12:00-16:00; Sat
// 10:00-12:00", "daily 22:00-06:00", "Fri-Mon 09:00-17:00".
//
// # Time zones and daylight saving
//
// A schedule is wall-clock time in the location of the time it is asked about.
// Every window boundary is computed with time.Date for its calendar day, never
// by adding 24 hours to an instant, so a window keeps its wall-clock times on
// the days a daylight-saving change makes 23 or 25 hours long.
//
// Boundaries that fall on a time that does not exist or exists twice follow
// time.Date's normalization:
//
//   - Spring forward: a time inside the gap is moved forward by the length of
//     the gap. In Europe/Oslo on 2026-03-29, 02:30 becomes 03:30 CEST, so
//     "Sun 02:30-04:00" runs 03:30-04:00 that day and "Sun 02:15-02:45" runs
//     03:15-03:45. A window whose start ends up at or after its end
//     ("Sun 02:30-03:00") does not happen that day.
//   - Fall back: time.Date picks one of the two occurrences and Go does not
//     promise which. In Europe/Oslo (and other zones east of UTC) it is the
//     second, standard-time one: on 2026-10-25, 02:30 is 02:30 CET, so
//     "Sun 02:30-04:00" does not include the first 02:30-02:59 (CEST).
//     Zones west of UTC get the first occurrence.
package schedule

import "time"

// maxMergeSteps bounds the window-merging loops. A schedule that does not
// cover the whole week has a gap at least once a week, so the loops end long
// before this; it only guards against a bug turning into a hang.
const maxMergeSteps = 10000

// Schedule is a parsed weekly schedule. The zero value is not useful; use
// Parse.
type Schedule struct {
	days   [7][]window // indexed by time.Weekday, sorted by start
	always bool        // the windows cover the whole week
}

// span is one concrete occurrence of a window.
type span struct{ start, end time.Time }

func (sp span) contains(t time.Time) bool { return !t.Before(sp.start) && t.Before(sp.end) }

// In reports whether t falls inside a window, in t's location.
func (s *Schedule) In(t time.Time) bool {
	if s.always {
		return true
	}
	for _, sp := range s.spans(t, -2, 1) {
		if sp.contains(t) {
			return true
		}
	}
	return false
}

// Next returns the next transition strictly after t: the end of the window t
// is in (entering false) or the start of the next window (entering true).
// Overlapping and touching windows count as one, so there is never a
// transition between them. A schedule that covers the whole week has no
// transitions; Next then returns the zero time and false.
func (s *Schedule) Next(t time.Time) (at time.Time, entering bool) {
	if s.always {
		return time.Time{}, false
	}
	if _, end, ok := s.Current(t); ok {
		return end, false
	}
	// Every week has a window, but daylight saving can remove the only one
	// on a given day, so look two weeks ahead.
	for _, sp := range s.spans(t, -2, 15) {
		if sp.start.After(t) && (at.IsZero() || sp.start.Before(at)) {
			at = sp.start
		}
	}
	return at, !at.IsZero()
}

// Current returns the window t is in, with overlapping and touching windows
// merged into one, and ok false when t is outside every window. For a
// schedule that covers the whole week, start and end are zero and ok is true.
func (s *Schedule) Current(t time.Time) (start, end time.Time, ok bool) {
	if s.always {
		return time.Time{}, time.Time{}, true
	}
	for _, sp := range s.spans(t, -2, 1) {
		if !sp.contains(t) {
			continue
		}
		if !ok || sp.start.Before(start) {
			start = sp.start
		}
		if !ok || sp.end.After(end) {
			end = sp.end
		}
		ok = true
	}
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	return s.extendBack(start), s.extendForward(end), true
}

// extendForward follows windows that overlap or touch end and returns the
// end of the merged run.
func (s *Schedule) extendForward(end time.Time) time.Time {
	for range maxMergeSteps {
		next := end
		for _, sp := range s.spans(end, -2, 1) {
			if !sp.start.After(end) && sp.end.After(next) {
				next = sp.end
			}
		}
		if next.Equal(end) {
			break
		}
		end = next
	}
	return end
}

// extendBack follows windows that overlap or touch start and returns the
// start of the merged run.
func (s *Schedule) extendBack(start time.Time) time.Time {
	for range maxMergeSteps {
		prev := start
		for _, sp := range s.spans(start, -2, 0) {
			if sp.start.Before(prev) && !sp.end.Before(start) {
				prev = sp.start
			}
		}
		if prev.Equal(start) {
			break
		}
		start = prev
	}
	return start
}

// spans returns the windows that start on the calendar days from..to
// relative to t's date, in t's location. A window starts at most one day
// before the day it ends on, so days -2..+1 hold every window that can
// contain t (the extra day absorbs daylight-saving shifts).
func (s *Schedule) spans(t time.Time, from, to int) []span {
	y, m, d := t.Date()
	loc := t.Location()
	var out []span
	for i := from; i <= to; i++ {
		wd := time.Date(y, m, d+i, 0, 0, 0, 0, time.UTC).Weekday()
		for _, w := range s.days[wd] {
			start := time.Date(y, m, d+i, 0, w.start, 0, 0, loc)
			end := time.Date(y, m, d+i, 0, w.end, 0, 0, loc)
			if end.After(start) { // false when daylight saving swallows the window
				out = append(out, span{start, end})
			}
		}
	}
	return out
}

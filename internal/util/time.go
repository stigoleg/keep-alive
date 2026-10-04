package util

import (
	"fmt"
	"strings"
	"time"
)

var clockLayouts = []string{"15:04", "3:04PM", "3:04 PM"}

// ParseClock parses a time of day in 24-hour ("22:00", "9:45") or 12-hour
// ("10:00PM", "10:00 pm") form.
func ParseClock(s string) (hour, minute int, err error) {
	norm := strings.ToUpper(strings.TrimSpace(s))
	for _, layout := range clockLayouts {
		if t, err := time.Parse(layout, norm); err == nil {
			return t.Hour(), t.Minute(), nil
		}
	}
	return 0, 0, fmt.Errorf("invalid time %q: use 24-hour HH:MM (22:00) or 12-hour HH:MM AM/PM (10:00PM)", strings.TrimSpace(s))
}

// NextClockTime returns the next occurrence of the time of day s that is at
// least one minute after now. The next day is computed with time.Date, so a
// daylight-saving change in between is handled.
func NextClockTime(s string, now time.Time) (time.Time, error) {
	hour, minute, err := ParseClock(s)
	if err != nil {
		return time.Time{}, err
	}
	y, m, d := now.Date()
	t := time.Date(y, m, d, hour, minute, 0, 0, now.Location())
	if t.Sub(now) < time.Minute {
		t = time.Date(y, m, d+1, hour, minute, 0, 0, now.Location())
	}
	return t, nil
}

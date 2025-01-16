package util

import (
	"fmt"
	"strings"
	"time"
)

// ParseTimeString parses a time string in either 12-hour or 24-hour format
// Supported formats:
// - 24-hour: "HH:MM" (e.g., "23:30", "09:45")
// - 12-hour: "HH:MM[AM|PM]" (e.g., "11:30PM", "09:45AM")
func ParseTimeString(timeStr string) (time.Time, error) {
	return ParseTimeStringWithNow(timeStr, time.Now())
}

// ParseTimeStringWithNow is like ParseTimeString but accepts a custom "now" time
// This is primarily used for testing to ensure consistent results
func ParseTimeStringWithNow(timeStr string, now time.Time) (time.Time, error) {
	timeStr = strings.TrimSpace(strings.ToUpper(timeStr))

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Try 24-hour format first
	if t, err := time.Parse("15:04", timeStr); err == nil {
		return today.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute), nil
	}

	// Try 12-hour format with AM/PM
	formats := []string{"3:04PM", "3:04 PM", "03:04PM", "03:04 PM"}
	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return today.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute), nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s\n\nValid formats:\n"+
		"• 24-hour format: HH:MM (e.g., '23:30', '09:45')\n"+
		"• 12-hour format: HH:MM[AM|PM] (e.g., '11:30PM', '9:45 AM')", timeStr)
}

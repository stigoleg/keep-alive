package util

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

func ParseDuration(input string) (time.Duration, error) {
	if minutes, err := strconv.Atoi(input); err == nil {
		return time.Duration(minutes) * time.Minute, nil
	}

	duration, err := time.ParseDuration(input)
	if err != nil {
		var msg strings.Builder
		msg.WriteString("Invalid duration format: ")
		msg.WriteString(input)
		msg.WriteString("\n\nValid formats:\n")
		msg.WriteString("• A number (e.g., '30' for 30 minutes)\n")
		msg.WriteString("• A duration string:\n")
		msg.WriteString("  - Hours: '2h'\n")
		msg.WriteString("  - Minutes: '30m'\n")
		msg.WriteString("  - Combined: '2h30m', '1h30m'\n")
		return 0, errors.New(msg.String())
	}
	return duration, nil
}

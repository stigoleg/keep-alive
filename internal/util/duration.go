package util

import (
	"fmt"
	"strconv"
	"time"
)

func ParseDuration(input string) (time.Duration, error) {
	if minutes, err := strconv.Atoi(input); err == nil {
		return time.Duration(minutes) * time.Minute, nil
	}

	duration, err := time.ParseDuration(input)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %s", input)
	}
	return duration, nil
}

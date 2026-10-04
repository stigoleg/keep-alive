//go:build darwin

package platform

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

var darwinBatteryPercent = regexp.MustCompile(`(\d+)%`)

func parseDarwinBatteryPercentage(output string) (int, error) {
	matches := darwinBatteryPercent.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0, fmt.Errorf("battery percentage not found")
	}

	percentage, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse battery percentage %q: %v", matches[1], err)
	}
	if percentage < 0 || percentage > 100 {
		return 0, fmt.Errorf("battery percentage out of range: %d", percentage)
	}
	return percentage, nil
}

// GetBatteryStatus reads the internal battery through pmset.
func GetBatteryStatus() (BatteryStatus, error) {
	out, err := exec.Command("pmset", "-g", "batt").CombinedOutput()
	if err != nil {
		return BatteryStatus{}, fmt.Errorf("failed to read battery status: %v", err)
	}

	percentage, err := parseDarwinBatteryPercentage(string(out))
	if err != nil {
		return BatteryStatus{}, err
	}

	return BatteryStatus{Percentage: percentage, Available: true}, nil
}

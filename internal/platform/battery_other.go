//go:build !darwin && !windows && !linux

package platform

import "errors"

// GetBatteryStatus is not supported on this operating system.
func GetBatteryStatus() (BatteryStatus, error) {
	return BatteryStatus{}, errors.New("battery status is unsupported on this platform")
}

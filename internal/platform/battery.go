// Package platform reads the battery level for --battery and doctor.
package platform

// BatteryStatus describes the current battery charge.
type BatteryStatus struct {
	Percentage int
	Available  bool
	// Charging means the machine runs on external power, so the battery
	// is charging or full.
	Charging bool
}

// Package platform reads the battery level for --battery and doctor.
package platform

// BatteryStatus describes the current battery charge.
type BatteryStatus struct {
	Percentage int
	Available  bool
}

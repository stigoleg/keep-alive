//go:build windows

package platform

import "testing"

func TestBatteryPercentageFromWindowsStatus(t *testing.T) {
	got, err := batteryPercentageFromWindowsStatus(systemPowerStatus{BatteryLifePercent: 20})
	if err != nil {
		t.Fatalf("batteryPercentageFromWindowsStatus() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("batteryPercentageFromWindowsStatus() = %d, want 20", got)
	}
}

func TestBatteryPercentageFromWindowsStatusRejectsUnknown(t *testing.T) {
	if _, err := batteryPercentageFromWindowsStatus(systemPowerStatus{BatteryLifePercent: 255}); err == nil {
		t.Fatal("batteryPercentageFromWindowsStatus() expected error")
	}
}

func TestBatteryStatusFromWindowsCharging(t *testing.T) {
	for line, want := range map[byte]bool{0: false, 1: true, 255: false} {
		st, err := batteryStatusFromWindows(systemPowerStatus{ACLineStatus: line, BatteryLifePercent: 40})
		if err != nil || st.Charging != want || st.Percentage != 40 || !st.Available {
			t.Errorf("ACLineStatus %d: %+v, %v", line, st, err)
		}
	}
}

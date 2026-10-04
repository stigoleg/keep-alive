//go:build windows

package platform

import (
	"fmt"
	"syscall"
	"unsafe"
)

var procGetSystemPowerStatus = syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func batteryPercentageFromWindowsStatus(status systemPowerStatus) (int, error) {
	if status.BatteryLifePercent == 255 {
		return 0, fmt.Errorf("battery percentage is unknown")
	}
	if status.BatteryFlag == 128 {
		return 0, fmt.Errorf("no system battery found")
	}
	percentage := int(status.BatteryLifePercent)
	if percentage < 0 || percentage > 100 {
		return 0, fmt.Errorf("battery percentage out of range: %d", percentage)
	}
	return percentage, nil
}

// GetBatteryStatus reads the system battery through GetSystemPowerStatus.
func GetBatteryStatus() (BatteryStatus, error) {
	var status systemPowerStatus
	r1, _, err := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if r1 == 0 {
		return BatteryStatus{}, err
	}

	percentage, err := batteryPercentageFromWindowsStatus(status)
	if err != nil {
		return BatteryStatus{}, err
	}
	return BatteryStatus{Percentage: percentage, Available: true}, nil
}

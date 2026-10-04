//go:build linux

package platform

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const powerSupplyRoot = "/sys/class/power_supply"

func parseLinuxBatteryCapacity(value string) (int, error) {
	percentage, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("failed to parse battery capacity %q: %v", strings.TrimSpace(value), err)
	}
	if percentage < 0 || percentage > 100 {
		return 0, fmt.Errorf("battery capacity out of range: %d", percentage)
	}
	return percentage, nil
}

// linuxBattery is one system battery under /sys/class/power_supply.
type linuxBattery struct {
	name     string
	capacity int // -1 when the capacity file is missing or unreadable
	// energy holds energy_now/energy_full (µWh); charge holds
	// charge_now/charge_full (µAh). Zero full means not reported.
	energyNow, energyFull float64
	chargeNow, chargeFull float64
}

func (b linuxBattery) percent() (int, bool) {
	switch {
	case b.capacity >= 0:
		return b.capacity, true
	case b.energyFull > 0:
		return ratioPercent(b.energyNow, b.energyFull), true
	case b.chargeFull > 0:
		return ratioPercent(b.chargeNow, b.chargeFull), true
	}
	return 0, false
}

func ratioPercent(now, full float64) int {
	return min(100, max(0, int(math.Round(100*now/full))))
}

// readLinuxBatteries returns the system batteries: peripheral ones
// (scope=Device, e.g. a wireless mouse) and empty bays (present=0) are
// skipped.
func readLinuxBatteries(root string) ([]linuxBattery, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to read power supply directory: %v", err)
	}
	read := func(dir, file string) (string, bool) {
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(b)), true
	}
	number := func(dir, file string) float64 {
		s, ok := read(dir, file)
		if !ok {
			return 0
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < 0 {
			return 0
		}
		return v
	}

	var batteries []linuxBattery
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		if typ, _ := read(dir, "type"); typ != "Battery" {
			continue
		}
		if scope, _ := read(dir, "scope"); strings.EqualFold(scope, "Device") {
			continue
		}
		if present, ok := read(dir, "present"); ok && present == "0" {
			continue
		}
		b := linuxBattery{name: entry.Name(), capacity: -1}
		if s, ok := read(dir, "capacity"); ok {
			if c, err := parseLinuxBatteryCapacity(s); err == nil {
				b.capacity = c
			}
		}
		b.energyNow, b.energyFull = number(dir, "energy_now"), number(dir, "energy_full")
		b.chargeNow, b.chargeFull = number(dir, "charge_now"), number(dir, "charge_full")
		batteries = append(batteries, b)
	}
	if len(batteries) == 0 {
		return nil, fmt.Errorf("no battery found")
	}
	return batteries, nil
}

// aggregateLinuxBatteries combines system batteries into one level: by
// summed energy (or charge) when every battery reports it, else the mean of
// their percentages.
func aggregateLinuxBatteries(batteries []linuxBattery) (int, error) {
	for _, b := range batteries {
		if _, ok := b.percent(); !ok {
			return 0, fmt.Errorf("battery %s reports no charge level", b.name)
		}
	}
	if len(batteries) == 1 {
		p, _ := batteries[0].percent()
		return p, nil
	}
	var energyNow, energyFull, chargeNow, chargeFull float64
	allEnergy, allCharge := true, true
	for _, b := range batteries {
		allEnergy = allEnergy && b.energyFull > 0
		allCharge = allCharge && b.chargeFull > 0
		energyNow, energyFull = energyNow+b.energyNow, energyFull+b.energyFull
		chargeNow, chargeFull = chargeNow+b.chargeNow, chargeFull+b.chargeFull
	}
	switch {
	case allEnergy:
		return ratioPercent(energyNow, energyFull), nil
	case allCharge:
		return ratioPercent(chargeNow, chargeFull), nil
	}
	sum := 0
	for _, b := range batteries {
		p, _ := b.percent()
		sum += p
	}
	return int(math.Round(float64(sum) / float64(len(batteries)))), nil
}

func linuxBatteryPercent(root string) (int, error) {
	batteries, err := readLinuxBatteries(root)
	if err != nil {
		return 0, err
	}
	return aggregateLinuxBatteries(batteries)
}

// linuxOnExternalPower reports whether a mains or USB supply is online, or
// a system battery says it is charging.
func linuxOnExternalPower(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		read := func(file string) string {
			b, _ := os.ReadFile(filepath.Join(root, entry.Name(), file))
			return strings.TrimSpace(string(b))
		}
		switch read("type") {
		case "Mains", "USB":
			if read("online") == "1" {
				return true
			}
		case "Battery":
			if !strings.EqualFold(read("scope"), "Device") && read("status") == "Charging" {
				return true
			}
		}
	}
	return false
}

// GetBatteryStatus reads the system batteries from sysfs.
func GetBatteryStatus() (BatteryStatus, error) {
	percentage, err := linuxBatteryPercent(powerSupplyRoot)
	if err != nil {
		return BatteryStatus{}, err
	}
	return BatteryStatus{Percentage: percentage, Available: true, Charging: linuxOnExternalPower(powerSupplyRoot)}, nil
}

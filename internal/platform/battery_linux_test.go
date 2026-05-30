//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLinuxBatteryCapacity(t *testing.T) {
	got, err := parseLinuxBatteryCapacity("20\n")
	if err != nil {
		t.Fatalf("parseLinuxBatteryCapacity() error = %v", err)
	}
	if got != 20 {
		t.Fatalf("parseLinuxBatteryCapacity() = %d, want 20", got)
	}
}

func TestReadLinuxBatteryCapacities(t *testing.T) {
	root := t.TempDir()
	writePowerSupply(t, root, "AC0", "Mains", "100")
	writePowerSupply(t, root, "BAT0", "Battery", "42")
	writePowerSupply(t, root, "BAT1", "Battery", "37")

	got, err := readLinuxBatteryCapacities(root)
	if err != nil {
		t.Fatalf("readLinuxBatteryCapacities() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("readLinuxBatteryCapacities() length = %d, want 2", len(got))
	}

	lowest, err := lowestBatteryCapacity(got)
	if err != nil {
		t.Fatalf("lowestBatteryCapacity() error = %v", err)
	}
	if lowest != 37 {
		t.Fatalf("lowestBatteryCapacity() = %d, want 37", lowest)
	}
}

func writePowerSupply(t *testing.T, root, name, supplyType, capacity string) {
	t.Helper()

	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "type"), []byte(supplyType), 0o644); err != nil {
		t.Fatalf("write type: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "capacity"), []byte(capacity), 0o644); err != nil {
		t.Fatalf("write capacity: %v", err)
	}
}

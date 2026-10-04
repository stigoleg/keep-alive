//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLinuxBatteryPercent(t *testing.T) {
	tests := []struct {
		name     string
		supplies map[string]map[string]string
		want     int
		wantErr  string
	}{
		{
			name: "one battery with capacity",
			supplies: map[string]map[string]string{
				"AC0":  {"type": "Mains", "online": "1"},
				"BAT0": {"type": "Battery", "capacity": "42"},
			},
			want: 42,
		},
		{
			name: "peripheral batteries are ignored",
			supplies: map[string]map[string]string{
				"BAT0":                    {"type": "Battery", "capacity": "80"},
				"hidpp_battery_0":         {"type": "Battery", "scope": "Device", "capacity": "5"},
				"ps-controller-battery-1": {"type": "Battery", "scope": "Device\n"},
			},
			want: 80,
		},
		{
			name: "missing capacity falls back to energy",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "energy_now": "30000000", "energy_full": "40000000"},
			},
			want: 75,
		},
		{
			name: "missing capacity falls back to charge",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "charge_now": "1000000", "charge_full": "4000000"},
			},
			want: 25,
		},
		{
			name: "two batteries are summed by energy, not the minimum",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "capacity": "90", "energy_now": "45000000", "energy_full": "50000000"},
				"BAT1": {"type": "Battery", "capacity": "10", "energy_now": "2300000", "energy_full": "23000000"},
			},
			want: 65, // 47.3 / 73 Wh
		},
		{
			name: "two batteries with charge only",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "charge_now": "3000000", "charge_full": "4000000"},
				"BAT1": {"type": "Battery", "charge_now": "1000000", "charge_full": "4000000"},
			},
			want: 50,
		},
		{
			name: "two batteries without energy average their capacity",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "capacity": "90"},
				"BAT1": {"type": "Battery", "capacity": "40"},
			},
			want: 65,
		},
		{
			name: "an absent second battery is skipped",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "capacity": "55"},
				"BAT1": {"type": "Battery", "present": "0"},
			},
			want: 55,
		},
		{
			name: "energy above full is clamped",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "energy_now": "41000000", "energy_full": "40000000"},
			},
			want: 100,
		},
		{
			name:     "desktop without a battery",
			supplies: map[string]map[string]string{"AC0": {"type": "Mains"}},
			wantErr:  "no battery found",
		},
		{
			name: "only peripheral batteries",
			supplies: map[string]map[string]string{
				"hidpp_battery_0": {"type": "Battery", "scope": "Device", "capacity": "50"},
			},
			wantErr: "no battery found",
		},
		{
			name: "battery without any level",
			supplies: map[string]map[string]string{
				"BAT0": {"type": "Battery", "status": "Unknown"},
			},
			wantErr: "BAT0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, files := range tt.supplies {
				writePowerSupply(t, root, name, files)
			}
			got, err := linuxBatteryPercent(root)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("linuxBatteryPercent() = %d, %v; want error containing %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("linuxBatteryPercent() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("linuxBatteryPercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

func writePowerSupply(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for file, content := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
}

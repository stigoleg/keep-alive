package config

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestParseFlags(t *testing.T) {
	// Save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Use a fixed time for consistent testing
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local) // 10:00 AM

	tests := []struct {
		name        string
		args        []string
		wantMinutes int
		wantBattery int
		wantErr     bool
		wantVersion bool
	}{
		{
			name:        "valid duration flag",
			args:        []string{"keepalive", "-d", "2h30m"},
			wantMinutes: 150,
		},
		{
			name:        "valid duration short flag",
			args:        []string{"keepalive", "-d", "150"},
			wantMinutes: 150,
		},
		{
			name: "valid clock 24h format",
			args: []string{"keepalive", "-c", "22:30"},
		},
		{
			name: "valid clock 12h format PM",
			args: []string{"keepalive", "-c", "10:30PM"},
		},
		{
			name: "valid clock 12h format AM",
			args: []string{"keepalive", "-c", "09:45AM"},
		},
		{
			name:    "invalid clock format",
			args:    []string{"keepalive", "-c", "25:00"},
			wantErr: true,
		},
		{
			name:    "both duration and clock flags",
			args:    []string{"keepalive", "-d", "2h30m", "-c", "22:30"},
			wantErr: true,
		},
		{
			name:        "version flag",
			args:        []string{"keepalive", "--version"},
			wantVersion: true,
		},
		{
			name:        "valid battery flag",
			args:        []string{"keepalive", "-b", "20"},
			wantBattery: 20,
		},
		{
			name:        "valid battery long flag",
			args:        []string{"keepalive", "--battery", "30"},
			wantBattery: 30,
		},
		{
			name:        "battery combines with duration",
			args:        []string{"keepalive", "-d", "20", "-b", "65"},
			wantMinutes: 20,
			wantBattery: 65,
		},
		{
			name:        "battery combines with clock",
			args:        []string{"keepalive", "-c", "12:00", "-b", "65"},
			wantMinutes: 120,
			wantBattery: 65,
		},
		{
			name:    "duration and clock still conflict with battery",
			args:    []string{"keepalive", "-b", "25", "-d", "2h", "-c", "22:00"},
			wantErr: true,
		},
		{
			name:    "battery rejects zero",
			args:    []string{"keepalive", "-b", "0"},
			wantErr: true,
		},
		{
			name:    "battery rejects negative",
			args:    []string{"keepalive", "-b", "-1"},
			wantErr: true,
		},
		{
			name:    "battery rejects above one hundred",
			args:    []string{"keepalive", "-b", "101"},
			wantErr: true,
		},
		{
			name:    "battery rejects non integer",
			args:    []string{"keepalive", "-b", "twenty"},
			wantErr: true,
		},
		{
			name:        "no flags",
			args:        []string{"keepalive"},
			wantMinutes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test args
			os.Args = tt.args

			cfg, err := ParseFlagsWithNow("test-version", now)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseFlags() expected error")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseFlags() unexpected error: %v", err)
				return
			}
			if cfg.ShowVersion != tt.wantVersion {
				t.Errorf("ParseFlags() ShowVersion = %v, want %v", cfg.ShowVersion, tt.wantVersion)
			}
			if tt.wantVersion {
				return
			}
			if cfg.BatteryThreshold != tt.wantBattery {
				t.Errorf("ParseFlags() BatteryThreshold = %d, want %d", cfg.BatteryThreshold, tt.wantBattery)
			}

			if tt.wantMinutes != 0 && cfg.Duration != tt.wantMinutes {
				t.Errorf("ParseFlags() got duration %d, want %d", cfg.Duration, tt.wantMinutes)
			}

			// For clock flag tests, verify the time is in the future
			if len(tt.args) > 2 && tt.args[1] == "-c" && !cfg.Clock.IsZero() {
				if !cfg.Clock.After(now) {
					t.Errorf("ParseFlags() clock time %v should be in the future", cfg.Clock)
				}
			}
		})
	}
}

func TestParseFlagsTimeCalculation(t *testing.T) {
	// Save original args and restore them after the test
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Use a fixed time for testing
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local) // 10:00 AM
	targetHour := 12                                      // 12:00 PM (2 hours from now)
	timeStr := fmt.Sprintf("%02d:00", targetHour)

	os.Args = []string{"keepalive", "-c", timeStr}

	cfg, err := ParseFlagsWithNow("test-version", now)
	if err != nil {
		t.Fatalf("ParseFlags() unexpected error: %v", err)
	}

	// Verify the clock time is set correctly
	if cfg.Clock.Hour() != targetHour {
		t.Errorf("ParseFlags() clock hour %d, want %d", cfg.Clock.Hour(), targetHour)
	}
	if cfg.Clock.Minute() != 0 {
		t.Errorf("ParseFlags() clock minute %d, want 0", cfg.Clock.Minute())
	}

	// Verify the duration is exactly 2 hours
	if cfg.Duration != 120 {
		t.Errorf("ParseFlags() duration %d minutes, want exactly 120 minutes", cfg.Duration)
	}
}

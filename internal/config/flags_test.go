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
		skip        bool // Skip test cases that would cause os.Exit
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
			name: "invalid clock format",
			args: []string{"keepalive", "-c", "25:00"},
			skip: true, // Would cause os.Exit(1)
		},
		{
			name: "both duration and clock flags",
			args: []string{"keepalive", "-d", "2h30m", "-c", "22:30"},
			skip: true, // Would cause os.Exit(1)
		},
		{
			name: "version flag",
			args: []string{"keepalive", "--version"},
			skip: true, // Would cause os.Exit(0)
		},
		{
			name:        "no flags",
			args:        []string{"keepalive"},
			wantMinutes: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("Skipping test case that would cause os.Exit")
			}

			// Set up test args
			os.Args = tt.args

			cfg, err := ParseFlagsWithNow("test-version", now)
			if err != nil {
				t.Errorf("ParseFlags() unexpected error: %v", err)
				return
			}

			// Skip duration check for clock flag tests
			if len(tt.args) > 1 && tt.args[1] != "-c" && cfg.Duration != tt.wantMinutes {
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

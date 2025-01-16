package util

import (
	"testing"
	"time"
)

func TestParseTimeString(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	tests := []struct {
		name      string
		timeStr   string
		wantHour  int
		wantMin   int
		wantError bool
	}{
		// 24-hour format tests
		{
			name:      "valid 24h time - evening",
			timeStr:   "22:30",
			wantHour:  22,
			wantMin:   30,
			wantError: false,
		},
		{
			name:      "valid 24h time - morning",
			timeStr:   "09:45",
			wantHour:  9,
			wantMin:   45,
			wantError: false,
		},
		{
			name:      "valid 24h time - midnight",
			timeStr:   "00:00",
			wantHour:  0,
			wantMin:   0,
			wantError: false,
		},
		{
			name:      "valid 24h time - noon",
			timeStr:   "12:00",
			wantHour:  12,
			wantMin:   0,
			wantError: false,
		},

		// 12-hour format tests
		{
			name:      "valid 12h time - PM",
			timeStr:   "10:30PM",
			wantHour:  22,
			wantMin:   30,
			wantError: false,
		},
		{
			name:      "valid 12h time - AM",
			timeStr:   "09:45AM",
			wantHour:  9,
			wantMin:   45,
			wantError: false,
		},
		{
			name:      "valid 12h time - with space PM",
			timeStr:   "10:30 PM",
			wantHour:  22,
			wantMin:   30,
			wantError: false,
		},
		{
			name:      "valid 12h time - with space AM",
			timeStr:   "09:45 AM",
			wantHour:  9,
			wantMin:   45,
			wantError: false,
		},
		{
			name:      "valid 12h time - lowercase am",
			timeStr:   "09:45am",
			wantHour:  9,
			wantMin:   45,
			wantError: false,
		},
		{
			name:      "valid 12h time - mixed case Pm",
			timeStr:   "10:30Pm",
			wantHour:  22,
			wantMin:   30,
			wantError: false,
		},

		// Error cases
		{
			name:      "invalid format - no minutes",
			timeStr:   "22:",
			wantError: true,
		},
		{
			name:      "invalid format - no separator",
			timeStr:   "2230",
			wantError: true,
		},
		{
			name:      "invalid format - wrong separator",
			timeStr:   "22.30",
			wantError: true,
		},
		{
			name:      "invalid format - extra characters",
			timeStr:   "22:30xyz",
			wantError: true,
		},
		{
			name:      "invalid format - out of range hours",
			timeStr:   "25:00",
			wantError: true,
		},
		{
			name:      "invalid format - out of range minutes",
			timeStr:   "22:60",
			wantError: true,
		},
		{
			name:      "invalid format - empty string",
			timeStr:   "",
			wantError: true,
		},
		{
			name:      "invalid format - spaces only",
			timeStr:   "   ",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeString(tt.timeStr)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseTimeString(%q) expected error but got none", tt.timeStr)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseTimeString(%q) unexpected error: %v", tt.timeStr, err)
				return
			}

			// Check if the parsed time matches expected hour and minute
			if got.Hour() != tt.wantHour {
				t.Errorf("ParseTimeString(%q) got hour %d, want %d", tt.timeStr, got.Hour(), tt.wantHour)
			}
			if got.Minute() != tt.wantMin {
				t.Errorf("ParseTimeString(%q) got minute %d, want %d", tt.timeStr, got.Minute(), tt.wantMin)
			}

			// Verify the date is today
			if got.Year() != today.Year() || got.Month() != today.Month() || got.Day() != today.Day() {
				t.Errorf("ParseTimeString(%q) got date %v, want today's date", tt.timeStr, got)
			}
		})
	}
}

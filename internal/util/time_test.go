package util

import (
	"testing"
	"time"
)

func TestParseClock(t *testing.T) {
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
			hour, minute, err := ParseClock(tt.timeStr)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseClock(%q) expected error but got none", tt.timeStr)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseClock(%q) unexpected error: %v", tt.timeStr, err)
				return
			}
			if hour != tt.wantHour || minute != tt.wantMin {
				t.Errorf("ParseClock(%q) = %02d:%02d, want %02d:%02d", tt.timeStr, hour, minute, tt.wantHour, tt.wantMin)
			}
		})
	}
}

func TestNextClockTime(t *testing.T) {
	loc := time.FixedZone("test", 3600)
	now := time.Date(2026, 6, 10, 10, 0, 30, 0, loc)
	tests := []struct {
		in   string
		want time.Time
	}{
		{"22:00", time.Date(2026, 6, 10, 22, 0, 0, 0, loc)},
		{"10:00PM", time.Date(2026, 6, 10, 22, 0, 0, 0, loc)},
		{"10:00 pm", time.Date(2026, 6, 10, 22, 0, 0, 0, loc)},
		{"9:45", time.Date(2026, 6, 11, 9, 45, 0, 0, loc)},  // earlier today: tomorrow
		{"10:00", time.Date(2026, 6, 11, 10, 0, 0, 0, loc)}, // the current minute: tomorrow
		{"10:01", time.Date(2026, 6, 11, 10, 1, 0, 0, loc)}, // under a minute away: tomorrow
		{"10:02", time.Date(2026, 6, 10, 10, 2, 0, 0, loc)},
	}
	for _, tt := range tests {
		got, err := NextClockTime(tt.in, now)
		if err != nil || !got.Equal(tt.want) {
			t.Errorf("NextClockTime(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	if _, err := NextClockTime("25:00", now); err == nil {
		t.Error("NextClockTime accepted 25:00")
	}
}

func TestNextClockTimeAcrossDST(t *testing.T) {
	oslo, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		t.Skipf("no tz database: %v", err)
	}
	// 2026-03-29 has 23 hours in Oslo; Add(24h) would land on 00:00.
	now := time.Date(2026, 3, 28, 23, 0, 0, 0, oslo)
	got, err := NextClockTime("23:00", now)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 29, 23, 0, 0, 0, oslo); !got.Equal(want) {
		t.Fatalf("NextClockTime across DST = %v, want %v", got, want)
	}
}

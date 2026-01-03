package util

import (
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  time.Duration
		wantError bool
	}{
		// Integer minutes
		{
			name:     "integer minutes - 30",
			input:    "30",
			expected: 30 * time.Minute,
		},
		{
			name:     "integer minutes - 0",
			input:    "0",
			expected: 0,
		},
		{
			name:     "integer minutes - 120",
			input:    "120",
			expected: 120 * time.Minute,
		},

		// Duration strings
		{
			name:     "duration string - hours only",
			input:    "2h",
			expected: 2 * time.Hour,
		},
		{
			name:     "duration string - minutes only",
			input:    "45m",
			expected: 45 * time.Minute,
		},
		{
			name:     "duration string - hours and minutes",
			input:    "2h30m",
			expected: 2*time.Hour + 30*time.Minute,
		},
		{
			name:     "duration string - with seconds",
			input:    "1h30m45s",
			expected: 1*time.Hour + 30*time.Minute + 45*time.Second,
		},

		// Error cases
		{
			name:      "invalid format - letters",
			input:     "abc",
			wantError: true,
		},
		{
			name:      "invalid format - mixed invalid",
			input:     "2x30m",
			wantError: true,
		},
		{
			name:      "empty string",
			input:     "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseDuration(%q) expected error but got none", tt.input)
				}
				// Verify error message contains helpful format info
				if err != nil && !strings.Contains(err.Error(), "Valid formats") {
					t.Errorf("ParseDuration(%q) error should contain format help, got: %v", tt.input, err)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseDuration(%q) unexpected error: %v", tt.input, err)
				return
			}

			if got != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

package util

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Plain integer (minutes)
		{name: "integer minutes", input: "30", want: 30 * time.Minute},
		{name: "single minute", input: "1", want: 1 * time.Minute},
		{name: "large minutes", input: "480", want: 480 * time.Minute},
		{name: "zero", input: "0", want: 0},

		// Go duration strings
		{name: "hours only", input: "2h", want: 2 * time.Hour},
		{name: "minutes only", input: "30m", want: 30 * time.Minute},
		{name: "hours and minutes", input: "2h30m", want: 2*time.Hour + 30*time.Minute},
		{name: "complex duration", input: "1h30m45s", want: 1*time.Hour + 30*time.Minute + 45*time.Second},
		{name: "seconds only", input: "90s", want: 90 * time.Second},

		// Invalid inputs
		{name: "empty string", input: "", wantErr: true},
		{name: "letters only", input: "abc", wantErr: true},
		{name: "negative integer", input: "-5", wantErr: false, want: -5 * time.Minute}, // Atoi parses negative
		{name: "invalid duration string", input: "2x30y", wantErr: true},
		{name: "spaces only", input: "   ", wantErr: true},
		{name: "mixed garbage", input: "12abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseDuration(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseDuration(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

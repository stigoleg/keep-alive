package util

import "testing"

func TestHasCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{
			name:     "common command exists - go",
			command:  "go",
			expected: true,
		},
		{
			name:     "nonexistent command",
			command:  "this-command-definitely-does-not-exist-12345",
			expected: false,
		},
		{
			name:     "empty string",
			command:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasCommand(tt.command)
			if got != tt.expected {
				t.Errorf("HasCommand(%q) = %v, want %v", tt.command, got, tt.expected)
			}
		})
	}
}

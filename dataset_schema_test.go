package main

import "testing"

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		expected   string
	}{
		{
			name:       "simple identifier",
			identifier: "people",
			expected:   `"people"`,
		},
		{
			name:       "identifier with uppercase and space",
			identifier: "People Table",
			expected:   `"People Table"`,
		},
		{
			name:       "identifier with internal quote",
			identifier: `bad"name`,
			expected:   `"bad""name"`,
		},
		{
			name:       "already quoted identifier is escaped",
			identifier: `"people"`,
			expected:   `"""people"""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteIdentifier(tt.identifier); got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

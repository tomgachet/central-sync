package main

import (
	"testing"
	"time"
)

func timePtr(t *testing.T, value string) *time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", value, err)
	}

	return &parsed
}

func TestFormatODataDateTimeUsesUTCWithMilliseconds(t *testing.T) {
	value := time.Date(2026, 6, 23, 14, 5, 6, 789123000, time.FixedZone("CEST", 2*60*60))

	if got := formatODataDateTime(value); got != "2026-06-23T12:05:06.789Z" {
		t.Fatalf("expected UTC timestamp with milliseconds, got %q", got)
	}
}

func TestBuildSubmissionRootFilter(t *testing.T) {
	submissionDate := timePtr(t, "2026-06-01T10:00:00Z")
	updatedAt := timePtr(t, "2026-06-02T11:30:00Z")

	tests := []struct {
		name         string
		lastSync     *LastSuccessfulSubmissionSync
		syncMode     string
		approvedOnly bool
		expected     string
	}{
		{
			name:     "no last sync without approved filter",
			lastSync: nil,
			syncMode: SyncModeAppendOnly,
			expected: "",
		},
		{
			name:         "approved only without last sync",
			lastSync:     nil,
			syncMode:     SyncModeAppendOnly,
			approvedOnly: true,
			expected:     "__system/reviewState eq 'approved'",
		},
		{
			name: "append only uses submission date",
			lastSync: &LastSuccessfulSubmissionSync{
				MaxSubmissionDate: submissionDate,
				MaxUpdatedAt:      updatedAt,
			},
			syncMode: SyncModeAppendOnly,
			expected: "__system/submissionDate gt 2026-06-01T10:00:00.000Z",
		},
		{
			name: "upsert uses submission date and updated at",
			lastSync: &LastSuccessfulSubmissionSync{
				MaxSubmissionDate: submissionDate,
				MaxUpdatedAt:      updatedAt,
			},
			syncMode: "upsert",
			expected: "(__system/submissionDate gt 2026-06-01T10:00:00.000Z) or (__system/updatedAt gt 2026-06-02T11:30:00.000Z)",
		},
		{
			name: "upsert with approved only combines filters",
			lastSync: &LastSuccessfulSubmissionSync{
				MaxSubmissionDate: submissionDate,
				MaxUpdatedAt:      updatedAt,
			},
			syncMode:     SyncModeUpsert,
			approvedOnly: true,
			expected:     "((__system/submissionDate gt 2026-06-01T10:00:00.000Z) or (__system/updatedAt gt 2026-06-02T11:30:00.000Z)) and (__system/reviewState eq 'approved')",
		},
		{
			name: "upsert with only updated at",
			lastSync: &LastSuccessfulSubmissionSync{
				MaxUpdatedAt: updatedAt,
			},
			syncMode: SyncModeUpsert,
			expected: "__system/updatedAt gt 2026-06-02T11:30:00.000Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSubmissionRootFilter(tt.lastSync, tt.syncMode, tt.approvedOnly)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestBuildSubmissionRepeatFilterUsesRootPrefix(t *testing.T) {
	lastSync := &LastSuccessfulSubmissionSync{
		MaxSubmissionDate: timePtr(t, "2026-06-01T10:00:00Z"),
	}

	got := buildSubmissionRepeatFilter(lastSync, SyncModeAppendOnly, true)
	expected := "($root/Submissions/__system/submissionDate gt 2026-06-01T10:00:00.000Z) and ($root/Submissions/__system/reviewState eq 'approved')"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

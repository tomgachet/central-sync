package main

import (
	"fmt"
	"time"
)

func buildSubmissionRootFilter(lastSync *LastSuccessfulSubmissionSync, syncMode string) string {
	return buildSubmissionFilter(lastSync, syncMode, "__system/")
}

func buildSubmissionRepeatFilter(lastSync *LastSuccessfulSubmissionSync, syncMode string) string {
	return buildSubmissionFilter(lastSync, syncMode, "$root/Submissions/__system/")
}

func buildSubmissionFilter(lastSync *LastSuccessfulSubmissionSync, syncMode string, systemPrefix string) string {
	if lastSync == nil {
		return ""
	}

	switch syncMode {
	case SyncModeAppendOnly:
		if lastSync.MaxSubmissionDate == nil {
			return ""
		}

		return fmt.Sprintf(
			"%ssubmissionDate gt %s",
			systemPrefix,
			formatODataDateTime(*lastSync.MaxSubmissionDate),
		)

	case SyncModeUpsert:
		var parts []string

		if lastSync.MaxSubmissionDate != nil {
			parts = append(parts,
				fmt.Sprintf(
					"%ssubmissionDate gt %s",
					systemPrefix,
					formatODataDateTime(*lastSync.MaxSubmissionDate),
				),
			)
		}

		if lastSync.MaxUpdatedAt != nil {
			parts = append(parts,
				fmt.Sprintf(
					"%supdatedAt gt %s",
					systemPrefix,
					formatODataDateTime(*lastSync.MaxUpdatedAt),
				),
			)
		}

		if len(parts) == 0 {
			return ""
		}
		if len(parts) == 1 {
			return parts[0]
		}

		return fmt.Sprintf("(%s) or (%s)", parts[0], parts[1])

	default:
		return ""
	}
}

func formatODataDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
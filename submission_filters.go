package main

import (
	"fmt"
	"time"
)

func buildSubmissionRootFilter(lastRun *SyncRun, syncMode string) string {
	return buildSubmissionFilter(lastRun, syncMode, "__system/")
}

func buildSubmissionRepeatFilter(lastRun *SyncRun, syncMode string) string {
	return buildSubmissionFilter(lastRun, syncMode, "$root/Submissions/__system/")
}

func buildSubmissionFilter(lastRun *SyncRun, syncMode string, systemPrefix string) string {
	if lastRun == nil {
		return ""
	}

	switch syncMode {
	case SyncModeAppendOnly:
		if lastRun.SyncOutSubmissionDate == nil {
			return ""
		}

		return fmt.Sprintf(
			"%ssubmissionDate gt %s",
			systemPrefix,
			formatODataDateTime(*lastRun.SyncOutSubmissionDate),
		)

	case SyncModeUpsert:
		var parts []string

		if lastRun.SyncOutSubmissionDate != nil {
			parts = append(parts,
				fmt.Sprintf(
					"%ssubmissionDate gt %s",
					systemPrefix,
					formatODataDateTime(*lastRun.SyncOutSubmissionDate),
				),
			)
		}

		if lastRun.SyncOutUpdatedAt != nil {
			parts = append(parts,
				fmt.Sprintf(
					"%supdatedAt gt %s",
					systemPrefix,
					formatODataDateTime(*lastRun.SyncOutUpdatedAt),
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
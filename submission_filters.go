package main

import (
	"fmt"
	"time"
)

func buildSubmissionRootFilter(
	lastSync *LastSuccessfulSubmissionSync,
	syncMode string,
	approvedOnly bool,
) string {
	return buildSubmissionFilter(lastSync, syncMode, "__system/", approvedOnly)
}

func buildSubmissionRepeatFilter(
	lastSync *LastSuccessfulSubmissionSync,
	syncMode string,
	approvedOnly bool,
) string {
	return buildSubmissionFilter(lastSync, syncMode, "$root/Submissions/__system/", approvedOnly)
}

func buildSubmissionFilter(
	lastSync *LastSuccessfulSubmissionSync,
	syncMode string,
	systemPrefix string,
	approvedOnly bool,
) string {
	var baseFilter string

	if lastSync != nil {
		switch syncMode {
		case SyncModeAppendOnly:
			if lastSync.MaxSubmissionDate != nil {
				baseFilter = fmt.Sprintf(
					"%ssubmissionDate gt %s",
					systemPrefix,
					formatODataDateTime(*lastSync.MaxSubmissionDate),
				)
			}

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

			if len(parts) == 1 {
				baseFilter = parts[0]
			} else if len(parts) >= 2 {
				baseFilter = fmt.Sprintf("(%s) or (%s)", parts[0], parts[1])
			}
		}
	}

	if !approvedOnly {
		return baseFilter
	}

	approvedFilter := fmt.Sprintf("%sreviewState eq 'approved'", systemPrefix)

	if baseFilter == "" {
		return approvedFilter
	}

	return fmt.Sprintf("(%s) and (%s)", baseFilter, approvedFilter)
}

func formatODataDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
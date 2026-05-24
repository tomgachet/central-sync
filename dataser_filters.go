package main

import "fmt"

func buildDatasetFilter(lastSync *LastSuccessfulDatasetSync) string {
	if lastSync == nil {
		return ""
	}

	var parts []string

	if lastSync.MaxCreatedAt != nil {
		parts = append(parts,
			fmt.Sprintf(
				"__system/createdAt gt %s",
				formatODataDateTime(*lastSync.MaxCreatedAt),
			),
		)
	}

	if lastSync.MaxUpdatedAt != nil {
		parts = append(parts,
			fmt.Sprintf(
				"__system/updatedAt gt %s",
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
}
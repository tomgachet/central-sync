package main

func extractFailedSubmissionUUIDs(items []FailedSubmissionRef) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range items {
		if item.SubmissionUUID == "" || seen[item.SubmissionUUID] {
			continue
		}
		seen[item.SubmissionUUID] = true
		result = append(result, item.SubmissionUUID)
	}

	return result
}

func mergeSubmissionRows(
	filteredRows []map[string]interface{},
	failedRows []map[string]interface{},
) []map[string]interface{} {
	rowByID := make(map[string]map[string]interface{})

	for _, row := range filteredRows {
		if rowUUID, err := extractSubmissionRowUUID(row); err == nil && rowUUID != "" {
			rowByID[rowUUID] = row
		}
	}

	for _, row := range failedRows {
		if rowUUID, err := extractSubmissionRowUUID(row); err == nil && rowUUID != "" {
			rowByID[rowUUID] = row
		}
	}

	var merged []map[string]interface{}
	for _, row := range rowByID {
		merged = append(merged, row)
	}

	return merged
}
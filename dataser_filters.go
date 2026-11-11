package main

import "fmt"

func buildDatasetFilter(lastSync *LastSuccessfulDatasetSync) string {
	// A missing creation watermark requires a full read: new entities may
	// have no updatedAt timestamp yet.
	if lastSync == nil || lastSync.MaxCreatedAt == nil {
		return ""
	}

	created := fmt.Sprintf("__system/createdAt gt %s", formatODataDateTime(*lastSync.MaxCreatedAt))
	updated := "__system/updatedAt ne null"
	if lastSync.MaxUpdatedAt != nil {
		updated = fmt.Sprintf("__system/updatedAt gt %s", formatODataDateTime(*lastSync.MaxUpdatedAt))
	}
	// Until the first update watermark exists, include all updated entities,
	// including those created before the creation watermark.
	return fmt.Sprintf("(%s) or (%s)", created, updated)
}

func buildDeletedDatasetFilter(lastSync *LastSuccessfulDatasetSync) string {
	if lastSync == nil || lastSync.MaxDeletedAt == nil {
		return "__system/deletedAt ne null"
	}

	return fmt.Sprintf(
		"(__system/deletedAt ne null) and (__system/deletedAt gt %s)",
		formatODataDateTime(*lastSync.MaxDeletedAt),
	)
}

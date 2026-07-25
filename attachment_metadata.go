package main

import (
	"database/sql"
	"fmt"
	"time"
)

type sqlExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

type SubmissionAttachmentMetadata struct {
	RunID          int64
	ProjectID      int
	FormXMLID      string
	SubmissionUUID string
	Filename       string
	StorageBackend string
	StoragePath    string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
	ETag           string
	SyncedAt       time.Time
}

func upsertSubmissionAttachmentMetadata(db sqlExecutor, metadata SubmissionAttachmentMetadata) error {
	query := fmt.Sprintf(`
		INSERT INTO %s.submission_attachments (
			project_id,
			form_xml_id,
			submission_uuid,
			filename,
			storage_backend,
			storage_path,
			content_type,
			size_bytes,
			checksum_sha256,
			etag,
			last_run_id,
			synced_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (project_id, form_xml_id, submission_uuid, filename)
		DO UPDATE SET
			storage_backend = EXCLUDED.storage_backend,
			storage_path = EXCLUDED.storage_path,
			content_type = EXCLUDED.content_type,
			size_bytes = EXCLUDED.size_bytes,
			checksum_sha256 = EXCLUDED.checksum_sha256,
			etag = EXCLUDED.etag,
			last_run_id = EXCLUDED.last_run_id,
			synced_at = EXCLUDED.synced_at
	`, quoteIdentifier(syncMetadataSchema))

	syncedAt := metadata.SyncedAt
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}

	_, err := db.Exec(
		query,
		metadata.ProjectID,
		metadata.FormXMLID,
		metadata.SubmissionUUID,
		metadata.Filename,
		metadata.StorageBackend,
		metadata.StoragePath,
		metadata.ContentType,
		metadata.SizeBytes,
		metadata.ChecksumSHA256,
		metadata.ETag,
		metadata.RunID,
		syncedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert metadata for attachment %q on submission %s: %w", metadata.Filename, metadata.SubmissionUUID, err)
	}

	return nil
}

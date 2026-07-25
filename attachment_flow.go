package main

import (
	"database/sql"
	"fmt"
)

func configuredAttachmentStorage(
	projects []ProjectMapping,
	config AttachmentStorageConfig,
) (AttachmentStorage, error) {
	if !hasEnabledAttachmentSync(projects) {
		return nil, nil
	}

	switch config.Backend {
	case StorageBackendLocal:
		return newLocalAttachmentStorage(config.LocalDirectory)
	default:
		return nil, fmt.Errorf("unsupported attachment storage backend %q", config.Backend)
	}
}

func hasEnabledAttachmentSync(projects []ProjectMapping) bool {
	for _, project := range projects {
		for _, form := range project.Forms {
			if form.Sync && shouldSyncAttachments(form) {
				return true
			}
		}
	}
	return false
}

func syncAndPersistSubmissionAttachments(
	db sqlExecutor,
	client *CentralClient,
	storage AttachmentStorage,
	runID int64,
	projectID int,
	formXMLID string,
	submissionUUID string,
) (*AttachmentSyncResult, error) {
	result, err := syncSubmissionAttachments(client, storage, projectID, formXMLID, submissionUUID)
	if err != nil {
		return result, err
	}

	for _, attachment := range result.Stored {
		err := upsertSubmissionAttachmentMetadata(db, SubmissionAttachmentMetadata{
			RunID:          runID,
			ProjectID:      projectID,
			FormXMLID:      formXMLID,
			SubmissionUUID: trimUUIDPrefix(submissionUUID),
			Filename:       attachment.Name,
			StorageBackend: StorageBackendLocal,
			StoragePath:    attachment.RelativePath,
			ContentType:    attachment.ContentType,
			SizeBytes:      attachment.SizeBytes,
			ETag:           attachment.ETag,
		})
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

func requireSubmissionAttachmentMetadataTable(db *sql.DB) error {
	var tableName sql.NullString
	err := db.QueryRow(`SELECT to_regclass('central_metadata.submission_attachments')`).Scan(&tableName)
	if err != nil {
		return fmt.Errorf("failed to check submission attachment metadata table: %w", err)
	}
	if !tableName.Valid {
		return fmt.Errorf("required table %q does not exist; run the v0.4.0 database migration", "central_metadata.submission_attachments")
	}
	return nil
}

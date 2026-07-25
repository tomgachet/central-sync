package main

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUpsertSubmissionAttachmentMetadata(t *testing.T) {
	executor := &recordingSQLExecutor{}
	syncedAt := time.Date(2026, time.July, 25, 12, 30, 0, 0, time.UTC)
	metadata := SubmissionAttachmentMetadata{
		RunID:          42,
		ProjectID:      7,
		FormXMLID:      "site_visit",
		SubmissionUUID: "123e4567-e89b-12d3-a456-426614174000",
		Filename:       "photo.jpg",
		StorageBackend: StorageBackendLocal,
		StoragePath:    "project-7/form-site_visit/submission-123/photo.jpg",
		ContentType:    "image/jpeg",
		SizeBytes:      1234,
		ChecksumSHA256: strings.Repeat("a", 64),
		ETag:           `"photo-v1"`,
		SyncedAt:       syncedAt,
	}

	if err := upsertSubmissionAttachmentMetadata(executor, metadata); err != nil {
		t.Fatalf("upsertSubmissionAttachmentMetadata returned error: %v", err)
	}

	if !strings.Contains(executor.query, `INSERT INTO "central_metadata".submission_attachments`) {
		t.Fatalf("unexpected query: %s", executor.query)
	}
	if !strings.Contains(executor.query, "ON CONFLICT (project_id, form_xml_id, submission_uuid, filename)") {
		t.Fatalf("query does not contain attachment conflict key: %s", executor.query)
	}
	if len(executor.args) != 12 {
		t.Fatalf("expected 12 arguments, got %d", len(executor.args))
	}
	if executor.args[0] != 7 || executor.args[3] != "photo.jpg" || executor.args[10] != int64(42) || executor.args[11] != syncedAt {
		t.Fatalf("unexpected arguments: %#v", executor.args)
	}
}

func TestUpsertSubmissionAttachmentMetadataUsesCurrentTimeByDefault(t *testing.T) {
	executor := &recordingSQLExecutor{}
	before := time.Now().UTC()

	err := upsertSubmissionAttachmentMetadata(executor, SubmissionAttachmentMetadata{
		RunID:          42,
		ProjectID:      7,
		FormXMLID:      "site_visit",
		SubmissionUUID: "123e4567-e89b-12d3-a456-426614174000",
		Filename:       "photo.jpg",
		StorageBackend: StorageBackendLocal,
		StoragePath:    "stored/photo.jpg",
	})
	if err != nil {
		t.Fatalf("upsertSubmissionAttachmentMetadata returned error: %v", err)
	}

	syncedAt, ok := executor.args[11].(time.Time)
	if !ok || syncedAt.Before(before) || syncedAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected default sync time: %#v", executor.args[11])
	}
}

func TestUpsertSubmissionAttachmentMetadataReturnsDatabaseError(t *testing.T) {
	executor := &recordingSQLExecutor{err: errors.New("database unavailable")}
	err := upsertSubmissionAttachmentMetadata(executor, SubmissionAttachmentMetadata{
		SubmissionUUID: "123e4567-e89b-12d3-a456-426614174000",
		Filename:       "photo.jpg",
	})
	if err == nil || !strings.Contains(err.Error(), "database unavailable") || !strings.Contains(err.Error(), "photo.jpg") {
		t.Fatalf("expected contextual database error, got %v", err)
	}
}

func TestMarkSubmissionAttachmentMissing(t *testing.T) {
	executor := &recordingSQLExecutor{}
	before := time.Now().UTC()
	err := markSubmissionAttachmentMissing(
		executor,
		42,
		7,
		"site_visit",
		"123e4567-e89b-12d3-a456-426614174000",
		"photo.jpg",
	)
	if err != nil {
		t.Fatalf("markSubmissionAttachmentMissing returned error: %v", err)
	}
	if !strings.Contains(executor.query, "central_exists = FALSE") || !strings.Contains(executor.query, "missing_at = $5") {
		t.Fatalf("unexpected missing attachment query: %s", executor.query)
	}
	if len(executor.args) != 6 || executor.args[0] != 7 || executor.args[3] != "photo.jpg" || executor.args[5] != int64(42) {
		t.Fatalf("unexpected missing attachment arguments: %#v", executor.args)
	}
	missingAt, ok := executor.args[4].(time.Time)
	if !ok || missingAt.Before(before) || missingAt.After(time.Now().UTC()) {
		t.Fatalf("unexpected missing timestamp: %#v", executor.args[4])
	}
}

type recordingSQLExecutor struct {
	query string
	args  []interface{}
	err   error
}

func (e *recordingSQLExecutor) Exec(query string, args ...interface{}) (sql.Result, error) {
	e.query = query
	e.args = args
	if e.err != nil {
		return nil, e.err
	}
	return staticSQLResult(1), nil
}

type staticSQLResult int64

func (r staticSQLResult) LastInsertId() (int64, error) {
	return 0, driver.ErrSkip
}

func (r staticSQLResult) RowsAffected() (int64, error) {
	return int64(r), nil
}

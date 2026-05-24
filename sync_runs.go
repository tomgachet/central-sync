package main

import (
	"fmt"
	"time"
)

const syncMetadataSchema = "central_metadata"

type SyncRunStartParams struct {
	ProjectID    int
	FormXMLID    *string
	ObjectType   string
	ObjectName   string
	SQLTableName string
	SyncMode     string
}

type SyncRunFinishParams struct {
	RunID        int64
	SyncStatus   string
	RowsFetched  int
	RowsInserted int
	RowsUpdated  int
	RowsSkipped  int
	ErrorMessage *string
}

type SyncStats struct {
	RowsFetched           int
	RowsInserted          int
	RowsUpdated           int
	RowsSkipped           int
	SyncOutSubmissionDate *time.Time
	SyncOutUpdatedAt      *time.Time
}

func startSyncRun(db DBExecutor, params SyncRunStartParams) (int64, error) {
	query := fmt.Sprintf(`
		INSERT INTO %s.sync_runs (
			project_id,
			form_xml_id,
			object_type,
			object_name,
			sql_table_name,
			sync_mode,
			started_at,
			sync_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING run_id
	`, quoteIdentifier(syncMetadataSchema))

	var runID int64
	err := db.QueryRow(
		query,
		params.ProjectID,
		params.FormXMLID,
		params.ObjectType,
		params.ObjectName,
		params.SQLTableName,
		params.SyncMode,
		time.Now().UTC(),
		"running",
	).Scan(&runID)
	if err != nil {
		return 0, fmt.Errorf("failed to start sync run: %w", err)
	}

	return runID, nil
}

func finishSyncRun(db DBExecutor, params SyncRunFinishParams) error {
	query := fmt.Sprintf(`
		UPDATE %s.sync_runs
		SET
			finished_at = $2,
			sync_status = $3,
			rows_fetched = $4,
			rows_inserted = $5,
			rows_updated = $6,
			rows_skipped = $7,
			error_message = $8
		WHERE run_id = $1
	`, quoteIdentifier(syncMetadataSchema))

	_, err := db.Exec(
		query,
		params.RunID,
		time.Now().UTC(),
		params.SyncStatus,
		params.RowsFetched,
		params.RowsInserted,
		params.RowsUpdated,
		params.RowsSkipped,
		params.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to finish sync run %d: %w", params.RunID, err)
	}

	return nil
}
package main

import (
	"database/sql"
	"fmt"
	"time"
)

const syncMetadataSchema = "central_metadata"

type SyncRun struct {
	RunID                 int64
	ProjectID             int
	FormXMLID             *string
	ObjectType            string
	ObjectName            string
	SQLTableName          string
	SyncMode              string
	StartedAt             time.Time
	FinishedAt            *time.Time
	SyncStatus            string
	SyncInSubmissionDate  *time.Time
	SyncInUpdatedAt       *time.Time
	SyncOutSubmissionDate *time.Time
	SyncOutUpdatedAt      *time.Time
	RowsFetched           int
	RowsInserted          int
	RowsUpdated           int
	RowsSkipped           int
	ErrorMessage          *string
}

type SyncRunStartParams struct {
	ProjectID            int
	FormXMLID            *string
	ObjectType           string
	ObjectName           string
	SQLTableName         string
	SyncMode             string
	SyncInSubmissionDate *time.Time
	SyncInUpdatedAt      *time.Time
}

type SyncRunFinishParams struct {
	RunID                 int64
	SyncStatus            string
	SyncOutSubmissionDate *time.Time
	SyncOutUpdatedAt      *time.Time
	RowsFetched           int
	RowsInserted          int
	RowsUpdated           int
	RowsSkipped           int
	ErrorMessage          *string
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
			sync_status,
			sync_in_submission_date,
			sync_in_updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
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
		params.SyncInSubmissionDate,
		params.SyncInUpdatedAt,
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
			sync_out_submission_date = $4,
			sync_out_updated_at = $5,
			rows_fetched = $6,
			rows_inserted = $7,
			rows_updated = $8,
			rows_skipped = $9,
			error_message = $10
		WHERE run_id = $1
	`, quoteIdentifier(syncMetadataSchema))

	_, err := db.Exec(
		query,
		params.RunID,
		time.Now().UTC(),
		params.SyncStatus,
		params.SyncOutSubmissionDate,
		params.SyncOutUpdatedAt,
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

func getLastSuccessfulSyncRun(
	db DBExecutor,
	projectID int,
	formXMLID *string,
	objectType string,
	objectName string,
) (*SyncRun, error) {
	query := fmt.Sprintf(`
		SELECT
			run_id,
			project_id,
			form_xml_id,
			object_type,
			object_name,
			sql_table_name,
			sync_mode,
			started_at,
			finished_at,
			sync_status,
			sync_in_submission_date,
			sync_in_updated_at,
			sync_out_submission_date,
			sync_out_updated_at,
			rows_fetched,
			rows_inserted,
			rows_updated,
			rows_skipped,
			error_message
		FROM %s.last_successful_sync_runs
		WHERE project_id = $1
		  AND object_type = $2
		  AND object_name = $3
		  AND (
			($4 IS NULL AND form_xml_id IS NULL)
			OR form_xml_id = $4
		  )
		LIMIT 1
	`, quoteIdentifier(syncMetadataSchema))

	var run SyncRun

	var formXMLIDNull sql.NullString
	var finishedAt sql.NullTime
	var syncInSubmissionDate sql.NullTime
	var syncInUpdatedAt sql.NullTime
	var syncOutSubmissionDate sql.NullTime
	var syncOutUpdatedAt sql.NullTime
	var errorMessage sql.NullString

	err := db.QueryRow(
		query,
		projectID,
		objectType,
		objectName,
		formXMLID,
	).Scan(
		&run.RunID,
		&run.ProjectID,
		&formXMLIDNull,
		&run.ObjectType,
		&run.ObjectName,
		&run.SQLTableName,
		&run.SyncMode,
		&run.StartedAt,
		&finishedAt,
		&run.SyncStatus,
		&syncInSubmissionDate,
		&syncInUpdatedAt,
		&syncOutSubmissionDate,
		&syncOutUpdatedAt,
		&run.RowsFetched,
		&run.RowsInserted,
		&run.RowsUpdated,
		&run.RowsSkipped,
		&errorMessage,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read last successful sync run: %w", err)
	}

	if formXMLIDNull.Valid {
		run.FormXMLID = &formXMLIDNull.String
	}
	if finishedAt.Valid {
		run.FinishedAt = &finishedAt.Time
	}
	if syncInSubmissionDate.Valid {
		run.SyncInSubmissionDate = &syncInSubmissionDate.Time
	}
	if syncInUpdatedAt.Valid {
		run.SyncInUpdatedAt = &syncInUpdatedAt.Time
	}
	if syncOutSubmissionDate.Valid {
		run.SyncOutSubmissionDate = &syncOutSubmissionDate.Time
	}
	if syncOutUpdatedAt.Valid {
		run.SyncOutUpdatedAt = &syncOutUpdatedAt.Time
	}
	if errorMessage.Valid {
		run.ErrorMessage = &errorMessage.String
	}

	return &run, nil
}
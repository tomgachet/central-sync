package main

import (
	"database/sql"
	"fmt"
	"time"
)

type SyncRunDetailInsertParams struct {
	RunID                 int64
	ProjectID             int
	FormXMLID             *string
	ObjectType            string
	ObjectName            string
	SQLTableName          string
	SubmissionUUID        *string
	EntityUUID            *string
	CentralCreatedAt      *time.Time
	CentralSubmissionDate *time.Time
	CentralUpdatedAt      *time.Time
	CentralDeletedAt      *time.Time
	SyncAction            string
	SyncStatus            string
	RowsFetched           int
	RowsInserted          int
	RowsUpdated           int
	RowsDeleted           int
	RowsSkipped           int
	RowsFailed            int
	ErrorMessage          *string
}

type LastSuccessfulSubmissionSync struct {
	ProjectID         int
	FormXMLID         string
	ObjectName        string
	MaxSubmissionDate *time.Time
	MaxUpdatedAt      *time.Time
}

type LastSuccessfulDatasetSync struct {
	ProjectID    int
	ObjectName   string
	MaxCreatedAt *time.Time
	MaxUpdatedAt *time.Time
	MaxDeletedAt *time.Time
}

type FailedSubmissionRef struct {
	ProjectID             int
	FormXMLID             string
	SubmissionUUID        string
	CentralSubmissionDate *time.Time
	CentralUpdatedAt      *time.Time
	ErrorMessage          *string
}

func insertSyncRunDetail(db DBExecutor, params SyncRunDetailInsertParams) error {
	query := fmt.Sprintf(`
		INSERT INTO %s.sync_runs_detail (
			run_id,
			project_id,
			form_xml_id,
			object_type,
			object_name,
			sql_table_name,
			submission_uuid,
			entity_uuid,
			central_created_at,
			central_submission_date,
			central_updated_at,
			central_deleted_at,
			sync_action,
			sync_status,
			rows_fetched,
			rows_inserted,
			rows_updated,
			rows_deleted,
			rows_skipped,
			rows_failed,
			error_message,
			processed_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
		)
	`, quoteIdentifier(syncMetadataSchema))

	_, err := db.Exec(
		query,
		params.RunID,
		params.ProjectID,
		params.FormXMLID,
		params.ObjectType,
		params.ObjectName,
		params.SQLTableName,
		params.SubmissionUUID,
		params.EntityUUID,
		params.CentralCreatedAt,
		params.CentralSubmissionDate,
		params.CentralUpdatedAt,
		params.CentralDeletedAt,
		params.SyncAction,
		params.SyncStatus,
		params.RowsFetched,
		params.RowsInserted,
		params.RowsUpdated,
		params.RowsDeleted,
		params.RowsSkipped,
		params.RowsFailed,
		params.ErrorMessage,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert sync run detail: %w", err)
	}

	return nil
}

func getLastSuccessfulSubmissionSync(
	db DBExecutor,
	projectID int,
	formXMLID string,
) (*LastSuccessfulSubmissionSync, error) {
	query := fmt.Sprintf(`
		SELECT
			project_id,
			form_xml_id,
			object_name,
			max_submission_date,
			max_updated_at
		FROM %s.last_successful_submissions_sync
		WHERE project_id = $1
		  AND form_xml_id = $2
		  AND object_name = 'Submissions'
		LIMIT 1
	`, quoteIdentifier(syncMetadataSchema))

	var result LastSuccessfulSubmissionSync
	var maxSubmissionDate sql.NullTime
	var maxUpdatedAt sql.NullTime

	err := db.QueryRow(query, projectID, formXMLID).Scan(
		&result.ProjectID,
		&result.FormXMLID,
		&result.ObjectName,
		&maxSubmissionDate,
		&maxUpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read last successful submissions sync: %w", err)
	}

	if maxSubmissionDate.Valid {
		result.MaxSubmissionDate = &maxSubmissionDate.Time
	}
	if maxUpdatedAt.Valid {
		result.MaxUpdatedAt = &maxUpdatedAt.Time
	}

	return &result, nil
}

func getLastSuccessfulDatasetSync(
	db DBExecutor,
	projectID int,
	objectName string,
) (*LastSuccessfulDatasetSync, error) {
	query := fmt.Sprintf(`
		SELECT
			project_id,
			object_name,
			max_created_at,
			max_updated_at,
			max_deleted_at
		FROM %s.last_successful_datasets_sync
		WHERE project_id = $1
		  AND object_name = $2
		LIMIT 1
	`, quoteIdentifier(syncMetadataSchema))

	var result LastSuccessfulDatasetSync
	var maxCreatedAt sql.NullTime
	var maxUpdatedAt sql.NullTime
	var maxDeletedAt sql.NullTime

	err := db.QueryRow(query, projectID, objectName).Scan(
		&result.ProjectID,
		&result.ObjectName,
		&maxCreatedAt,
		&maxUpdatedAt,
		&maxDeletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read last successful datasets sync: %w", err)
	}

	if maxCreatedAt.Valid {
		result.MaxCreatedAt = &maxCreatedAt.Time
	}
	if maxUpdatedAt.Valid {
		result.MaxUpdatedAt = &maxUpdatedAt.Time
	}
	if maxDeletedAt.Valid {
		result.MaxDeletedAt = &maxDeletedAt.Time
	}

	return &result, nil
}

func getLastFailedSubmissions(
	db *sql.DB,
	projectID int,
	formXMLID string,
) ([]FailedSubmissionRef, error) {
	query := fmt.Sprintf(`
		SELECT
			project_id,
			form_xml_id,
			submission_uuid,
			central_submission_date,
			central_updated_at,
			error_message
		FROM %s.last_failed_submissions
		WHERE project_id = $1
		  AND form_xml_id = $2
		ORDER BY central_submission_date NULLS LAST, submission_uuid
	`, quoteIdentifier(syncMetadataSchema))

	rows, err := db.Query(query, projectID, formXMLID)
	if err != nil {
		return nil, fmt.Errorf("failed to read last failed submissions: %w", err)
	}
	defer rows.Close()

	var results []FailedSubmissionRef

	for rows.Next() {
		var item FailedSubmissionRef
		var submissionDate sql.NullTime
		var updatedAt sql.NullTime
		var errorMessage sql.NullString

		err := rows.Scan(
			&item.ProjectID,
			&item.FormXMLID,
			&item.SubmissionUUID,
			&submissionDate,
			&updatedAt,
			&errorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan failed submission row: %w", err)
		}

		if submissionDate.Valid {
			item.CentralSubmissionDate = &submissionDate.Time
		}
		if updatedAt.Valid {
			item.CentralUpdatedAt = &updatedAt.Time
		}
		if errorMessage.Valid {
			item.ErrorMessage = &errorMessage.String
		}

		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed while iterating failed submissions: %w", err)
	}

	return results, nil
}
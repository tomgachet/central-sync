package main

import (
	"database/sql"
	"fmt"
	"time"
)

type PendingSubmissionDetail struct {
	FormTable          FormTable
	SubmissionUUID     *string
	SubmissionDate     *time.Time
	UpdatedAt          *time.Time
	SyncAction         string
	RowsInserted       int
	RowsUpdated        int
	RowsSkipped        int
}

func syncSubmissionBatch(
	db *sql.DB,
	runID int64,
	projectID int,
	formXMLID string,
	syncMode string,
	batch *SubmissionBatch,
) (*SyncStats, error) {
	stats := &SyncStats{
		RowsFetched: len(batch.Rows),
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction for submission %s: %w", batch.RootSubmissionUUID, err)
	}

	var pendingDetails []PendingSubmissionDetail

	for _, rowRef := range batch.Rows {
		action, submissionUUID, submissionDate, updatedAt, err := upsertFormTableRow(
			tx,
			rowRef.FormTable,
			rowRef.TableSchema,
			syncMode,
			rowRef.Row,
			rowRef.Shape,
		)
		if err != nil {
			_ = tx.Rollback()

			errorMessage := err.Error()
			rootSubmissionUUID := batch.RootSubmissionUUID
			failedSubmissionUUID := &rootSubmissionUUID

			_ = insertSyncRunDetail(db, SyncRunDetailInsertParams{
				RunID:                 runID,
				ProjectID:             projectID,
				FormXMLID:             &formXMLID,
				ObjectType:            "form_submission",
				ObjectName:            rowRef.FormTable.ODataName,
				SQLTableName:          rowRef.FormTable.SQLName,
				SubmissionUUID:        failedSubmissionUUID,
				CentralSubmissionDate: submissionDate,
				CentralUpdatedAt:      updatedAt,
				SyncAction:            "failed",
				SyncStatus:            "failed",
				RowsFetched:           1,
				RowsFailed:            1,
				ErrorMessage:          &errorMessage,
			})

			return stats, fmt.Errorf(
				"failed to sync submission %s on table %s: %w",
				batch.RootSubmissionUUID,
				rowRef.FormTable.ODataName,
				err,
			)
		}

		switch action {
		case "inserted":
			stats.RowsInserted++
		case "updated":
			stats.RowsUpdated++
		case "skipped":
			stats.RowsSkipped++
		}

		if rowRef.FormTable.IsRoot {
			stats.SyncOutSubmissionDate = maxTimePtr(stats.SyncOutSubmissionDate, submissionDate)
			stats.SyncOutUpdatedAt = maxTimePtr(stats.SyncOutUpdatedAt, updatedAt)
		}

		pendingDetails = append(pendingDetails, PendingSubmissionDetail{
			FormTable:      rowRef.FormTable,
			SubmissionUUID: submissionUUID,
			SubmissionDate: submissionDate,
			UpdatedAt:      updatedAt,
			SyncAction:     action,
			RowsInserted:   boolToCount(action == "inserted"),
			RowsUpdated:    boolToCount(action == "updated"),
			RowsSkipped:    boolToCount(action == "skipped"),
		})
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("failed to commit submission %s: %w", batch.RootSubmissionUUID, err)
	}

	for _, detail := range pendingDetails {
		err := insertSyncRunDetail(db, SyncRunDetailInsertParams{
			RunID:                 runID,
			ProjectID:             projectID,
			FormXMLID:             &formXMLID,
			ObjectType:            "form_submission",
			ObjectName:            detail.FormTable.ODataName,
			SQLTableName:          detail.FormTable.SQLName,
			SubmissionUUID:        detail.SubmissionUUID,
			CentralSubmissionDate: detail.SubmissionDate,
			CentralUpdatedAt:      detail.UpdatedAt,
			SyncAction:            detail.SyncAction,
			SyncStatus:            "success",
			RowsFetched:           1,
			RowsInserted:          detail.RowsInserted,
			RowsUpdated:           detail.RowsUpdated,
			RowsSkipped:           detail.RowsSkipped,
		})
		if err != nil {
			return stats, err
		}
	}

	return stats, nil
}
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
)

const appUserSchema = "central_users"

const (
	AppUserActionInserted      = "inserted"
	AppUserActionUpdated       = "updated"
	AppUserActionSkipped       = "skipped"
	AppUserActionRestored      = "restored"
	AppUserActionMarkedMissing = "marked_missing"
)

type AppUserReconcileResult struct {
	AppUserID        int64
	DisplayName      string
	CentralCreatedAt *time.Time
	CentralUpdatedAt *time.Time
	CentralDeletedAt *time.Time
	Action           string
}

func reconcileProjectAppUsers(
	db *sql.DB,
	runID int64,
	projectID int,
	appUsers []CentralAppUser,
) ([]AppUserReconcileResult, error) {
	if db == nil {
		return nil, fmt.Errorf("App User database is nil")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin App User reconciliation: %w", err)
	}
	defer tx.Rollback()

	syncedAt := time.Now().UTC()
	results := make([]AppUserReconcileResult, 0, len(appUsers))
	snapshotIDs := make([]int64, 0, len(appUsers))
	seenIDs := make(map[int64]struct{}, len(appUsers))

	for _, appUser := range appUsers {
		if appUser.ID <= 0 {
			return nil, fmt.Errorf("App User ID must be greater than 0")
		}
		if _, exists := seenIDs[appUser.ID]; exists {
			return nil, fmt.Errorf("duplicate App User ID %d in Central snapshot", appUser.ID)
		}
		seenIDs[appUser.ID] = struct{}{}
		snapshotIDs = append(snapshotIDs, appUser.ID)

		action, err := reconcileAppUser(tx, runID, projectID, appUser, syncedAt)
		if err != nil {
			return nil, err
		}
		results = append(results, AppUserReconcileResult{
			AppUserID:        appUser.ID,
			DisplayName:      appUser.DisplayName,
			CentralCreatedAt: appUser.CreatedAt,
			CentralUpdatedAt: appUser.UpdatedAt,
			CentralDeletedAt: appUser.DeletedAt,
			Action:           action,
		})
	}

	missingIDs, err := markAbsentAppUsersMissing(tx, runID, projectID, snapshotIDs, syncedAt)
	if err != nil {
		return nil, err
	}
	for _, missing := range missingIDs {
		results = append(results, AppUserReconcileResult{
			AppUserID:   missing.AppUserID,
			DisplayName: missing.DisplayName,
			Action:      AppUserActionMarkedMissing,
		})
	}

	for _, result := range results {
		if err := insertAppUserSyncRunDetail(tx, runID, projectID, result); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit App User reconciliation: %w", err)
	}
	return results, nil
}

func reconcileAppUser(
	tx *sql.Tx,
	runID int64,
	projectID int,
	appUser CentralAppUser,
	syncedAt time.Time,
) (string, error) {
	var createdBy interface{}
	if appUser.CreatedBy != nil {
		encodedCreatedBy, err := json.Marshal(appUser.CreatedBy)
		if err != nil {
			return "", fmt.Errorf("failed to encode creator for App User %d: %w", appUser.ID, err)
		}
		createdBy = encodedCreatedBy
	}
	properties, err := json.Marshal(appUser.Properties)
	if err != nil {
		return "", fmt.Errorf("failed to encode properties for App User %d: %w", appUser.ID, err)
	}
	if appUser.Properties == nil {
		properties = []byte("{}")
	}

	query := fmt.Sprintf(`
		WITH existing AS (
			SELECT
				display_name,
				actor_type,
				central_created_at,
				central_updated_at,
				central_deleted_at,
				last_used_at,
				created_by,
				properties,
				revoked,
				missing_from_central
			FROM %s.app_users
			WHERE project_id = $1
			  AND app_user_id = $2
			FOR UPDATE
		),
		upserted AS (
			INSERT INTO %s.app_users (
				project_id,
				app_user_id,
				display_name,
				actor_type,
				central_created_at,
				central_updated_at,
				central_deleted_at,
				last_used_at,
				created_by,
				properties,
				revoked,
				missing_from_central,
				missing_since,
				first_synced_at,
				last_synced_at,
				last_run_id
			)
			VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb,
				$11, FALSE, NULL, $12, $12, $13
			)
			ON CONFLICT (project_id, app_user_id)
			DO UPDATE SET
				display_name = EXCLUDED.display_name,
				actor_type = EXCLUDED.actor_type,
				central_created_at = EXCLUDED.central_created_at,
				central_updated_at = EXCLUDED.central_updated_at,
				central_deleted_at = EXCLUDED.central_deleted_at,
				last_used_at = EXCLUDED.last_used_at,
				created_by = EXCLUDED.created_by,
				properties = EXCLUDED.properties,
				revoked = EXCLUDED.revoked,
				missing_from_central = FALSE,
				missing_since = NULL,
				last_synced_at = EXCLUDED.last_synced_at,
				last_run_id = EXCLUDED.last_run_id
			RETURNING app_user_id
		)
		SELECT CASE
			WHEN NOT EXISTS (SELECT 1 FROM existing) THEN $14
			WHEN (SELECT missing_from_central FROM existing) THEN $15
			WHEN EXISTS (
				SELECT 1
				FROM existing
				WHERE display_name IS DISTINCT FROM $3
				   OR actor_type IS DISTINCT FROM $4
				   OR central_created_at IS DISTINCT FROM $5
				   OR central_updated_at IS DISTINCT FROM $6
				   OR central_deleted_at IS DISTINCT FROM $7
				   OR last_used_at IS DISTINCT FROM $8
				   OR created_by IS DISTINCT FROM $9::jsonb
				   OR properties IS DISTINCT FROM $10::jsonb
				   OR revoked IS DISTINCT FROM $11
			) THEN $16
			ELSE $17
		END
		FROM upserted
	`, quoteIdentifier(appUserSchema), quoteIdentifier(appUserSchema))

	var action string
	err = tx.QueryRow(
		query,
		projectID,
		appUser.ID,
		appUser.DisplayName,
		appUser.Type,
		appUser.CreatedAt,
		appUser.UpdatedAt,
		appUser.DeletedAt,
		appUser.LastUsed,
		createdBy,
		properties,
		appUser.Revoked,
		syncedAt,
		runID,
		AppUserActionInserted,
		AppUserActionRestored,
		AppUserActionUpdated,
		AppUserActionSkipped,
	).Scan(&action)
	if err != nil {
		return "", fmt.Errorf("failed to reconcile App User %d: %w", appUser.ID, err)
	}
	return action, nil
}

type missingAppUser struct {
	AppUserID   int64
	DisplayName string
}

func markAbsentAppUsersMissing(
	tx *sql.Tx,
	runID int64,
	projectID int,
	snapshotIDs []int64,
	syncedAt time.Time,
) ([]missingAppUser, error) {
	query := fmt.Sprintf(`
		UPDATE %s.app_users
		SET
			missing_from_central = TRUE,
			missing_since = COALESCE(missing_since, $3),
			last_synced_at = $3,
			last_run_id = $4
		WHERE project_id = $1
		  AND NOT (app_user_id = ANY($2))
		  AND missing_from_central = FALSE
		RETURNING app_user_id, display_name
	`, quoteIdentifier(appUserSchema))

	rows, err := tx.Query(query, projectID, pq.Array(snapshotIDs), syncedAt, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to mark missing App Users for project %d: %w", projectID, err)
	}
	defer rows.Close()

	var missingUsers []missingAppUser
	for rows.Next() {
		var missing missingAppUser
		if err := rows.Scan(&missing.AppUserID, &missing.DisplayName); err != nil {
			return nil, fmt.Errorf("failed to read missing App User for project %d: %w", projectID, err)
		}
		missingUsers = append(missingUsers, missing)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate missing App Users for project %d: %w", projectID, err)
	}
	return missingUsers, nil
}

func insertAppUserSyncRunDetail(
	tx *sql.Tx,
	runID int64,
	projectID int,
	result AppUserReconcileResult,
) error {
	rowsInserted := boolToCount(result.Action == AppUserActionInserted)
	rowsUpdated := boolToCount(
		result.Action == AppUserActionUpdated ||
			result.Action == AppUserActionRestored ||
			result.Action == AppUserActionMarkedMissing,
	)
	rowsSkipped := boolToCount(result.Action == AppUserActionSkipped)
	objectName := result.DisplayName
	if objectName == "" {
		objectName = fmt.Sprintf("%d", result.AppUserID)
	}

	err := insertSyncRunDetail(tx, SyncRunDetailInsertParams{
		RunID:            runID,
		ProjectID:        projectID,
		ObjectType:       "app_user",
		ObjectName:       objectName,
		SQLTableName:     "app_users",
		AppUserID:        &result.AppUserID,
		CentralCreatedAt: result.CentralCreatedAt,
		CentralUpdatedAt: result.CentralUpdatedAt,
		CentralDeletedAt: result.CentralDeletedAt,
		SyncAction:       result.Action,
		SyncStatus:       "success",
		RowsFetched:      boolToCount(result.Action != AppUserActionMarkedMissing),
		RowsInserted:     rowsInserted,
		RowsUpdated:      rowsUpdated,
		RowsSkipped:      rowsSkipped,
	})
	if err != nil {
		return fmt.Errorf("failed to record sync detail for App User %d: %w", result.AppUserID, err)
	}
	return nil
}

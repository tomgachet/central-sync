package main

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

func syncAllAppUsers(syncID uuid.UUID, projects []ProjectMapping, client *CentralClient) {
	for _, project := range projects {
		if !project.SyncAppUsers {
			continue
		}
		if err := syncProjectAppUsers(syncID, project, client); err != nil {
			logError(
				"[APP_USER] sync_id=%s project_id=%d sync error: %v",
				syncID,
				project.ProjectID,
				err,
			)
		}
	}
}

func syncProjectAppUsers(
	syncID uuid.UUID,
	project ProjectMapping,
	client *CentralClient,
) error {
	exists, err := projectExists(client, project.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to validate project %d: %w", project.ProjectID, err)
	}
	if !exists {
		logWarn(
			"[APP_USER] skipping project_id=%d project_name=%q: project does not exist in ODK Central",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	db, err := connectProjectDatabase(project.DatabaseName)
	if err != nil {
		return fmt.Errorf("database connection error for project %d: %w", project.ProjectID, err)
	}
	defer db.Close()

	if err := requireSchema(db, appUserSchema); err != nil {
		return fmt.Errorf("App User schema error for project %d: %w", project.ProjectID, err)
	}

	return syncSingleProjectAppUsers(syncID, db, project, client)
}

func syncSingleProjectAppUsers(
	syncID uuid.UUID,
	db *sql.DB,
	project ProjectMapping,
	client *CentralClient,
) error {
	runID, err := startSyncRun(db, SyncRunStartParams{
		SyncID:       syncID,
		ProjectID:    project.ProjectID,
		ObjectType:   "app_user",
		ObjectName:   "app_users",
		SQLTableName: "app_users",
		SyncMode:     SyncModeUpsert,
	})
	if err != nil {
		return fmt.Errorf("failed to start App User sync run: %w", err)
	}

	logInfo(
		"[APP_USER] sync_id=%s project_id=%d run_id=%d started",
		syncID,
		project.ProjectID,
		runID,
	)

	appUsers, err := listProjectAppUsers(client, project.ProjectID)
	if err != nil {
		return finishFailedAppUserRun(db, runID, 0, err)
	}

	results, err := reconcileProjectAppUsers(db, runID, project.ProjectID, appUsers)
	if err != nil {
		return finishFailedAppUserRun(db, runID, len(appUsers), err)
	}

	stats := summarizeAppUserResults(len(appUsers), results)
	if err := finishSyncRun(db, SyncRunFinishParams{
		RunID:        runID,
		SyncStatus:   "success",
		RowsFetched:  stats.RowsFetched,
		RowsInserted: stats.RowsInserted,
		RowsUpdated:  stats.RowsUpdated,
		RowsSkipped:  stats.RowsSkipped,
	}); err != nil {
		return fmt.Errorf("failed to finish App User sync run %d: %w", runID, err)
	}

	logInfo(
		"[APP_USER] sync_id=%s project_id=%d run_id=%d completed fetched=%d inserted=%d updated=%d skipped=%d marked_missing=%d",
		syncID,
		project.ProjectID,
		runID,
		stats.RowsFetched,
		stats.RowsInserted,
		stats.RowsUpdated,
		stats.RowsSkipped,
		countAppUserActions(results, AppUserActionMarkedMissing),
	)
	return nil
}

func finishFailedAppUserRun(db *sql.DB, runID int64, rowsFetched int, syncErr error) error {
	errorMessage := syncErr.Error()
	if err := finishSyncRun(db, SyncRunFinishParams{
		RunID:        runID,
		SyncStatus:   "failed",
		RowsFetched:  rowsFetched,
		RowsFailed:   rowsFetched,
		ErrorMessage: &errorMessage,
	}); err != nil {
		return fmt.Errorf("App User sync failed: %v; failed to finish run %d: %w", syncErr, runID, err)
	}
	return syncErr
}

func summarizeAppUserResults(rowsFetched int, results []AppUserReconcileResult) SyncStats {
	stats := SyncStats{RowsFetched: rowsFetched}
	for _, result := range results {
		switch result.Action {
		case AppUserActionInserted:
			stats.RowsInserted++
		case AppUserActionUpdated, AppUserActionRestored, AppUserActionMarkedMissing:
			stats.RowsUpdated++
		case AppUserActionSkipped:
			stats.RowsSkipped++
		}
	}
	return stats
}

func countAppUserActions(results []AppUserReconcileResult, action string) int {
	count := 0
	for _, result := range results {
		if result.Action == action {
			count++
		}
	}
	return count
}

package main

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func syncAllProjects(outcome *syncOutcome, syncID uuid.UUID, projects []ProjectMapping, client *CentralClient) {
	for _, project := range projects {
		err := syncProjectDatasets(outcome, syncID, project, client)
		if err != nil {
			outcome.record(false, err)
			logError("[PROJECT] sync_id=%s project sync error: %v", syncID, err)
		}
	}
}

func syncProjectDatasets(outcome *syncOutcome, syncID uuid.UUID, project ProjectMapping, client *CentralClient) error {
	datasetsToSync := getDatasetsToSync(project)
	if len(datasetsToSync) == 0 {
		return nil
	}

	exists, err := projectExists(client, project.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to validate project %d: %w", project.ProjectID, err)
	}
	if !exists {
		return fmt.Errorf("project %d (%q) does not exist in ODK Central", project.ProjectID, project.ProjectName)
	}

	logInfo(
		"[PROJECT] processing project_id=%d project_name=%q database=%s",
		project.ProjectID,
		project.ProjectName,
		project.DatabaseName,
	)

	db, err := connectProjectDatabase(project.DatabaseName)
	if err != nil {
		return fmt.Errorf("database connection error for project %d: %w", project.ProjectID, err)
	}
	defer db.Close()

	err = requireSchema(db, datasetSchema)
	if err != nil {
		return fmt.Errorf("schema error for project %d: %w", project.ProjectID, err)
	}

	for _, dataset := range datasetsToSync {
		completed, err := syncSingleDataset(syncID, db, project, dataset, client)
		outcome.record(completed, err)
		if err != nil {
			logError(
				"[DATASET] sync_id=%s project_id=%d dataset=%q table=%s sync error: %v",
				syncID,
				project.ProjectID,
				dataset.Name,
				dataset.TableName,
				err,
			)
		}
	}

	return nil
}

func syncSingleDataset(syncID uuid.UUID, db DBExecutor, project ProjectMapping, dataset DatasetMapping, client *CentralClient) (bool, error) {
	logInfo(
		"[DATASET] sync_id=%s project_id=%d dataset=%q table=%s starting sync",
		syncID,
		project.ProjectID,
		dataset.Name,
		dataset.TableName,
	)

	lastDatasetSync, err := getLastSuccessfulDatasetSync(db, project.ProjectID, dataset.Name)
	if err != nil {
		return false, fmt.Errorf("failed to read last successful dataset sync for dataset %s: %w", dataset.Name, err)
	}

	syncRunID, err := startSyncRun(db, SyncRunStartParams{
		SyncID:       syncID,
		ProjectID:    project.ProjectID,
		FormXMLID:    nil,
		ObjectType:   "dataset",
		ObjectName:   dataset.Name,
		SQLTableName: dataset.TableName,
		SyncMode:     SyncModeUpsert,
	})
	if err != nil {
		return false, fmt.Errorf("failed to start sync run for dataset %s: %w", dataset.Name, err)
	}

	logInfo(
		"[DATASET] sync_id=%s project_id=%d dataset=%q table=%s run_id=%d started",
		syncID,
		project.ProjectID,
		dataset.Name,
		dataset.TableName,
		syncRunID,
	)

	metadata, err := getDatasetMetadata(client, project.ProjectID, dataset.Name)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return false, fmt.Errorf("metadata error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureDatasetTableExists(db, dataset.TableName)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return false, fmt.Errorf("table error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureTechnicalColumnsExist(db, dataset.TableName)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return false, fmt.Errorf("technical column error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureDatasetPropertyColumnsExist(db, dataset.TableName, metadata.Properties)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return false, fmt.Errorf("property column error for dataset %s: %w", dataset.Name, err)
	}

	filter := buildDatasetFilter(lastDatasetSync)
	if filter != "" {
		logInfo(
			"[DATASET] project_id=%d dataset=%q run_id=%d applying filter: %s",
			project.ProjectID,
			dataset.Name,
			syncRunID,
			filter,
		)
	}

	entities, err := getAllDatasetEntities(client, project.ProjectID, dataset.Name, filter)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return false, fmt.Errorf("entities error for dataset %s: %w", dataset.Name, err)
	}

	deletedFilter := buildDeletedDatasetFilter(lastDatasetSync)
	logInfo(
		"[DATASET] project_id=%d dataset=%q run_id=%d applying deleted filter: %s",
		project.ProjectID,
		dataset.Name,
		syncRunID,
		deletedFilter,
	)

	deletedEntities, err := getAllDatasetEntities(client, project.ProjectID, dataset.Name, deletedFilter)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return false, fmt.Errorf("deleted entities error for dataset %s: %w", dataset.Name, err)
	}

	entities = mergeDatasetEntitiesByID(entities, deletedEntities)

	logInfo(
		"[DATASET] project_id=%d dataset=%q run_id=%d fetched_entities=%d deleted_entities=%d merged_entities=%d",
		project.ProjectID,
		dataset.Name,
		syncRunID,
		len(entities)-len(deletedEntities),
		len(deletedEntities),
		len(entities),
	)

	geojsonCollection, err := getDatasetEntitiesGeoJSON(client, project.ProjectID, dataset.Name)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return false, fmt.Errorf("dataset GeoJSON error for dataset %s: %w", dataset.Name, err)
	}

	geometryGeoJSONByEntityID := buildGeometryGeoJSONMap(geojsonCollection)

	stats, syncErr := syncDatasetEntities(
		db,
		syncRunID,
		project.ProjectID,
		dataset.TableName,
		dataset.Name,
		entities,
		metadata.Properties,
		geometryGeoJSONByEntityID,
	)

	completed := syncErr == nil || stats.RowsInserted+stats.RowsUpdated+stats.RowsSkipped > 0

	finalStatus := (syncOutcome{succeeded: completed, failed: syncErr != nil}).status()
	var finalErrorMessage *string

	if syncErr != nil {
		msg := syncErr.Error()
		finalErrorMessage = &msg
	}

	err = finishSyncRun(db, SyncRunFinishParams{
		RunID:        syncRunID,
		SyncStatus:   finalStatus,
		RowsFetched:  stats.RowsFetched,
		RowsInserted: stats.RowsInserted,
		RowsUpdated:  stats.RowsUpdated,
		RowsSkipped:  stats.RowsSkipped,
		RowsFailed:   stats.RowsFailed,
		ErrorMessage: finalErrorMessage,
	})
	if err != nil {
		return completed, errors.Join(syncErr, fmt.Errorf("failed to finalize sync run for dataset %s: %w", dataset.Name, err))
	}

	logInfo(
		"[DATASET] sync_id=%s project_id=%d dataset=%q table=%s run_id=%d status=%s fetched=%d inserted=%d updated=%d skipped=%d failed=%d",
		syncID,
		project.ProjectID,
		dataset.Name,
		dataset.TableName,
		syncRunID,
		finalStatus,
		stats.RowsFetched,
		stats.RowsInserted,
		stats.RowsUpdated,
		stats.RowsSkipped,
		stats.RowsFailed,
	)

	return completed, syncErr
}

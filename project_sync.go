package main

import "fmt"

func syncAllProjects(projects []ProjectMapping, client *CentralClient) {
	for _, project := range projects {
		err := syncProjectDatasets(project, client)
		if err != nil {
			logError("[PROJECT] project sync error: %v", err)
		}
	}
}

func syncProjectDatasets(project ProjectMapping, client *CentralClient) error {
	exists, err := projectExists(client, project.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to validate project %d: %w", project.ProjectID, err)
	}

	if !exists {
		logWarn(
			"[PROJECT] skipping project_id=%d project_name=%q: project does not exist in ODK Central",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	datasetsToSync := getDatasetsToSync(project)

	if len(datasetsToSync) == 0 {
		logWarn(
			"[PROJECT] skipping project_id=%d project_name=%q: no dataset to sync",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
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
		err := syncSingleDataset(db, project, dataset, client)
		if err != nil {
			logError(
				"[DATASET] project_id=%d dataset=%q table=%s sync error: %v",
				project.ProjectID,
				dataset.Name,
				dataset.TableName,
				err,
			)
		}
	}

	return nil
}

func syncSingleDataset(db DBExecutor, project ProjectMapping, dataset DatasetMapping, client *CentralClient) error {
	logInfo(
		"[DATASET] project_id=%d dataset=%q table=%s starting sync",
		project.ProjectID,
		dataset.Name,
		dataset.TableName,
	)

	lastDatasetSync, err := getLastSuccessfulDatasetSync(db, project.ProjectID, dataset.Name)
	if err != nil {
		return fmt.Errorf("failed to read last successful dataset sync for dataset %s: %w", dataset.Name, err)
	}

	syncRunID, err := startSyncRun(db, SyncRunStartParams{
		ProjectID:    project.ProjectID,
		FormXMLID:    nil,
		ObjectType:   "dataset",
		ObjectName:   dataset.Name,
		SQLTableName: dataset.TableName,
		SyncMode:     SyncModeUpsert,
	})
	if err != nil {
		return fmt.Errorf("failed to start sync run for dataset %s: %w", dataset.Name, err)
	}

	logInfo(
		"[DATASET] project_id=%d dataset=%q table=%s run_id=%d started",
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
		return fmt.Errorf("metadata error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureDatasetTableExists(db, dataset.TableName)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return fmt.Errorf("table error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureTechnicalColumnsExist(db, dataset.TableName)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return fmt.Errorf("technical column error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureDatasetPropertyColumnsExist(db, dataset.TableName, metadata.Properties)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return fmt.Errorf("property column error for dataset %s: %w", dataset.Name, err)
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
		return fmt.Errorf("entities error for dataset %s: %w", dataset.Name, err)
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
		return fmt.Errorf("deleted entities error for dataset %s: %w", dataset.Name, err)
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
		return fmt.Errorf("dataset GeoJSON error for dataset %s: %w", dataset.Name, err)
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

	finalStatus := "success"
	var finalErrorMessage *string

	if syncErr != nil {
		finalStatus = "partial_success"
		msg := syncErr.Error()
		finalErrorMessage = &msg
		logWarn(
			"[DATASET] project_id=%d dataset=%q run_id=%d partial success: %v",
			project.ProjectID,
			dataset.Name,
			syncRunID,
			syncErr,
		)
	}

	err = finishSyncRun(db, SyncRunFinishParams{
		RunID:        syncRunID,
		SyncStatus:   finalStatus,
		RowsFetched:  stats.RowsFetched,
		RowsInserted: stats.RowsInserted,
		RowsUpdated:  stats.RowsUpdated,
		RowsSkipped:  stats.RowsSkipped,
		ErrorMessage: finalErrorMessage,
	})
	if err != nil {
		return fmt.Errorf("failed to finalize sync run for dataset %s: %w", dataset.Name, err)
	}

	logInfo(
		"[DATASET] project_id=%d dataset=%q table=%s run_id=%d status=%s fetched=%d inserted=%d updated=%d skipped=%d failed=%d",
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

	return nil
}
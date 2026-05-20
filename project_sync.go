package main

import "fmt"

func syncAllProjects(projects []ProjectMapping, client *CentralClient) {
	for _, project := range projects {
		err := syncProjectDatasets(project, client)
		if err != nil {
			fmt.Println("Project sync error:", err)
		}
	}
}

func syncProjectDatasets(project ProjectMapping, client *CentralClient) error {
	exists, err := projectExists(client, project.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to validate project %d: %w", project.ProjectID, err)
	}

	if !exists {
		fmt.Printf(
			"\nSkipping project %d (%s): project does not exist in ODK Central\n",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	datasetsToSync := getDatasetsToSync(project)

	if len(datasetsToSync) == 0 {
		fmt.Printf(
			"\nSkipping project %d (%s): no dataset to sync\n",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	fmt.Printf(
		"\nProcessing project %d (%s) -> database %s\n",
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
			fmt.Println("Dataset sync error:", err)
		}
	}

	return nil
}

func syncSingleDataset(db DBExecutor, project ProjectMapping, dataset DatasetMapping, client *CentralClient) error {
	fmt.Printf(
		"\nSyncing dataset %s -> table %s\n",
		dataset.Name,
		dataset.TableName,
	)

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

	entities, err := getAllDatasetEntities(client, project.ProjectID, dataset.Name)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return fmt.Errorf("entities error for dataset %s: %w", dataset.Name, err)
	}

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

	stats, err := syncDatasetEntities(db, dataset.TableName, entities, metadata.Properties, geometryGeoJSONByEntityID)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:            syncRunID,
			SyncStatus:       "failed",
			SyncOutUpdatedAt: nil,
			RowsFetched:      len(entities),
			ErrorMessage:     &errorMessage,
		})
		return fmt.Errorf("entity sync error for dataset %s: %w", dataset.Name, err)
	}

	err = finishSyncRun(db, SyncRunFinishParams{
		RunID:            syncRunID,
		SyncStatus:       "success",
		SyncOutUpdatedAt: stats.SyncOutUpdatedAt,
		RowsFetched:      stats.RowsFetched,
		RowsInserted:     stats.RowsInserted,
		RowsUpdated:      stats.RowsUpdated,
		RowsSkipped:      stats.RowsSkipped,
	})
	if err != nil {
		return fmt.Errorf("failed to finalize sync run for dataset %s: %w", dataset.Name, err)
	}

	fmt.Printf(
		"Dataset %s synced successfully: fetched=%d inserted=%d updated=%d skipped=%d\n",
		dataset.Name,
		stats.RowsFetched,
		stats.RowsInserted,
		stats.RowsUpdated,
		stats.RowsSkipped,
	)

	return nil
}
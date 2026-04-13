package main

import (
	"fmt"
)

func syncAllProjects(projects []ProjectMapping, centralURL string, token string) {
	for _, project := range projects {
		err := syncProjectDatasets(project, centralURL, token)
		if err != nil {
			fmt.Println("Project sync error:", err)
		}
	}
}

func syncProjectDatasets(project ProjectMapping, centralURL string, token string) error {
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
		err := syncSingleDataset(db, project, dataset, centralURL, token)
		if err != nil {
			fmt.Println("Dataset sync error:", err)
		}
	}

	return nil
}

func syncSingleDataset(db DBExecutor, project ProjectMapping, dataset DatasetMapping, centralURL string, token string) error {
	fmt.Printf(
		"\nSyncing dataset %s -> table %s\n",
		dataset.Name,
		dataset.TableName,
	)

	metadata, err := getDatasetMetadata(centralURL, token, project.ProjectID, dataset.Name)
	if err != nil {
		return fmt.Errorf("metadata error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureDatasetTableExists(db, dataset.TableName)
	if err != nil {
		return fmt.Errorf("table error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureTechnicalColumnsExist(db, dataset.TableName)
	if err != nil {
		return fmt.Errorf("technical column error for dataset %s: %w", dataset.Name, err)
	}

	err = ensureDatasetPropertyColumnsExist(db, dataset.TableName, metadata.Properties)
	if err != nil {
		return fmt.Errorf("property column error for dataset %s: %w", dataset.Name, err)
	}

	entities, err := getAllDatasetEntities(centralURL, token, project.ProjectID, dataset.Name)
	if err != nil {
		return fmt.Errorf("entities error for dataset %s: %w", dataset.Name, err)
	}

	geojsonCollection, err := getDatasetEntitiesGeoJSON(centralURL, token, project.ProjectID, dataset.Name)
	if err != nil {
		return fmt.Errorf("dataset GeoJSON error for dataset %s: %w", dataset.Name, err)
	}

	geometryGeoJSONByEntityID := buildGeometryGeoJSONMap(geojsonCollection)

	err = syncDatasetEntities(db, dataset.TableName, entities, metadata.Properties, geometryGeoJSONByEntityID)
	if err != nil {
		return fmt.Errorf("entity sync error for dataset %s: %w", dataset.Name, err)
	}

	fmt.Printf(
		"Dataset %s synced successfully: %d entities\n",
		dataset.Name,
		len(entities),
	)

	return nil
}
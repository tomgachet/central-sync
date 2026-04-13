package main

import "fmt"

func main() {
	err := loadEnvFile(".env")
	if err != nil {
		fmt.Println("Environment error:", err)
		return
	}

	config, err := loadProjectConfig("central_config.yaml")
	if err != nil {
		fmt.Println("Configuration error:", err)
		return
	}

	if len(config.Projects) == 0 {
		fmt.Println("No project mapping found")
		return
	}

	centralURL, err := getRequiredEnv("ODK_CENTRAL_URL")
	if err != nil {
		fmt.Println("Environment error:", err)
		return
	}

	token, err := getValidCentralToken()
	if err != nil {
		fmt.Println("Central token error:", err)
		return
	}

	fmt.Println("Starting dataset sync")

	for _, project := range config.Projects {
		datasetsToSync := getDatasetsToSync(project)

		if len(datasetsToSync) == 0 {
			fmt.Printf(
				"\nSkipping project %d (%s): no dataset to sync\n",
				project.ProjectID,
				project.ProjectName,
			)
			continue
		}

		fmt.Printf(
			"\nProcessing project %d (%s) -> database %s\n",
			project.ProjectID,
			project.ProjectName,
			project.DatabaseName,
		)

		db, err := connectProjectDatabase(project.DatabaseName)
		if err != nil {
			fmt.Println("Database connection error:", err)
			continue
		}

		err = requireSchema(db, datasetSchema)
		if err != nil {
			fmt.Println("Schema error:", err)
			db.Close()
			continue
		}

		for _, dataset := range datasetsToSync {
			fmt.Printf(
				"\nSyncing dataset %s -> table %s\n",
				dataset.Name,
				dataset.TableName,
			)

			metadata, err := getDatasetMetadata(centralURL, token, project.ProjectID, dataset.Name)
			if err != nil {
				fmt.Println("Dataset metadata error:", err)
				continue
			}

			err = ensureDatasetTableExists(db, dataset.TableName)
			if err != nil {
				fmt.Println("Table error:", err)
				continue
			}

			err = ensureTechnicalColumnsExist(db, dataset.TableName)
			if err != nil {
				fmt.Println("Technical column error:", err)
				continue
			}

			err = ensureDatasetPropertyColumnsExist(db, dataset.TableName, metadata.Properties)
			if err != nil {
				fmt.Println("Dataset property column error:", err)
				continue
			}

			entities, err := getAllDatasetEntities(centralURL, token, project.ProjectID, dataset.Name)
			if err != nil {
				fmt.Println("Dataset entities error:", err)
				continue
			}

			err = syncDatasetEntities(db, dataset.TableName, entities, metadata.Properties)
			if err != nil {
				fmt.Println("Dataset sync error:", err)
				continue
			}

			fmt.Printf(
				"Dataset %s synced successfully: %d entities\n",
				dataset.Name,
				len(entities),
			)
		}

		db.Close()
	}

	fmt.Println("\nDataset sync finished")
}
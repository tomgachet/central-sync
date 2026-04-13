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

	project := config.Projects[0]
	datasetsToSync := getDatasetsToSync(project)

	if len(datasetsToSync) == 0 {
		fmt.Println("No dataset to sync")
		return
	}

	dataset := datasetsToSync[0]

	fmt.Printf(
		"Preparing dataset table for project %d (%s), dataset %s -> table %s\n",
		project.ProjectID,
		project.ProjectName,
		dataset.Name,
		dataset.TableName,
	)

	db, err := connectProjectDatabase(project.DatabaseName)
	if err != nil {
		fmt.Println("Database connection error:", err)
		return
	}
	defer db.Close()

	err = requireSchema(db, datasetSchema)
	if err != nil {
		fmt.Println("Schema error:", err)
		return
	}

	metadata, err := getDatasetMetadata(centralURL, token, project.ProjectID, dataset.Name)
	if err != nil {
		fmt.Println("Dataset metadata error:", err)
		return
	}

	err = ensureDatasetTableExists(db, dataset.TableName)
	if err != nil {
		fmt.Println("Table error:", err)
		return
	}

	err = ensureDatasetColumnsExist(db, dataset.TableName, metadata.Properties)
	if err != nil {
		fmt.Println("Column error:", err)
		return
	}

	fmt.Printf("Dataset table %s.%s is ready\n", datasetSchema, dataset.TableName)
}
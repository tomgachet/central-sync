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

	client, err := newCentralClient()
	if err != nil {
		fmt.Println("Central client error:", err)
		return
	}

	fmt.Println("Starting dataset sync")
	syncAllProjects(config.Projects, client)
	fmt.Println("\nDataset sync finished")

	fmt.Println("\nStarting form sync")
	syncAllForms(config.Projects, client)
	fmt.Println("\nForm sync finished")
}
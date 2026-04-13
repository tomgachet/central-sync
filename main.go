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

	syncAllProjects(config.Projects, centralURL, token)

	fmt.Println("\nDataset sync finished")
}
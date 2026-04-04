package main

import "fmt"

func main() {
	config, err := loadProjectConfig("central_config.yaml")
	if err != nil {
		fmt.Println("Configuration error:", err)
		return
	}

	if len(config.Projects) == 0 {
		fmt.Println("No project mapping found")
		return
	}

	project := config.Projects[0]

	fmt.Printf("Testing database connection for project %d (%s)\n", project.ProjectID, project.ProjectName)

	db, err := connectProjectDatabase(".env", project.DatabaseName)
	if err != nil {
		fmt.Println("Database connection error:", err)
		return
	}
	defer db.Close()

	fmt.Println("PostgreSQL connection successful")
}
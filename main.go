package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("central-sync %s\n", version)
			return
		}
	}

	if err := initLogger(); err != nil {
		println("failed to initialize logger:", err.Error())
		return
	}
	defer func() {
		if err := closeLogger(); err != nil {
			println("failed to close logger:", err.Error())
		}
	}()

	logInfo("central-sync started (version=%s)", version)

	err := loadEnvFile(".env")
	if err != nil {
		logError("environment error: %v", err)
		return
	}

	config, err := loadProjectConfig("central_config.yaml")
	if err != nil {
		logError("configuration error: %v", err)
		return
	}

	if len(config.Projects) == 0 {
		logWarn("no project mapping found")
		return
	}

	client, err := newCentralClient()
	if err != nil {
		logError("central client error: %v", err)
		return
	}

	logInfo("starting dataset sync")
	syncAllProjects(config.Projects, client)
	logInfo("dataset sync finished")

	logInfo("starting form sync")
	syncAllForms(config.Projects, client)
	logInfo("form sync finished")

	logInfo("central-sync finished (version=%s)", version)
}
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

	syncID, err := newSyncID()
	if err != nil {
		logError("startup error: %v", err)
		return
	}

	logInfo("central-sync started sync_id=%s version=%s", syncID, version)

	err = loadEnvFile(".env")
	if err != nil {
		logError("central-sync failed sync_id=%s environment error: %v", syncID, err)
		return
	}

	config, err := loadProjectConfig("central_config.yaml")
	if err != nil {
		logError("central-sync failed sync_id=%s configuration error: %v", syncID, err)
		return
	}

	if len(config.Projects) == 0 {
		logWarn("central-sync stopped sync_id=%s: no project mapping found", syncID)
		return
	}

	lockSet, err := acquireSyncRunLocks(config.Projects)
	if err != nil {
		logError("central-sync failed sync_id=%s sync lock error: %v", syncID, err)
		return
	}
	defer func() {
		if err := lockSet.Release(); err != nil {
			logError("central-sync sync_id=%s sync lock release error: %v", syncID, err)
		}
	}()

	client, err := newCentralClient()
	if err != nil {
		logError("central-sync failed sync_id=%s central client error: %v", syncID, err)
		return
	}

	logInfo("starting dataset sync")
	syncAllProjects(syncID, config.Projects, client)
	logInfo("dataset sync finished")

	logInfo("starting App User sync")
	syncAllAppUsers(syncID, config.Projects, client)
	logInfo("App User sync finished")

	logInfo("starting form sync")
	syncAllForms(syncID, config.Projects, client, config.AttachmentStorage)
	logInfo("form sync finished")

	logInfo("central-sync finished sync_id=%s version=%s", syncID, version)
}

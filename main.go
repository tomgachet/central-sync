package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			fmt.Printf("central-sync %s\n", version)
			return exitSuccess
		}
	}

	if err := initLogger(); err != nil {
		println("failed to initialize logger:", err.Error())
		return exitFailure
	}
	defer func() {
		if err := closeLogger(); err != nil {
			println("failed to close logger:", err.Error())
		}
	}()

	syncID, err := newSyncID()
	if err != nil {
		logError("startup error: %v", err)
		return exitFailure
	}

	logInfo("central-sync started sync_id=%s version=%s", syncID, version)

	err = loadEnvFile(".env")
	if err != nil {
		logError("central-sync failed sync_id=%s environment error: %v", syncID, err)
		return exitFailure
	}

	config, err := loadProjectConfig("central_config.yaml")
	if err != nil {
		logError("central-sync failed sync_id=%s configuration error: %v", syncID, err)
		return exitFailure
	}

	if len(config.Projects) == 0 {
		logWarn("central-sync stopped sync_id=%s: no project mapping found", syncID)
		return exitFailure
	}

	lockSet, err := acquireSyncRunLocks(config.Projects)
	if err != nil {
		logError("central-sync failed sync_id=%s sync lock error: %v", syncID, err)
		return exitFailure
	}
	defer func() {
		if err := lockSet.Release(); err != nil {
			logError("central-sync sync_id=%s sync lock release error: %v", syncID, err)
		}
	}()

	client, err := newCentralClient()
	if err != nil {
		logError("central-sync failed sync_id=%s central client error: %v", syncID, err)
		return exitFailure
	}

	var outcome syncOutcome
	logInfo("starting dataset sync")
	syncAllProjects(&outcome, syncID, config.Projects, client)
	logInfo("dataset sync finished")

	if hasAppUsersToSync(config.Projects) {
		logInfo("starting App User sync")
		syncAllAppUsers(&outcome, syncID, config.Projects, client)
		logInfo("App User sync finished")
	}

	logInfo("starting form sync")
	syncAllForms(&outcome, syncID, config.Projects, client, config.AttachmentStorage)
	logInfo("form sync finished")

	if !outcome.succeeded && !outcome.failed {
		logWarn("central-sync sync_id=%s: no synchronization work enabled", syncID)
	}

	code := outcome.exitCode()
	logInfo("central-sync finished sync_id=%s version=%s status=%s exit_status=%d", syncID, version, outcome.status(), code)
	return code
}

package main

import (
	"database/sql"
	"fmt"
)

func syncAllForms(projects []ProjectMapping, client *CentralClient) {
	for _, project := range projects {
		err := syncProjectForms(project, client)
		if err != nil {
			logError("[PROJECT] form sync error: %v", err)
		}
	}
}

func syncProjectForms(project ProjectMapping, client *CentralClient) error {
	exists, err := projectExists(client, project.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to validate project %d: %w", project.ProjectID, err)
	}

	if !exists {
		logWarn(
			"[PROJECT] skipping form sync for project_id=%d project_name=%q: project does not exist in ODK Central",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	formsToSync := getFormsToSync(project)

	if len(formsToSync) == 0 {
		logWarn(
			"[PROJECT] skipping form sync for project_id=%d project_name=%q: no form to sync",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	logInfo(
		"[PROJECT] processing forms for project_id=%d project_name=%q database=%s",
		project.ProjectID,
		project.ProjectName,
		project.DatabaseName,
	)

	db, err := connectProjectDatabase(project.DatabaseName)
	if err != nil {
		return fmt.Errorf("database connection error for project %d: %w", project.ProjectID, err)
	}
	defer db.Close()

	err = requireSchema(db, submissionSchema)
	if err != nil {
		return fmt.Errorf("submission schema error for project %d: %w", project.ProjectID, err)
	}

	for _, form := range formsToSync {
		err := syncSingleForm(db, project, form, client)
		if err != nil {
			logError(
				"[FORM] project_id=%d form=%q table=%s sync error: %v",
				project.ProjectID,
				form.XMLFormID,
				form.TableName,
				err,
			)
		}
	}

	return nil
}

func syncSingleForm(db *sql.DB, project ProjectMapping, form FormMapping, client *CentralClient) error {
	logInfo(
		"[FORM] project_id=%d form=%q table=%s mode=%s starting sync",
		project.ProjectID,
		form.XMLFormID,
		form.TableName,
		getFormSyncMode(form),
	)

	exists, err := formExists(client, project.ProjectID, form.XMLFormID)
	if err != nil {
		return fmt.Errorf("failed to validate form %s: %w", form.XMLFormID, err)
	}

	if !exists {
		return fmt.Errorf("form %s does not exist in ODK Central", form.XMLFormID)
	}

	formTables, err := getFormTables(client, project.ProjectID, form.XMLFormID, form.TableName)
	if err != nil {
		return fmt.Errorf("failed to discover OData tables for form %s: %w", form.XMLFormID, err)
	}

	doc, err := getFormMetadataDocument(client, project.ProjectID, form.XMLFormID)
	if err != nil {
		return fmt.Errorf("failed to load metadata for form %s: %w", form.XMLFormID, err)
	}

	parsedMetadata, err := parseFormMetadata(doc, formTables)
	if err != nil {
		return fmt.Errorf("failed to parse metadata for form %s: %w", form.XMLFormID, err)
	}

	syncMode := getFormSyncMode(form)
	approvedOnly := isApprovedOnly(form)
	approveAfterSync := shouldApproveAfterSync(form)

	lastSubmissionSync, err := getLastSuccessfulSubmissionSync(db, project.ProjectID, form.XMLFormID)
	if err != nil {
		return fmt.Errorf("failed to read last successful submissions sync for form %s: %w", form.XMLFormID, err)
	}

	failedSubmissions, err := getLastFailedSubmissions(db, project.ProjectID, form.XMLFormID)
	if err != nil {
		return fmt.Errorf("failed to read failed submissions for form %s: %w", form.XMLFormID, err)
	}
	failedSubmissionUUIDs := extractFailedSubmissionUUIDs(failedSubmissions)

	failedRowsByTable, err := fetchFailedSubmissionRowsByTable(
		client,
		project.ProjectID,
		form.XMLFormID,
		formTables,
		failedSubmissionUUIDs,
	)
	if err != nil {
		return fmt.Errorf("failed to fetch failed submission rows for form %s: %w", form.XMLFormID, err)
	}

	syncRunID, err := startSyncRun(db, SyncRunStartParams{
		ProjectID:    project.ProjectID,
		FormXMLID:    &form.XMLFormID,
		ObjectType:   "form",
		ObjectName:   form.XMLFormID,
		SQLTableName: form.TableName,
		SyncMode:     syncMode,
	})
	if err != nil {
		return fmt.Errorf("failed to start sync run for form %s: %w", form.XMLFormID, err)
	}

	logInfo(
		"[FORM] project_id=%d form=%q table=%s run_id=%d started approved_only=%t approve_after_sync=%t",
		project.ProjectID,
		form.XMLFormID,
		form.TableName,
		syncRunID,
		approvedOnly,
		approveAfterSync,
	)

	rowsByTable := make(map[string][]map[string]interface{})

	for _, formTable := range formTables {
		tableSchema, ok := parsedMetadata.Tables[formTable.ODataName]
		if !ok {
			errorMessage := fmt.Sprintf("missing parsed schema for OData table %s", formTable.ODataName)
			_ = finishSyncRun(db, SyncRunFinishParams{
				RunID:        syncRunID,
				SyncStatus:   "failed",
				ErrorMessage: &errorMessage,
			})
			return fmt.Errorf("%s", errorMessage)
		}

		err = ensureSubmissionTableExists(db, formTable)
		if err != nil {
			errorMessage := err.Error()
			_ = finishSyncRun(db, SyncRunFinishParams{
				RunID:        syncRunID,
				SyncStatus:   "failed",
				ErrorMessage: &errorMessage,
			})
			return fmt.Errorf("submission table error for form table %s: %w", formTable.ODataName, err)
		}

		err = ensureSubmissionTechnicalColumnsExist(db, formTable)
		if err != nil {
			errorMessage := err.Error()
			_ = finishSyncRun(db, SyncRunFinishParams{
				RunID:        syncRunID,
				SyncStatus:   "failed",
				ErrorMessage: &errorMessage,
			})
			return fmt.Errorf("submission technical column error for form table %s: %w", formTable.ODataName, err)
		}

		err = ensureSubmissionPropertyColumnsExist(db, formTable, tableSchema)
		if err != nil {
			errorMessage := err.Error()
			_ = finishSyncRun(db, SyncRunFinishParams{
				RunID:        syncRunID,
				SyncStatus:   "failed",
				ErrorMessage: &errorMessage,
			})
			return fmt.Errorf("submission property column error for form table %s: %w", formTable.ODataName, err)
		}

		filter := ""
		if formTable.IsRoot {
			filter = buildSubmissionRootFilter(lastSubmissionSync, syncMode, approvedOnly)
		} else {
			filter = buildSubmissionRepeatFilter(lastSubmissionSync, syncMode, approvedOnly)
		}

		if filter != "" {
			logInfo(
				"[FORM] project_id=%d form=%q run_id=%d table=%q applying filter: %s",
				project.ProjectID,
				form.XMLFormID,
				syncRunID,
				formTable.ODataName,
				filter,
			)
		}

		rows, err := getAllFormTableRows(
			client,
			project.ProjectID,
			form.XMLFormID,
			formTable.ODataURL,
			filter,
		)
		if err != nil {
			errorMessage := err.Error()
			_ = finishSyncRun(db, SyncRunFinishParams{
				RunID:        syncRunID,
				SyncStatus:   "failed",
				ErrorMessage: &errorMessage,
			})
			return fmt.Errorf("failed to fetch rows for form table %s: %w", formTable.ODataName, err)
		}

		filteredCount := len(rows)

		failedRows := failedRowsByTable[formTable.ODataName]
		if len(failedRows) > 0 {
			rows = mergeSubmissionRows(rows, failedRows)
			logInfo(
				"[FORM] project_id=%d form=%q run_id=%d table=%q fetched_rows=%d replay_failed_rows=%d merged_rows=%d",
				project.ProjectID,
				form.XMLFormID,
				syncRunID,
				formTable.ODataName,
				filteredCount,
				len(failedRows),
				len(rows),
			)
		} else {
			logInfo(
				"[FORM] project_id=%d form=%q run_id=%d table=%q fetched_rows=%d",
				project.ProjectID,
				form.XMLFormID,
				syncRunID,
				formTable.ODataName,
				filteredCount,
			)
		}

		rowsByTable[formTable.ODataName] = rows
	}

	batches, err := buildSubmissionBatches(formTables, *parsedMetadata, rowsByTable)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			ErrorMessage: &errorMessage,
		})
		return fmt.Errorf("failed to build submission batches for form %s: %w", form.XMLFormID, err)
	}

	logInfo(
		"[FORM] project_id=%d form=%q run_id=%d batches_built=%d",
		project.ProjectID,
		form.XMLFormID,
		syncRunID,
		len(batches),
	)

	var totalStats SyncStats
	hadFailure := false
	var firstErrorMessage *string

	for _, batch := range batches {
		batchStats, err := syncSubmissionBatch(
			db,
			client,
			syncRunID,
			project.ProjectID,
			form.XMLFormID,
			syncMode,
			approveAfterSync,
			batch,
		)

		if err != nil {
			hadFailure = true
			msg := err.Error()
			if firstErrorMessage == nil {
				firstErrorMessage = &msg
			}
			logError(
				"[FORM] project_id=%d form=%q run_id=%d submission_uuid=%s batch sync error: %v",
				project.ProjectID,
				form.XMLFormID,
				syncRunID,
				batch.RootSubmissionUUID,
				err,
			)
			continue
		}

		if batchStats != nil {
			totalStats.RowsFetched += batchStats.RowsFetched
			totalStats.RowsInserted += batchStats.RowsInserted
			totalStats.RowsUpdated += batchStats.RowsUpdated
			totalStats.RowsSkipped += batchStats.RowsSkipped
			totalStats.RowsFailed += batchStats.RowsFailed
			totalStats.SyncOutSubmissionDate = maxTimePtr(totalStats.SyncOutSubmissionDate, batchStats.SyncOutSubmissionDate)
			totalStats.SyncOutUpdatedAt = maxTimePtr(totalStats.SyncOutUpdatedAt, batchStats.SyncOutUpdatedAt)
		}
	}

	finalStatus := "success"
	if hadFailure {
		finalStatus = "partial_success"
	}

	err = finishSyncRun(db, SyncRunFinishParams{
		RunID:        syncRunID,
		SyncStatus:   finalStatus,
		RowsFetched:  totalStats.RowsFetched,
		RowsInserted: totalStats.RowsInserted,
		RowsUpdated:  totalStats.RowsUpdated,
		RowsSkipped:  totalStats.RowsSkipped,
		ErrorMessage: firstErrorMessage,
	})
	if err != nil {
		return fmt.Errorf("failed to finalize sync run for form %s: %w", form.XMLFormID, err)
	}

	logInfo(
		"[FORM] project_id=%d form=%q table=%s run_id=%d status=%s batches=%d fetched=%d inserted=%d updated=%d skipped=%d failed=%d",
		project.ProjectID,
		form.XMLFormID,
		form.TableName,
		syncRunID,
		finalStatus,
		len(batches),
		totalStats.RowsFetched,
		totalStats.RowsInserted,
		totalStats.RowsUpdated,
		totalStats.RowsSkipped,
		totalStats.RowsFailed,
	)

	return nil
}
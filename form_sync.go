package main

import (
	"database/sql"
	"fmt"
)

func syncAllForms(projects []ProjectMapping, client *CentralClient) {
	for _, project := range projects {
		err := syncProjectForms(project, client)
		if err != nil {
			fmt.Println("Project form sync error:", err)
		}
	}
}

func syncProjectForms(project ProjectMapping, client *CentralClient) error {
	exists, err := projectExists(client, project.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to validate project %d: %w", project.ProjectID, err)
	}

	if !exists {
		fmt.Printf(
			"\nSkipping form sync for project %d (%s): project does not exist in ODK Central\n",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	formsToSync := getFormsToSync(project)

	if len(formsToSync) == 0 {
		fmt.Printf(
			"\nSkipping form sync for project %d (%s): no form to sync\n",
			project.ProjectID,
			project.ProjectName,
		)
		return nil
	}

	fmt.Printf(
		"\nProcessing forms for project %d (%s) -> database %s\n",
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
			fmt.Println("Form sync error:", err)
		}
	}

	return nil
}

func syncSingleForm(db *sql.DB, project ProjectMapping, form FormMapping, client *CentralClient) error {
	fmt.Printf(
		"\nSyncing form %s -> root table %s (mode=%s)\n",
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
			return fmt.Errorf(errorMessage)
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
			filter = buildSubmissionRootFilter(lastSubmissionSync, syncMode)
		} else {
			filter = buildSubmissionRepeatFilter(lastSubmissionSync, syncMode)
		}

		if filter != "" {
			fmt.Printf("  Applying filter on %s: %s\n", formTable.ODataName, filter)
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

		failedRows := failedRowsByTable[formTable.ODataName]
		if len(failedRows) > 0 {
			rows = mergeSubmissionRows(rows, failedRows)
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

	var totalStats SyncStats
	hadFailure := false
	var firstErrorMessage *string

	for _, batch := range batches {
	batchStats, err := syncSubmissionBatch(
		db,
		syncRunID,
		project.ProjectID,
		form.XMLFormID,
		syncMode,
		batch,
	)

	if err != nil {
		hadFailure = true
		msg := err.Error()
		if firstErrorMessage == nil {
			firstErrorMessage = &msg
		}
		fmt.Printf("  Submission batch sync error for %s: %v\n", batch.RootSubmissionUUID, err)
		continue
	}

	if batchStats != nil {
		totalStats.RowsFetched += batchStats.RowsFetched
		totalStats.RowsInserted += batchStats.RowsInserted
		totalStats.RowsUpdated += batchStats.RowsUpdated
		totalStats.RowsSkipped += batchStats.RowsSkipped
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

	fmt.Printf(
		"Form %s synced: batches=%d fetched=%d inserted=%d updated=%d skipped=%d status=%s\n",
		form.XMLFormID,
		len(batches),
		totalStats.RowsFetched,
		totalStats.RowsInserted,
		totalStats.RowsUpdated,
		totalStats.RowsSkipped,
		finalStatus,
	)

	return nil
}
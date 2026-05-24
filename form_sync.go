package main

import "fmt"

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

func syncSingleForm(db DBExecutor, project ProjectMapping, form FormMapping, client *CentralClient) error {
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

	for _, formTable := range formTables {
		tableSchema, ok := parsedMetadata.Tables[formTable.ODataName]
		if !ok {
			return fmt.Errorf("missing parsed schema for OData table %s", formTable.ODataName)
		}

		err := syncSingleFormTable(db, project, form, formTable, tableSchema, syncMode, lastSubmissionSync, client)
		if err != nil {
			fmt.Println("Form table sync error:", err)
		}
	}

	return nil
}

func syncSingleFormTable(
	db DBExecutor,
	project ProjectMapping,
	form FormMapping,
	formTable FormTable,
	tableSchema FormTableSchema,
	syncMode string,
	lastSubmissionSync *LastSuccessfulSubmissionSync,
	client *CentralClient,
) error {
	fmt.Printf(
		"  Syncing OData table %s -> SQL table %s (mode=%s)\n",
		formTable.ODataName,
		formTable.SQLName,
		syncMode,
	)

	syncRunID, err := startSyncRun(db, SyncRunStartParams{
		ProjectID:    project.ProjectID,
		FormXMLID:    &form.XMLFormID,
		ObjectType:   "form",
		ObjectName:   form.XMLFormID,
		SQLTableName: formTable.SQLName,
		SyncMode:     syncMode,
	})
	if err != nil {
		return fmt.Errorf("failed to start sync run for form table %s: %w", formTable.ODataName, err)
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

	filter := ""
	if formTable.IsRoot {
		filter = buildSubmissionRootFilter(lastSubmissionSync, syncMode)
	} else {
		filter = buildSubmissionRepeatFilter(lastSubmissionSync, syncMode)
	}

	if filter != "" {
		fmt.Printf("    Applying filter: %s\n", filter)
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

	stats, err := syncFormTableRows(
	db,
	syncRunID,
	project.ProjectID,
	form.XMLFormID,
	formTable,
	tableSchema,
	syncMode,
	rows,
	)
	if err != nil {
		errorMessage := err.Error()
		_ = finishSyncRun(db, SyncRunFinishParams{
			RunID:        syncRunID,
			SyncStatus:   "failed",
			RowsFetched:  len(rows),
			ErrorMessage: &errorMessage,
		})
		return fmt.Errorf("failed to sync rows for form table %s: %w", formTable.ODataName, err)
	}

	err = finishSyncRun(db, SyncRunFinishParams{
		RunID:        syncRunID,
		SyncStatus:   "success",
		RowsFetched:  stats.RowsFetched,
		RowsInserted: stats.RowsInserted,
		RowsUpdated:  stats.RowsUpdated,
		RowsSkipped:  stats.RowsSkipped,
	})
	if err != nil {
		return fmt.Errorf("failed to finalize sync run for form table %s: %w", formTable.ODataName, err)
	}

	fmt.Printf(
		"  Form table %s synced successfully: fetched=%d inserted=%d updated=%d skipped=%d\n",
		formTable.ODataName,
		stats.RowsFetched,
		stats.RowsInserted,
		stats.RowsUpdated,
		stats.RowsSkipped,
	)

	return nil
}
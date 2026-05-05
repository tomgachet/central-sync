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
		"\nSyncing form %s -> table %s\n",
		form.XMLFormID,
		form.TableName,
	)

	exists, err := formExists(client, project.ProjectID, form.XMLFormID)
	if err != nil {
		return fmt.Errorf("failed to validate form %s: %w", form.XMLFormID, err)
	}

	if !exists {
		return fmt.Errorf("form %s does not exist in ODK Central", form.XMLFormID)
	}

	err = ensureSubmissionTableExists(db, form.TableName)
	if err != nil {
		return fmt.Errorf("submission table error for form %s: %w", form.XMLFormID, err)
	}

	err = ensureSubmissionTechnicalColumnsExist(db, form.TableName)
	if err != nil {
		return fmt.Errorf("submission technical column error for form %s: %w", form.XMLFormID, err)
	}

	rows, err := getAllFormSubmissions(client, project.ProjectID, form.XMLFormID)
	if err != nil {
		return fmt.Errorf("failed to fetch submissions for form %s: %w", form.XMLFormID, err)
	}

	err = syncFormSubmissions(db, form.TableName, rows)
	if err != nil {
		return fmt.Errorf("failed to sync submissions for form %s: %w", form.XMLFormID, err)
	}

	fmt.Printf(
		"Form %s synced successfully: %d submissions\n",
		form.XMLFormID,
		len(rows),
	)

	return nil
}
package main

import (
	"fmt"
)

const submissionSchema = "central_submissions"

func ensureSubmissionTableExists(db DBExecutor, tableName string) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			submission_uuid TEXT PRIMARY KEY,
			instance_id TEXT,
			data_json JSONB,

			central_submission_date TIMESTAMPTZ,
			central_updated_at TIMESTAMPTZ,
			central_deleted_at TIMESTAMPTZ,

			central_submitter_id INT,
			central_submitter_name VARCHAR(150),

			central_form_version TEXT,

			central_attachments_present INT,
			central_attachments_expected INT,

			central_device_id TEXT,
			central_edits INT,

			central_review_state TEXT,
			central_status TEXT,

			synced_at TIMESTAMPTZ
		)
	`, quoteIdentifier(submissionSchema), quoteIdentifier(tableName))

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf(
			"failed to create submission table %s.%s: %w",
			submissionSchema,
			tableName,
			err,
		)
	}

	return nil
}

func ensureSubmissionTechnicalColumnsExist(db DBExecutor, tableName string) error {
	technicalColumns := map[string]string{
		"instance_id":                  "TEXT",
		"data_json":                    "JSONB",

		"central_submission_date":      "TIMESTAMPTZ",
		"central_updated_at":           "TIMESTAMPTZ",
		"central_deleted_at":           "TIMESTAMPTZ",

		"central_submitter_id":         "INT",
		"central_submitter_name":       "VARCHAR(150)",

		"central_form_version":         "TEXT",

		"central_attachments_present":  "INT",
		"central_attachments_expected": "INT",

		"central_device_id":            "TEXT",
		"central_edits":                "INT",

		"central_review_state":         "TEXT",
		"central_status":               "TEXT",

		"synced_at":                    "TIMESTAMPTZ",
	}

	for columnName, columnType := range technicalColumns {
		exists, err := columnExists(db, submissionSchema, tableName, columnName)
		if err != nil {
			return err
		}

		if exists {
			continue
		}

		query := fmt.Sprintf(
			`ALTER TABLE %s.%s ADD COLUMN %s %s`,
			quoteIdentifier(submissionSchema),
			quoteIdentifier(tableName),
			quoteIdentifier(columnName),
			columnType,
		)

		_, err = db.Exec(query)
		if err != nil {
			return fmt.Errorf(
				"failed to add column %s to table %s.%s: %w",
				columnName,
				submissionSchema,
				tableName,
				err,
			)
		}
	}

	return nil
}
package main

import (
	"fmt"
	"strings"
)

const submissionSchema = "central_submissions"

func ensureSubmissionTableExists(db DBExecutor, formTable FormTable) error {
	if formTable.IsRoot {
		return ensureRootSubmissionTableExists(db, formTable.SQLName)
	}

	return ensureRepeatSubmissionTableExists(db, formTable.SQLName)
}

func ensureSubmissionTechnicalColumnsExist(db DBExecutor, formTable FormTable) error {
	if formTable.IsRoot {
		return ensureRootSubmissionTechnicalColumnsExist(db, formTable.SQLName)
	}

	return ensureRepeatSubmissionTechnicalColumnsExist(db, formTable.SQLName)
}

func ensureRootSubmissionTableExists(db DBExecutor, tableName string) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			row_uuid UUID PRIMARY KEY,
			instance_id UUID,
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
			"failed to create root submission table %s.%s: %w",
			submissionSchema,
			tableName,
			err,
		)
	}

	return nil
}

func ensureRepeatSubmissionTableExists(db DBExecutor, tableName string) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			row_uuid TEXT PRIMARY KEY,
			parent_row_uuid TEXT,
			data_json JSONB,
			synced_at TIMESTAMPTZ
		)
	`, quoteIdentifier(submissionSchema), quoteIdentifier(tableName))

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf(
			"failed to create repeat submission table %s.%s: %w",
			submissionSchema,
			tableName,
			err,
		)
	}

	return nil
}

func ensureRootSubmissionTechnicalColumnsExist(db DBExecutor, tableName string) error {
	columns := map[string]string{
		"instance_id":                  "UUID",
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

	return ensureSubmissionColumnsExist(db, tableName, columns)
}

func ensureRepeatSubmissionTechnicalColumnsExist(db DBExecutor, tableName string) error {
	columns := map[string]string{
		"parent_row_uuid": "TEXT",
		"data_json":       "JSONB",
		"synced_at":       "TIMESTAMPTZ",
	}

	return ensureSubmissionColumnsExist(db, tableName, columns)
}

func ensureSubmissionColumnsExist(db DBExecutor, tableName string, columns map[string]string) error {
	for name, typ := range columns {
		exists, err := columnExists(db, submissionSchema, tableName, name)
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
			quoteIdentifier(name),
			typ,
		)

		_, err = db.Exec(query)
		if err != nil {
			return fmt.Errorf(
				"failed to add column %s to table %s.%s: %w",
				name,
				submissionSchema,
				tableName,
				err,
			)
		}
	}

	return nil
}

func ensureSubmissionPropertyColumnsExist(db DBExecutor, formTable FormTable, tableSchema FormTableSchema) error {
	for _, column := range tableSchema.Columns {
		if isReservedSubmissionPropertyColumn(column.Name) {
			continue
		}

		if isSubmissionLinkColumn(column.Name) {
			continue
		}

		err := ensureSubmissionPropertyColumnExists(db, formTable.SQLName, column.Name, column.SQLType)
		if err != nil {
			return err
		}
	}

	return nil
}

func ensureSubmissionPropertyColumnExists(db DBExecutor, tableName string, columnName string, sqlType string) error {
	exists, err := columnExists(db, submissionSchema, tableName, columnName)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	if sqlType == "" {
		sqlType = "TEXT"
	}

	query := fmt.Sprintf(
		`ALTER TABLE %s.%s ADD COLUMN %s %s`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(tableName),
		quoteIdentifier(columnName),
		sqlType,
	)

	_, err = db.Exec(query)
	if err != nil {
		return fmt.Errorf(
			"failed to add property column %s to table %s.%s: %w",
			columnName,
			submissionSchema,
			tableName,
			err,
		)
	}

	return nil
}

func isReservedSubmissionPropertyColumn(columnName string) bool {
	reserved := map[string]bool{
		"row_uuid":                     true,
		"parent_row_uuid":              true,
		"instance_id":                  true,
		"data_json":                    true,
		"central_submission_date":      true,
		"central_updated_at":           true,
		"central_deleted_at":           true,
		"central_submitter_id":         true,
		"central_submitter_name":       true,
		"central_form_version":         true,
		"central_attachments_present":  true,
		"central_attachments_expected": true,
		"central_device_id":            true,
		"central_edits":                true,
		"central_review_state":         true,
		"central_status":               true,
		"synced_at":                    true,
	}

	return reserved[columnName]
}

func isSubmissionLinkColumn(columnName string) bool {
	return strings.HasPrefix(columnName, "__") && strings.HasSuffix(columnName, "-id")
}
package main

import (
	"database/sql"
	"fmt"
	"strings"
)

func ensureDatasetTableExists(db *sql.DB, tableName string) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			entity_uuid TEXT PRIMARY KEY,
			label TEXT,
			data_json JSONB,
			central_created_at TIMESTAMPTZ,
			central_updated_at TIMESTAMPTZ,
			central_deleted_at TIMESTAMPTZ,
			synced_at TIMESTAMPTZ
		)
	`, datasetSchema, quoteIdentifier(tableName))

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create dataset table %s.%s: %w", datasetSchema, tableName, err)
	}

	return nil
}

func columnExists(db *sql.DB, schemaName string, tableName string, columnName string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = $1
			  AND table_name = $2
			  AND column_name = $3
		)
	`

	var exists bool
	err := db.QueryRow(query, schemaName, tableName, columnName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check column existence: %w", err)
	}

	return exists, nil
}

func ensureDatasetColumnExists(db *sql.DB, tableName string, columnName string) error {
	exists, err := columnExists(db, datasetSchema, tableName, columnName)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	query := fmt.Sprintf(
		`ALTER TABLE %s.%s ADD COLUMN %s TEXT`,
		datasetSchema,
		quoteIdentifier(tableName),
		quoteIdentifier(columnName),
	)

	_, err = db.Exec(query)
	if err != nil {
		return fmt.Errorf(
			"failed to add column %s to table %s.%s: %w",
			columnName,
			datasetSchema,
			tableName,
			err,
		)
	}

	return nil
}

func ensureDatasetColumnsExist(db *sql.DB, tableName string, properties []DatasetProperty) error {
	for _, property := range properties {
		columnName := property.ODataName
		if columnName == "" {
			columnName = property.Name
		}

		columnName = strings.TrimSpace(columnName)
		if columnName == "" {
			continue
		}

		err := ensureDatasetColumnExists(db, tableName, columnName)
		if err != nil {
			return err
		}
	}

	return nil
}

func quoteIdentifier(identifier string) string {
	safe := strings.ReplaceAll(identifier, `"`, `""`)
	return `"` + safe + `"`
}
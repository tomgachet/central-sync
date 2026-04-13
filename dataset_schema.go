package main

import (
	"fmt"
	"strings"
)

func ensureDatasetTableExists(db DBExecutor, tableName string) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.%s (
			entity_uuid TEXT PRIMARY KEY,
			label TEXT,
			data_json JSONB,
			geometry_geojson JSONB,
			central_created_at TIMESTAMPTZ,
			central_updated_at TIMESTAMPTZ,
			central_deleted_at TIMESTAMPTZ,
			central_version INT,
			synced_at TIMESTAMPTZ
		)
	`, quoteIdentifier(datasetSchema), quoteIdentifier(tableName))

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf(
			"failed to create dataset table %s.%s: %w",
			datasetSchema,
			tableName,
			err,
		)
	}

	return nil
}

func ensureTechnicalColumnsExist(db DBExecutor, tableName string) error {
	technicalColumns := map[string]string{
		"label":              "TEXT",
		"data_json":          "JSONB",
		"geometry_geojson":   "JSONB",
		"central_created_at": "TIMESTAMPTZ",
		"central_updated_at": "TIMESTAMPTZ",
		"central_deleted_at": "TIMESTAMPTZ",
		"central_version":    "INT",
		"synced_at":          "TIMESTAMPTZ",
	}

	for columnName, columnType := range technicalColumns {
		exists, err := columnExists(db, datasetSchema, tableName, columnName)
		if err != nil {
			return err
		}

		if exists {
			continue
		}

		query := fmt.Sprintf(
			`ALTER TABLE %s.%s ADD COLUMN %s %s`,
			quoteIdentifier(datasetSchema),
			quoteIdentifier(tableName),
			quoteIdentifier(columnName),
			columnType,
		)

		_, err = db.Exec(query)
		if err != nil {
			return fmt.Errorf(
				"failed to add technical column %s to table %s.%s: %w",
				columnName,
				datasetSchema,
				tableName,
				err,
			)
		}
	}

	return nil
}

func ensureDatasetPropertyColumnsExist(db DBExecutor, tableName string, properties []DatasetProperty) error {
	for _, property := range properties {
		columnName := property.ODataName
		if columnName == "" {
			columnName = property.Name
		}

		columnName = strings.TrimSpace(columnName)
		if columnName == "" {
			continue
		}

		if isReservedTechnicalColumn(columnName) {
			continue
		}

		err := ensureDatasetPropertyColumnExists(db, tableName, columnName)
		if err != nil {
			return err
		}
	}

	return nil
}

func ensureDatasetPropertyColumnExists(db DBExecutor, tableName string, columnName string) error {
	exists, err := columnExists(db, datasetSchema, tableName, columnName)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	query := fmt.Sprintf(
		`ALTER TABLE %s.%s ADD COLUMN %s TEXT`,
		quoteIdentifier(datasetSchema),
		quoteIdentifier(tableName),
		quoteIdentifier(columnName),
	)

	_, err = db.Exec(query)
	if err != nil {
		return fmt.Errorf(
			"failed to add property column %s to table %s.%s: %w",
			columnName,
			datasetSchema,
			tableName,
			err,
		)
	}

	return nil
}

func columnExists(db DBExecutor, schemaName string, tableName string, columnName string) (bool, error) {
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

func quoteIdentifier(identifier string) string {
	safe := strings.ReplaceAll(identifier, `"`, `""`)
	return `"` + safe + `"`
}

func isReservedTechnicalColumn(columnName string) bool {
	reserved := map[string]bool{
		"entity_uuid":        true,
		"label":              true,
		"data_json":          true,
		"geometry_geojson":   true,
		"central_created_at": true,
		"central_updated_at": true,
		"central_deleted_at": true,
		"central_version":    true,
		"synced_at":          true,
	}

	return reserved[columnName]
}
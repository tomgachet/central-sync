package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func syncDatasetEntities(
	db DBExecutor,
	tableName string,
	entities []map[string]interface{},
	properties []DatasetProperty,
	geometryGeoJSONByEntityID map[string]interface{},
) (*SyncStats, error) {
	stats := &SyncStats{
		RowsFetched: len(entities),
	}

	for _, entity := range entities {
		action, entityUpdatedAt, err := upsertDatasetEntity(db, tableName, entity, properties, geometryGeoJSONByEntityID)
		if err != nil {
			return nil, err
		}

		switch action {
		case "inserted":
			stats.RowsInserted++
		case "updated":
			stats.RowsUpdated++
		case "skipped":
			stats.RowsSkipped++
		}

		stats.SyncOutUpdatedAt = maxTimePtr(stats.SyncOutUpdatedAt, entityUpdatedAt)
	}

	fmt.Printf(
		"Sync summary for %s.%s: fetched=%d inserted=%d updated=%d skipped=%d\n",
		datasetSchema,
		tableName,
		stats.RowsFetched,
		stats.RowsInserted,
		stats.RowsUpdated,
		stats.RowsSkipped,
	)

	return stats, nil
}

func upsertDatasetEntity(
	db DBExecutor,
	tableName string,
	entity map[string]interface{},
	properties []DatasetProperty,
	geometryGeoJSONByEntityID map[string]interface{},
) (string, *time.Time, error) {
	entityUUID, err := getEntityUUID(entity)
	if err != nil {
		return "", nil, err
	}

	label := getEntityLabel(entity)

	systemData, err := getEntitySystemData(entity)
	if err != nil {
		return "", nil, err
	}

	existingVersion, exists, err := getStoredEntityVersion(db, tableName, entityUUID)
	if err != nil {
		return "", nil, err
	}

	if exists && existingVersion == systemData.Version {
		return "skipped", systemData.UpdatedAt, nil
	}

	dataJSON, err := json.Marshal(entity)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal entity JSON: %w", err)
	}

	geometryGeoJSONValue, err := buildGeometryGeoJSONValue(geometryGeoJSONByEntityID, entityUUID)
	if err != nil {
		return "", nil, err
	}

	columns := []string{
		"entity_uuid",
		"label",
		"data_json",
		"geometry_geojson",
		"central_created_at",
		"central_updated_at",
		"central_deleted_at",
		"central_version",
		"synced_at",
	}

	values := []interface{}{
		entityUUID,
		label,
		dataJSON,
		geometryGeoJSONValue,
		systemData.CreatedAt,
		systemData.UpdatedAt,
		systemData.DeletedAt,
		systemData.Version,
		time.Now().UTC(),
	}

	updateAssignments := []string{
		`"label" = EXCLUDED."label"`,
		`"data_json" = EXCLUDED."data_json"`,
		`"geometry_geojson" = EXCLUDED."geometry_geojson"`,
		`"central_created_at" = EXCLUDED."central_created_at"`,
		`"central_updated_at" = EXCLUDED."central_updated_at"`,
		`"central_deleted_at" = EXCLUDED."central_deleted_at"`,
		`"central_version" = EXCLUDED."central_version"`,
		`"synced_at" = EXCLUDED."synced_at"`,
	}

	seenColumns := map[string]bool{
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

	for _, property := range properties {
		columnName := property.ODataName
		if columnName == "" {
			columnName = property.Name
		}

		columnName = strings.TrimSpace(columnName)
		if columnName == "" {
			continue
		}

		if seenColumns[columnName] {
			continue
		}
		seenColumns[columnName] = true

		columns = append(columns, columnName)
		values = append(values, getEntityPropertyValue(entity, columnName))
		updateAssignments = append(
			updateAssignments,
			fmt.Sprintf("%s = EXCLUDED.%s", quoteIdentifier(columnName), quoteIdentifier(columnName)),
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s.%s (%s) VALUES (%s)
		 ON CONFLICT (entity_uuid)
		 DO UPDATE SET %s`,
		quoteIdentifier(datasetSchema),
		quoteIdentifier(tableName),
		buildQuotedColumnList(columns),
		buildPlaceholders(len(values)),
		strings.Join(updateAssignments, ", "),
	)

	_, err = db.Exec(query, values...)
	if err != nil {
		return "", nil, fmt.Errorf(
			"failed to upsert entity %s into %s.%s: %w",
			entityUUID,
			datasetSchema,
			tableName,
			err,
		)
	}

	if exists {
		return "updated", systemData.UpdatedAt, nil
	}

	return "inserted", systemData.UpdatedAt, nil
}

type EntitySystemData struct {
	CreatedAt *time.Time
	UpdatedAt *time.Time
	DeletedAt *time.Time
	Version   int
}

func getEntitySystemData(entity map[string]interface{}) (*EntitySystemData, error) {
	rawSystem, ok := entity["__system"]
	if !ok || rawSystem == nil {
		return nil, fmt.Errorf("entity is missing __system")
	}

	systemMap, ok := rawSystem.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("entity __system is invalid")
	}

	version, err := extractInt(systemMap["version"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.version: %w", err)
	}

	createdAt, err := extractOptionalTime(systemMap["createdAt"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.createdAt: %w", err)
	}

	updatedAt, err := extractOptionalTime(systemMap["updatedAt"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.updatedAt: %w", err)
	}

	deletedAt, err := extractOptionalTime(systemMap["deletedAt"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.deletedAt: %w", err)
	}

	return &EntitySystemData{
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		DeletedAt: deletedAt,
		Version:   version,
	}, nil
}

func getStoredEntityVersion(db DBExecutor, tableName string, entityUUID string) (int, bool, error) {
	query := fmt.Sprintf(
		`SELECT central_version FROM %s.%s WHERE entity_uuid = $1`,
		quoteIdentifier(datasetSchema),
		quoteIdentifier(tableName),
	)

	var version sql.NullInt32
	err := db.QueryRow(query, entityUUID).Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to read stored entity version: %w", err)
	}

	if !version.Valid {
		return 0, true, nil
	}

	return int(version.Int32), true, nil
}

func getEntityUUID(entity map[string]interface{}) (string, error) {
	raw, ok := entity["__id"]
	if !ok {
		return "", fmt.Errorf("entity is missing __id")
	}

	value, ok := raw.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("entity __id is invalid")
	}

	return value, nil
}

func getEntityLabel(entity map[string]interface{}) string {
	raw, ok := entity["label"]
	if !ok {
		return ""
	}

	value, ok := raw.(string)
	if !ok {
		return ""
	}

	return value
}

func getEntityPropertyValue(entity map[string]interface{}, key string) interface{} {
	raw, ok := entity[key]
	if !ok || raw == nil {
		return nil
	}

	switch value := raw.(type) {
	case string:
		return value
	case bool:
		return fmt.Sprintf("%t", value)
	case float64:
		return fmt.Sprintf("%v", value)
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(bytes)
	}
}

func buildGeometryGeoJSONValue(geometryGeoJSONByEntityID map[string]interface{}, entityUUID string) ([]byte, error) {
	raw, ok := geometryGeoJSONByEntityID[entityUUID]
	if !ok || raw == nil {
		return nil, nil
	}

	geometryJSON, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal geometry GeoJSON for entity %s: %w", entityUUID, err)
	}

	return geometryJSON, nil
}

func extractInt(raw interface{}) (int, error) {
	switch value := raw.(type) {
	case float64:
		return int(value), nil
	case int:
		return value, nil
	case int32:
		return int(value), nil
	case int64:
		return int(value), nil
	case string:
		var i int
		_, err := fmt.Sscanf(value, "%d", &i)
		if err != nil {
			return 0, fmt.Errorf("unsupported numeric string %q", value)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", raw)
	}
}

func buildQuotedColumnList(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteIdentifier(column))
	}
	return strings.Join(quoted, ", ")
}

func buildPlaceholders(count int) string {
	placeholders := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
	}
	return strings.Join(placeholders, ", ")
}
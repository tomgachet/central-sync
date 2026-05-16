package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func syncFormTableRows(db DBExecutor, formTable FormTable, tableSchema FormTableSchema, rows []map[string]interface{}) error {
	insertedCount := 0
	updatedCount := 0
	skippedCount := 0

	for _, row := range rows {
		action, err := upsertFormTableRow(db, formTable, tableSchema, row)
		if err != nil {
			return err
		}

		switch action {
		case "inserted":
			insertedCount++
		case "updated":
			updatedCount++
		case "skipped":
			skippedCount++
		}
	}

	fmt.Printf(
		"Sync summary for %s.%s: inserted=%d updated=%d skipped=%d\n",
		submissionSchema,
		formTable.SQLName,
		insertedCount,
		updatedCount,
		skippedCount,
	)

	return nil
}

func upsertFormTableRow(db DBExecutor, formTable FormTable, tableSchema FormTableSchema, row map[string]interface{}) (string, error) {
	shape, err := analyzeSubmissionRow(formTable, row)
	if err != nil {
		return "", err
	}

	if shape.Kind == SubmissionTableRoot {
		return upsertRootSubmissionRow(db, formTable, tableSchema, row, shape)
	}

	return upsertRepeatSubmissionRow(db, formTable, tableSchema, row, shape)
}

func upsertRootSubmissionRow(
	db DBExecutor,
	formTable FormTable,
	tableSchema FormTableSchema,
	row map[string]interface{},
	shape *SubmissionRowShape,
) (string, error) {
	systemData, err := getSubmissionSystemData(row)
	if err != nil {
		return "", err
	}

	existingState, exists, err := getStoredRootSubmissionState(db, formTable.SQLName, shape.RowUUID)
	if err != nil {
		return "", err
	}

	if exists && rootSubmissionStateUnchanged(existingState, systemData) {
		return "skipped", nil
	}

	dataJSON, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("failed to marshal submission JSON: %w", err)
	}

	propertyColumns, propertyValues := buildTypedSubmissionPropertyValues(tableSchema, shape.FlatProperties)

	columns := []string{
		"row_uuid",
		"instance_id",
		"data_json",
		"central_submission_date",
		"central_updated_at",
		"central_deleted_at",
		"central_submitter_id",
		"central_submitter_name",
		"central_form_version",
		"central_attachments_present",
		"central_attachments_expected",
		"central_device_id",
		"central_edits",
		"central_review_state",
		"central_status",
		"synced_at",
	}

	values := []interface{}{
		shape.RowUUID,
		getSubmissionInstanceID(row),
		dataJSON,
		systemData.SubmissionDate,
		systemData.UpdatedAt,
		systemData.DeletedAt,
		systemData.SubmitterID,
		systemData.SubmitterName,
		systemData.FormVersion,
		systemData.AttachmentsPresent,
		systemData.AttachmentsExpected,
		systemData.DeviceID,
		systemData.Edits,
		systemData.ReviewState,
		systemData.Status,
		time.Now().UTC(),
	}

	updateAssignments := []string{
		`"instance_id" = EXCLUDED."instance_id"`,
		`"data_json" = EXCLUDED."data_json"`,
		`"central_submission_date" = EXCLUDED."central_submission_date"`,
		`"central_updated_at" = EXCLUDED."central_updated_at"`,
		`"central_deleted_at" = EXCLUDED."central_deleted_at"`,
		`"central_submitter_id" = EXCLUDED."central_submitter_id"`,
		`"central_submitter_name" = EXCLUDED."central_submitter_name"`,
		`"central_form_version" = EXCLUDED."central_form_version"`,
		`"central_attachments_present" = EXCLUDED."central_attachments_present"`,
		`"central_attachments_expected" = EXCLUDED."central_attachments_expected"`,
		`"central_device_id" = EXCLUDED."central_device_id"`,
		`"central_edits" = EXCLUDED."central_edits"`,
		`"central_review_state" = EXCLUDED."central_review_state"`,
		`"central_status" = EXCLUDED."central_status"`,
		`"synced_at" = EXCLUDED."synced_at"`,
	}

	for _, propertyColumn := range propertyColumns {
		columns = append(columns, propertyColumn)
		values = append(values, propertyValues[propertyColumn])
		updateAssignments = append(
			updateAssignments,
			fmt.Sprintf("%s = EXCLUDED.%s", quoteIdentifier(propertyColumn), quoteIdentifier(propertyColumn)),
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO %s.%s (%s) VALUES (%s)
		 ON CONFLICT ("row_uuid")
		 DO UPDATE SET %s`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(formTable.SQLName),
		buildQuotedColumnList(columns),
		buildPlaceholders(len(values)),
		stringsJoin(updateAssignments, ", "),
	)

	_, err = db.Exec(query, values...)
	if err != nil {
		return "", fmt.Errorf("failed to upsert root submission row %s: %w", shape.RowUUID, err)
	}

	if exists {
		return "updated", nil
	}

	return "inserted", nil
}

func upsertRepeatSubmissionRow(
	db DBExecutor,
	formTable FormTable,
	tableSchema FormTableSchema,
	row map[string]interface{},
	shape *SubmissionRowShape,
) (string, error) {
	exists, err := repeatSubmissionRowExists(db, formTable.SQLName, shape.RowUUID)
	if err != nil {
		return "", err
	}

	if exists {
		return "skipped", nil
	}

	dataJSON, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("failed to marshal repeat submission JSON: %w", err)
	}

	propertyColumns, propertyValues := buildTypedSubmissionPropertyValues(tableSchema, shape.FlatProperties)

	columns := []string{
		"row_uuid",
		"parent_row_uuid",
		"data_json",
		"synced_at",
	}

	values := []interface{}{
		shape.RowUUID,
		shape.ParentRowUUID,
		dataJSON,
		time.Now().UTC(),
	}

	for _, propertyColumn := range propertyColumns {
		columns = append(columns, propertyColumn)
		values = append(values, propertyValues[propertyColumn])
	}

	query := fmt.Sprintf(
		`INSERT INTO %s.%s (%s) VALUES (%s)
		 ON CONFLICT ("row_uuid")
		 DO NOTHING`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(formTable.SQLName),
		buildQuotedColumnList(columns),
		buildPlaceholders(len(values)),
	)

	_, err = db.Exec(query, values...)
	if err != nil {
		return "", fmt.Errorf("failed to insert repeat submission row %s: %w", shape.RowUUID, err)
	}

	return "inserted", nil
}

type SubmissionSystemData struct {
	SubmissionDate      *time.Time
	UpdatedAt           *time.Time
	DeletedAt           *time.Time
	SubmitterID         *int
	SubmitterName       string
	FormVersion         string
	AttachmentsPresent  *int
	AttachmentsExpected *int
	DeviceID            string
	Edits               *int
	ReviewState         string
	Status              string
}

type StoredRootSubmissionState struct {
	SubmissionDate sql.NullTime
	UpdatedAt      sql.NullTime
	DeletedAt      sql.NullTime
}

func getSubmissionInstanceID(row map[string]interface{}) *string {
	meta, ok := row["meta"].(map[string]interface{})
	if !ok {
		return nil
	}

	raw, ok := meta["instanceID"].(string)
	if !ok || raw == "" {
		return nil
	}

	clean := trimUUIDPrefix(raw)
	return &clean
}

func getSubmissionSystemData(row map[string]interface{}) (*SubmissionSystemData, error) {
	rawSystem, ok := row["__system"]
	if !ok || rawSystem == nil {
		return nil, fmt.Errorf("submission missing __system")
	}

	systemMap, ok := rawSystem.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("submission __system is invalid")
	}

	submissionDate, err := extractOptionalTime(systemMap["submissionDate"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.submissionDate: %w", err)
	}

	updatedAt, err := extractOptionalTime(systemMap["updatedAt"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.updatedAt: %w", err)
	}

	deletedAt, err := extractOptionalTime(systemMap["deletedAt"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.deletedAt: %w", err)
	}

	attachmentsPresent, err := extractOptionalInt(systemMap["attachmentsPresent"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.attachmentsPresent: %w", err)
	}

	attachmentsExpected, err := extractOptionalInt(systemMap["attachmentsExpected"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.attachmentsExpected: %w", err)
	}

	edits, err := extractOptionalInt(systemMap["edits"])
	if err != nil {
		return nil, fmt.Errorf("invalid __system.edits: %w", err)
	}

	return &SubmissionSystemData{
		SubmissionDate:      submissionDate,
		UpdatedAt:           updatedAt,
		DeletedAt:           deletedAt,
		SubmitterID:         mustInt(systemMap["submitterId"]),
		SubmitterName:       extractOptionalString(systemMap["submitterName"]),
		FormVersion:         extractOptionalString(systemMap["formVersion"]),
		AttachmentsPresent:  attachmentsPresent,
		AttachmentsExpected: attachmentsExpected,
		DeviceID:            extractOptionalString(systemMap["deviceId"]),
		Edits:               edits,
		ReviewState:         extractOptionalString(systemMap["reviewState"]),
		Status:              extractOptionalString(systemMap["status"]),
	}, nil
}

func getStoredRootSubmissionState(db DBExecutor, tableName string, rowUUID string) (*StoredRootSubmissionState, bool, error) {
	query := fmt.Sprintf(
		`SELECT "central_submission_date", "central_updated_at", "central_deleted_at"
		 FROM %s.%s
		 WHERE "row_uuid" = $1`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(tableName),
	)

	var state StoredRootSubmissionState
	err := db.QueryRow(query, rowUUID).Scan(&state.SubmissionDate, &state.UpdatedAt, &state.DeletedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to read stored root submission state: %w", err)
	}

	return &state, true, nil
}

func repeatSubmissionRowExists(db DBExecutor, tableName string, rowUUID string) (bool, error) {
	query := fmt.Sprintf(
		`SELECT EXISTS (
			SELECT 1
			FROM %s.%s
			WHERE "row_uuid" = $1
		)`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(tableName),
	)

	var exists bool
	err := db.QueryRow(query, rowUUID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check repeat row existence: %w", err)
	}

	return exists, nil
}

func rootSubmissionStateUnchanged(stored *StoredRootSubmissionState, current *SubmissionSystemData) bool {
	if stored == nil || current == nil {
		return false
	}

	if !sameNullableTime(stored.UpdatedAt, current.UpdatedAt) {
		return false
	}

	if !sameNullableTime(stored.SubmissionDate, current.SubmissionDate) {
		return false
	}

	if !sameNullableTime(stored.DeletedAt, current.DeletedAt) {
		return false
	}

	return true
}

func sameNullableTime(stored sql.NullTime, current *time.Time) bool {
	if !stored.Valid && current == nil {
		return true
	}

	if stored.Valid != (current != nil) {
		return false
	}

	if current == nil {
		return false
	}

	return stored.Time.Equal(*current)
}

func buildTypedSubmissionPropertyValues(tableSchema FormTableSchema, flatProperties map[string]interface{}) ([]string, map[string]interface{}) {
	columnTypes := make(map[string]string)
	for _, column := range tableSchema.Columns {
		columnTypes[column.Name] = column.SQLType
	}

	propertyValues := make(map[string]interface{})
	var propertyColumns []string

	for columnName, rawValue := range flatProperties {
		if isReservedSubmissionPropertyColumn(columnName) || isSubmissionLinkColumn(columnName) {
			continue
		}

		sqlType := columnTypes[columnName]
		propertyColumns = append(propertyColumns, columnName)
		propertyValues[columnName] = convertSubmissionPropertyValue(rawValue, sqlType)
	}

	sortStrings(propertyColumns)
	return propertyColumns, propertyValues
}

func convertSubmissionPropertyValue(raw interface{}, sqlType string) interface{} {
	if raw == nil {
		return nil
	}

	switch sqlType {
	case "BOOLEAN":
		switch v := raw.(type) {
		case bool:
			return v
		case string:
			return v == "true"
		default:
			return nil
		}

	case "INT", "BIGINT":
		switch v := raw.(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		case string:
			var out int64
			_, err := fmt.Sscanf(v, "%d", &out)
			if err != nil {
				return nil
			}
			return out
		default:
			return nil
		}

	case "DOUBLE PRECISION":
		switch v := raw.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case string:
			var out float64
			_, err := fmt.Sscanf(v, "%f", &out)
			if err != nil {
				return nil
			}
			return out
		default:
			return nil
		}

	case "TIMESTAMPTZ":
		if t, err := extractOptionalTime(raw); err == nil {
			return t
		}
		return nil

	case "DATE":
		if s, ok := raw.(string); ok {
			return s
		}
		return nil

	case "JSONB":
		bytes, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		return bytes

	default:
		switch v := raw.(type) {
		case string:
			return v
		case bool:
			if v {
				return "true"
			}
			return "false"
		case float64:
			return fmt.Sprintf("%v", v)
		default:
			bytes, err := json.Marshal(v)
			if err != nil {
				return fmt.Sprintf("%v", v)
			}
			return string(bytes)
		}
	}
}
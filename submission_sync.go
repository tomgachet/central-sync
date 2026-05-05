package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func syncFormSubmissions(db DBExecutor, tableName string, rows []map[string]interface{}) error {
	insertedCount := 0
	updatedCount := 0
	skippedCount := 0

	for _, row := range rows {
		action, err := upsertFormSubmission(db, tableName, row)
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
		tableName,
		insertedCount,
		updatedCount,
		skippedCount,
	)

	return nil
}

func upsertFormSubmission(db DBExecutor, tableName string, row map[string]interface{}) (string, error) {
	submissionUUID, err := getSubmissionUUID(row)
	if err != nil {
		return "", err
	}

	instanceID := getSubmissionInstanceID(row)

	systemData, err := getSubmissionSystemData(row)
	if err != nil {
		return "", err
	}

	existingState, exists, err := getStoredSubmissionState(db, tableName, submissionUUID)
	if err != nil {
		return "", err
	}

	if exists && submissionStateUnchanged(existingState, systemData) {
		return "skipped", nil
	}

	dataJSON, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("failed to marshal submission JSON: %w", err)
	}

	pointGeoJSON, err := buildSubmissionGeoJSONValue(row, "point")
	if err != nil {
		return "", err
	}

	shapeGeoJSON, err := buildSubmissionGeoJSONValue(row, "shape")
	if err != nil {
		return "", err
	}

	query := fmt.Sprintf(
		`INSERT INTO %s.%s (
			"submission_uuid",
			"instance_id",
			"data_json",
			"point_geojson",
			"shape_geojson",
			"central_submission_date",
			"central_updated_at",
			"central_deleted_at",
			"central_submitter_id",
			"central_submitter_name",
			"central_form_version",
			"synced_at"
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT ("submission_uuid")
		DO UPDATE SET
			"instance_id" = EXCLUDED."instance_id",
			"data_json" = EXCLUDED."data_json",
			"point_geojson" = EXCLUDED."point_geojson",
			"shape_geojson" = EXCLUDED."shape_geojson",
			"central_submission_date" = EXCLUDED."central_submission_date",
			"central_updated_at" = EXCLUDED."central_updated_at",
			"central_deleted_at" = EXCLUDED."central_deleted_at",
			"central_submitter_id" = EXCLUDED."central_submitter_id",
			"central_submitter_name" = EXCLUDED."central_submitter_name",
			"central_form_version" = EXCLUDED."central_form_version",
			"synced_at" = EXCLUDED."synced_at"`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(tableName),
	)

	_, err = db.Exec(
		query,
		submissionUUID,
		instanceID,
		dataJSON,
		pointGeoJSON,
		shapeGeoJSON,
		systemData.SubmissionDate,
		systemData.UpdatedAt,
		systemData.DeletedAt,
		systemData.SubmitterID,
		systemData.SubmitterName,
		systemData.FormVersion,
		time.Now().UTC(),
	)
	if err != nil {
		return "", fmt.Errorf(
			"failed to upsert submission %s into %s.%s: %w",
			submissionUUID,
			submissionSchema,
			tableName,
			err,
		)
	}

	if exists {
		return "updated", nil
	}

	return "inserted", nil
}

type SubmissionSystemData struct {
	SubmissionDate *time.Time
	UpdatedAt      *time.Time
	DeletedAt      *time.Time
	SubmitterID    string
	SubmitterName  string
	FormVersion    string
}

type StoredSubmissionState struct {
	SubmissionDate sql.NullTime
	UpdatedAt      sql.NullTime
}

func getSubmissionUUID(row map[string]interface{}) (string, error) {
	raw, ok := row["__id"]
	if !ok {
		return "", fmt.Errorf("submission is missing __id")
	}

	value, ok := raw.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("submission __id is invalid")
	}

	return value, nil
}

func getSubmissionInstanceID(row map[string]interface{}) string {
	rawMeta, ok := row["meta"]
	if !ok || rawMeta == nil {
		return ""
	}

	metaMap, ok := rawMeta.(map[string]interface{})
	if !ok {
		return ""
	}

	rawInstanceID, ok := metaMap["instanceID"]
	if !ok || rawInstanceID == nil {
		return ""
	}

	value, ok := rawInstanceID.(string)
	if !ok {
		return ""
	}

	return value
}

func getSubmissionSystemData(row map[string]interface{}) (*SubmissionSystemData, error) {
	rawSystem, ok := row["__system"]
	if !ok || rawSystem == nil {
		return nil, fmt.Errorf("submission is missing __system")
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

	return &SubmissionSystemData{
		SubmissionDate: submissionDate,
		UpdatedAt:      updatedAt,
		DeletedAt:      deletedAt,
		SubmitterID:    extractOptionalString(systemMap["submitterId"]),
		SubmitterName:  extractOptionalString(systemMap["submitterName"]),
		FormVersion:    extractOptionalString(systemMap["formVersion"]),
	}, nil
}

func getStoredSubmissionState(db DBExecutor, tableName string, submissionUUID string) (*StoredSubmissionState, bool, error) {
	query := fmt.Sprintf(
		`SELECT "central_submission_date", "central_updated_at"
		 FROM %s.%s
		 WHERE "submission_uuid" = $1`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(tableName),
	)

	var state StoredSubmissionState
	err := db.QueryRow(query, submissionUUID).Scan(&state.SubmissionDate, &state.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to read stored submission state: %w", err)
	}

	return &state, true, nil
}

func submissionStateUnchanged(stored *StoredSubmissionState, current *SubmissionSystemData) bool {
	if stored == nil || current == nil {
		return false
	}

	if !sameNullableTime(stored.UpdatedAt, current.UpdatedAt) {
		return false
	}

	if !sameNullableTime(stored.SubmissionDate, current.SubmissionDate) {
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

func buildSubmissionGeoJSONValue(row map[string]interface{}, key string) ([]byte, error) {
	raw, ok := row[key]
	if !ok || raw == nil {
		return nil, nil
	}

	geoJSON, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s GeoJSON: %w", key, err)
	}

	return geoJSON, nil
}

func extractOptionalString(raw interface{}) string {
	if raw == nil {
		return ""
	}

	value, ok := raw.(string)
	if !ok {
		return fmt.Sprintf("%v", raw)
	}

	return value
}
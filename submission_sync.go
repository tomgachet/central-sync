package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func syncFormSubmissions(db DBExecutor, tableName string, rows []map[string]interface{}) error {
	inserted := 0
	updated := 0
	skipped := 0

	for _, row := range rows {
		action, err := upsertFormSubmission(db, tableName, row)
		if err != nil {
			return err
		}

		switch action {
		case "inserted":
			inserted++
		case "updated":
			updated++
		case "skipped":
			skipped++
		}
	}

	fmt.Printf(
		"Sync summary for %s.%s: inserted=%d updated=%d skipped=%d\n",
		submissionSchema,
		tableName,
		inserted,
		updated,
		skipped,
	)

	return nil
}

func upsertFormSubmission(db DBExecutor, tableName string, row map[string]interface{}) (string, error) {
	submissionUUID, err := getSubmissionUUID(row)
	if err != nil {
		return "", err
	}

	instanceID := getSubmissionInstanceID(row)

	system, err := getSubmissionSystemData(row)
	if err != nil {
		return "", err
	}

	stored, exists, err := getStoredSubmissionState(db, tableName, submissionUUID)
	if err != nil {
		return "", err
	}

	if exists && submissionStateUnchanged(stored, system) {
		return "skipped", nil
	}

	dataJSON, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("failed to marshal submission JSON: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s.%s (
			submission_uuid,
			instance_id,
			data_json,
			central_submission_date,
			central_updated_at,
			central_deleted_at,
			central_submitter_id,
			central_submitter_name,
			central_form_version,
			central_attachments_present,
			central_attachments_expected,
			central_device_id,
			central_edits,
			central_review_state,
			central_status,
			synced_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (submission_uuid)
		DO UPDATE SET
			instance_id = EXCLUDED.instance_id,
			data_json = EXCLUDED.data_json,
			central_submission_date = EXCLUDED.central_submission_date,
			central_updated_at = EXCLUDED.central_updated_at,
			central_deleted_at = EXCLUDED.central_deleted_at,
			central_submitter_id = EXCLUDED.central_submitter_id,
			central_submitter_name = EXCLUDED.central_submitter_name,
			central_form_version = EXCLUDED.central_form_version,
			central_attachments_present = EXCLUDED.central_attachments_present,
			central_attachments_expected = EXCLUDED.central_attachments_expected,
			central_device_id = EXCLUDED.central_device_id,
			central_edits = EXCLUDED.central_edits,
			central_review_state = EXCLUDED.central_review_state,
			central_status = EXCLUDED.central_status,
			synced_at = EXCLUDED.synced_at
	`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(tableName),
	)

	_, err = db.Exec(
		query,
		submissionUUID,
		instanceID,
		dataJSON,
		system.SubmissionDate,
		system.UpdatedAt,
		system.DeletedAt,
		system.SubmitterID,
		system.SubmitterName,
		system.FormVersion,
		system.AttachmentsPresent,
		system.AttachmentsExpected,
		system.DeviceID,
		system.Edits,
		system.ReviewState,
		system.Status,
		time.Now().UTC(),
	)
	if err != nil {
		return "", fmt.Errorf("failed to upsert submission %s: %w", submissionUUID, err)
	}

	if exists {
		return "updated", nil
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

type StoredSubmissionState struct {
	SubmissionDate sql.NullTime
	UpdatedAt      sql.NullTime
}

func getSubmissionUUID(row map[string]interface{}) (string, error) {
	val, ok := row["__id"].(string)
	if !ok || val == "" {
		return "", fmt.Errorf("invalid __id")
	}
	return val, nil
}

func getSubmissionInstanceID(row map[string]interface{}) string {
	meta, ok := row["meta"].(map[string]interface{})
	if !ok {
		return ""
	}

	val, _ := meta["instanceID"].(string)
	return val
}

func getSubmissionSystemData(row map[string]interface{}) (*SubmissionSystemData, error) {
	sys, ok := row["__system"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing __system")
	}

	return &SubmissionSystemData{
		SubmissionDate:      mustTime(sys["submissionDate"]),
		UpdatedAt:           mustTime(sys["updatedAt"]),
		DeletedAt:           mustTime(sys["deletedAt"]),
		SubmitterID:         mustInt(sys["submitterId"]),
		SubmitterName:       mustString(sys["submitterName"]),
		FormVersion:         mustString(sys["formVersion"]),
		AttachmentsPresent:  mustInt(sys["attachmentsPresent"]),
		AttachmentsExpected: mustInt(sys["attachmentsExpected"]),
		DeviceID:            mustString(sys["deviceId"]),
		Edits:               mustInt(sys["edits"]),
		ReviewState:         mustString(sys["reviewState"]),
		Status:              mustString(sys["status"]),
	}, nil
}

func getStoredSubmissionState(db DBExecutor, tableName string, uuid string) (*StoredSubmissionState, bool, error) {
	query := fmt.Sprintf(`
		SELECT central_submission_date, central_updated_at
		FROM %s.%s
		WHERE submission_uuid = $1
	`,
		quoteIdentifier(submissionSchema),
		quoteIdentifier(tableName),
	)

	var state StoredSubmissionState
	err := db.QueryRow(query, uuid).Scan(&state.SubmissionDate, &state.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
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

func mustString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

func mustInt(v interface{}) *int {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case float64:
		i := int(val)
		return &i
	case int:
		return &val
	default:
		return nil
	}
}

func mustTime(v interface{}) *time.Time {
	if v == nil {
		return nil
	}

	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}

	return &t
}
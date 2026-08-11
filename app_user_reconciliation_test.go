package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const classifyAppUserQueryPattern = `(?s)SELECT.*missing_from_central.*AS changed.*FROM "central_users"\.app_users.*FOR UPDATE`
const upsertAppUserQueryPattern = `(?s)INSERT INTO "central_users"\.app_users .*ON CONFLICT \(project_id, app_user_id\).*DO UPDATE SET`
const markMissingAppUsersQueryPattern = `(?s)UPDATE "central_users"\.app_users.*missing_from_central = TRUE.*RETURNING app_user_id`
const insertAppUserDetailQueryPattern = `(?s)INSERT INTO "central_metadata"\.sync_runs_detail .*app_user_id.*VALUES`

func TestReconcileProjectAppUsersCommitsCompleteSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create SQL mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(classifyAppUserQueryPattern).
		WithArgs(
			7,
			int64(115),
			"Collector North",
			"field_key",
			nil,
			nil,
			nil,
			nil,
			nil,
			[]byte(`{"region":"North"}`),
			false,
		).
		WillReturnRows(sqlmock.NewRows([]string{"missing_from_central", "changed"}))
	mock.ExpectExec(upsertAppUserQueryPattern).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(markMissingAppUsersQueryPattern).
		WithArgs(7, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"app_user_id", "display_name"}).
			AddRow(int64(99), "Missing Collector"))
	expectAppUserDetailInsert(mock)
	expectAppUserDetailInsert(mock)
	mock.ExpectCommit()

	results, err := reconcileProjectAppUsers(db, 42, 7, []CentralAppUser{{
		ID:          115,
		ProjectID:   7,
		DisplayName: "Collector North",
		Type:        "field_key",
		Properties:  map[string]string{"region": "North"},
	}})
	if err != nil {
		t.Fatalf("reconcileProjectAppUsers returned error: %v", err)
	}
	if len(results) != 2 ||
		results[0].AppUserID != 115 ||
		results[0].Action != AppUserActionInserted ||
		results[1].AppUserID != 99 ||
		results[1].Action != AppUserActionMarkedMissing {
		t.Fatalf("unexpected reconciliation results: %+v", results)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestReconcileProjectAppUsersRestoresExistingUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create SQL mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(classifyAppUserQueryPattern).
		WillReturnRows(sqlmock.NewRows([]string{"missing_from_central", "changed"}).AddRow(true, false))
	mock.ExpectExec(upsertAppUserQueryPattern).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(markMissingAppUsersQueryPattern).
		WillReturnRows(sqlmock.NewRows([]string{"app_user_id", "display_name"}))
	expectAppUserDetailInsert(mock)
	mock.ExpectCommit()

	results, err := reconcileProjectAppUsers(db, 42, 7, []CentralAppUser{{
		ID:          115,
		DisplayName: "Collector North",
		Type:        "field_key",
	}})
	if err != nil {
		t.Fatalf("reconcileProjectAppUsers returned error: %v", err)
	}
	if len(results) != 1 || results[0].Action != AppUserActionRestored {
		t.Fatalf("unexpected reconciliation results: %+v", results)
	}
}

func TestClassifyAppUserActionDistinguishesUpdatedAndSkipped(t *testing.T) {
	tests := []struct {
		name     string
		changed  bool
		expected string
	}{
		{name: "updated", changed: true, expected: AppUserActionUpdated},
		{name: "skipped", changed: false, expected: AppUserActionSkipped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create SQL mock: %v", err)
			}
			defer db.Close()

			mock.ExpectBegin()
			mock.ExpectQuery(classifyAppUserQueryPattern).
				WillReturnRows(sqlmock.NewRows([]string{"missing_from_central", "changed"}).
					AddRow(false, tt.changed))
			mock.ExpectRollback()

			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("failed to begin transaction: %v", err)
			}
			action, err := classifyAppUserAction(
				tx,
				7,
				CentralAppUser{ID: 115, DisplayName: "Collector", Type: "field_key"},
				nil,
				[]byte("{}"),
			)
			if err != nil {
				t.Fatalf("classifyAppUserAction returned error: %v", err)
			}
			if action != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, action)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatalf("failed to roll back test transaction: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet SQL expectations: %v", err)
			}
		})
	}
}

func TestReconcileProjectAppUsersDoesNotWriteDetailForSkippedUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create SQL mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(classifyAppUserQueryPattern).
		WillReturnRows(sqlmock.NewRows([]string{"missing_from_central", "changed"}).
			AddRow(false, false))
	mock.ExpectExec(upsertAppUserQueryPattern).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(markMissingAppUsersQueryPattern).
		WillReturnRows(sqlmock.NewRows([]string{"app_user_id", "display_name"}))
	mock.ExpectCommit()

	results, err := reconcileProjectAppUsers(db, 42, 7, []CentralAppUser{{
		ID:          115,
		DisplayName: "Collector",
		Type:        "field_key",
	}})
	if err != nil {
		t.Fatalf("reconcileProjectAppUsers returned error: %v", err)
	}
	if len(results) != 1 || results[0].Action != AppUserActionSkipped {
		t.Fatalf("unexpected reconciliation results: %+v", results)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected sync detail for skipped App User: %v", err)
	}
}

func TestReconcileProjectAppUsersMarksAllExistingUsersMissingForEmptySnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create SQL mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(markMissingAppUsersQueryPattern).
		WithArgs(7, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"app_user_id", "display_name"}).
			AddRow(int64(115), "Collector North").
			AddRow(int64(116), "Collector South"))
	expectAppUserDetailInsert(mock)
	expectAppUserDetailInsert(mock)
	mock.ExpectCommit()

	results, err := reconcileProjectAppUsers(db, 42, 7, nil)
	if err != nil {
		t.Fatalf("reconcileProjectAppUsers returned error: %v", err)
	}
	if len(results) != 2 ||
		results[0].Action != AppUserActionMarkedMissing ||
		results[1].Action != AppUserActionMarkedMissing {
		t.Fatalf("unexpected reconciliation results: %+v", results)
	}
}

func expectAppUserDetailInsert(mock sqlmock.Sqlmock) {
	mock.ExpectExec(insertAppUserDetailQueryPattern).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestReconcileProjectAppUsersRollsBackOnUpsertFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create SQL mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(classifyAppUserQueryPattern).
		WillReturnRows(sqlmock.NewRows([]string{"missing_from_central", "changed"}))
	mock.ExpectExec(upsertAppUserQueryPattern).
		WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()

	_, err = reconcileProjectAppUsers(db, 42, 7, []CentralAppUser{{
		ID:          115,
		DisplayName: "Collector North",
		Type:        "field_key",
	}})
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("expected contextual database error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestReconcileProjectAppUsersRejectsDuplicateSnapshotIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create SQL mock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(classifyAppUserQueryPattern).
		WillReturnRows(sqlmock.NewRows([]string{"missing_from_central", "changed"}))
	mock.ExpectExec(upsertAppUserQueryPattern).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectRollback()

	appUser := CentralAppUser{ID: 115, DisplayName: "Collector", Type: "field_key"}
	_, err = reconcileProjectAppUsers(db, 42, 7, []CentralAppUser{appUser, appUser})
	if err == nil || !strings.Contains(err.Error(), "duplicate App User ID 115") {
		t.Fatalf("expected duplicate ID error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestAppUserSchemaUsesCompositeIdentityAndMissingState(t *testing.T) {
	assertSQLFileContains(t, "sql_init/01_init_structure.sql", []string{
		"CREATE SCHEMA IF NOT EXISTS central_users",
		"CREATE TABLE IF NOT EXISTS central_users.app_users",
		"PRIMARY KEY (project_id, app_user_id)",
		"missing_from_central BOOLEAN NOT NULL DEFAULT FALSE",
		"properties JSONB NOT NULL DEFAULT '{}'::jsonb",
		"app_user_id BIGINT",
		"sync_runs_detail_app_user_idx",
	})
	assertSQLFileContains(t, "sql_migrations/v0.5.0_add_app_users.sql", []string{
		"CREATE SCHEMA IF NOT EXISTS central_users",
		"CREATE TABLE IF NOT EXISTS central_users.app_users",
		"ADD COLUMN IF NOT EXISTS app_user_id BIGINT",
		"ALTER TABLE central_users.app_users OWNER TO your_central_user",
	})
}

func assertSQLFileContains(t *testing.T, path string, expected []string) {
	t.Helper()
	content := readTestFile(t, path)
	for _, fragment := range expected {
		if !strings.Contains(content, fragment) {
			t.Fatalf("%s does not contain %q", path, fragment)
		}
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(content)
}

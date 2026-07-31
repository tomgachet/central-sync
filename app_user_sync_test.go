package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestSummarizeAppUserResults(t *testing.T) {
	results := []AppUserReconcileResult{
		{AppUserID: 1, Action: AppUserActionInserted},
		{AppUserID: 2, Action: AppUserActionUpdated},
		{AppUserID: 3, Action: AppUserActionRestored},
		{AppUserID: 4, Action: AppUserActionSkipped},
		{AppUserID: 5, Action: AppUserActionMarkedMissing},
	}

	stats := summarizeAppUserResults(4, results)
	if stats.RowsFetched != 4 ||
		stats.RowsInserted != 1 ||
		stats.RowsUpdated != 3 ||
		stats.RowsSkipped != 1 {
		t.Fatalf("unexpected App User stats: %+v", stats)
	}
	if got := countAppUserActions(results, AppUserActionMarkedMissing); got != 1 {
		t.Fatalf("expected 1 missing action, got %d", got)
	}
}

func TestSyncSingleProjectAppUsersDoesNotReconcileFailedSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create SQL mock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`(?s)INSERT INTO "central_metadata"\.sync_runs .*RETURNING run_id`).
		WillReturnRows(sqlmock.NewRows([]string{"run_id"}).AddRow(int64(42)))
	mock.ExpectExec(`(?s)UPDATE "central_metadata"\.sync_runs.*sync_status`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"invalid":"snapshot"}`))
	}))
	defer server.Close()

	client := &CentralClient{
		BaseURL:    server.URL,
		Token:      "central-session-token",
		HTTPClient: server.Client(),
	}
	project := ProjectMapping{ProjectID: 7, DatabaseName: "project_db", SyncAppUsers: true}

	err = syncSingleProjectAppUsers(
		uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		db,
		project,
		client,
	)
	if err == nil || !strings.Contains(err.Error(), "failed to decode App Users response") {
		t.Fatalf("expected snapshot decoding error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected reconciliation query after failed snapshot: %v", err)
	}
}

package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestSyncSingleDatasetOutcome(t *testing.T) {
	for _, test := range []struct {
		name       string
		entities   string
		inserted   int
		failed     int
		wantStatus string
		wantExit   int
	}{
		{"no new entities", "", 0, 0, "success", 0},
		{"one invalid entity", `{"__id":"invalid"},{"__id":"valid","__system":{"version":1}}`, 1, 1, "partial_success", 1},
		{"all entities invalid", `{"__id":"invalid"}`, 0, 1, "failed", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.MatchExpectationsInOrder(false)

			mock.ExpectQuery("FROM .*last_successful_datasets_sync").WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`INSERT INTO "central_metadata"\.sync_runs .*RETURNING run_id`).
				WillReturnRows(sqlmock.NewRows([]string{"run_id"}).AddRow(42))
			mock.ExpectExec(`CREATE TABLE IF NOT EXISTS "central_datasets"\."etudes"`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			for range 8 {
				mock.ExpectQuery("FROM information_schema.columns").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			}
			if test.inserted > 0 {
				mock.ExpectQuery("SELECT central_version, central_deleted_at").WithArgs("valid").
					WillReturnError(sql.ErrNoRows)
				mock.ExpectExec(`INSERT INTO "central_datasets"\."etudes"`).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			for range test.inserted + test.failed {
				mock.ExpectExec(`INSERT INTO "central_metadata"\.sync_runs_detail`).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec(`UPDATE "central_metadata"\.sync_runs`).
				WithArgs(42, sqlmock.AnyArg(), test.wantStatus, test.inserted+test.failed, test.inserted,
					0, 0, test.failed, 0, 0, 0, 0, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/projects/7/datasets/etudes":
					fmt.Fprint(w, `{"properties":[]}`)
				case "/v1/projects/7/datasets/etudes.svc/Entities":
					entities := test.entities
					if r.URL.Query().Get("$filter") != "" {
						entities = ""
					}
					fmt.Fprintf(w, `{"value":[%s]}`, entities)
				case "/v1/projects/7/datasets/etudes/entities.geojson":
					fmt.Fprint(w, `{"type":"FeatureCollection","features":[]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := &CentralClient{BaseURL: server.URL, HTTPClient: server.Client()}
			project := ProjectMapping{ProjectID: 7}
			dataset := DatasetMapping{Name: "etudes", TableName: "etudes"}
			completed, err := syncSingleDataset(uuid.New(), db, project, dataset, client)
			if mockErr := mock.ExpectationsWereMet(); mockErr != nil {
				t.Fatalf("%v (sync error: %v)", mockErr, err)
			}
			var outcome syncOutcome
			outcome.record(completed, err)
			if got := outcome.exitCode(); got != test.wantExit {
				t.Fatalf("exit status = %d, want %d (completed = %t, err = %v)", got, test.wantExit, completed, err)
			}
		})
	}
}

func TestDatasetEntityVersionAndDeletion(t *testing.T) {
	for _, tt := range []struct {
		name          string
		exists        bool
		storedVersion int
		version       int
		deleted       interface{}
		want          string
	}{
		{"initial load", false, 0, 1, nil, "inserted"},
		{"unchanged version", true, 1, 1, nil, "skipped"},
		{"first update", true, 1, 2, nil, "updated"},
		{"later update", true, 2, 3, nil, "updated"},
		{"deletion without version change", true, 2, 2, "2026-11-11T17:00:00Z", "updated"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			rows := sqlmock.NewRows([]string{"central_version", "central_deleted_at"})
			if tt.exists {
				rows.AddRow(tt.storedVersion, nil)
			}
			mock.ExpectQuery("SELECT central_version, central_deleted_at").WithArgs("old-entity").WillReturnRows(rows)
			if tt.want != "skipped" {
				mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			entity := map[string]interface{}{"__id": "old-entity", "Archiver": "yes", "__system": map[string]interface{}{"version": tt.version, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": nil, "deletedAt": tt.deleted}}
			if tt.version > 1 {
				entity["__system"].(map[string]interface{})["updatedAt"] = "2026-11-11T16:00:00Z"
			}
			action, _, _, _, _, err := upsertDatasetEntity(db, "etudes", entity, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if action != tt.want {
				t.Fatalf("action = %s, want %s", action, tt.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDatasetEmptyRunDoesNotWriteDetails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stats, err := syncDatasetEntities(db, 1, 1, "etudes", "etudes", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RowsFetched != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	// No database writes are expected: the historical MAX watermarks remain intact.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadDatasetWatermarksAfterEmptyRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	created := timePtr(t, "2026-11-11T15:48:18.248Z")
	for _, updated := range []interface{}{nil, *timePtr(t, "2026-11-11T16:00:00Z")} {
		mock.ExpectQuery("FROM .*last_successful_datasets_sync").WithArgs(1, "etudes").WillReturnRows(sqlmock.NewRows([]string{"project_id", "object_name", "max_created_at", "max_updated_at", "max_deleted_at"}).AddRow(1, "etudes", *created, updated, nil))
		last, err := getLastSuccessfulDatasetSync(db, 1, "etudes")
		if err != nil {
			t.Fatal(err)
		}
		if last.MaxCreatedAt == nil || !last.MaxCreatedAt.Equal(*created) {
			t.Fatalf("lost creation watermark: %+v", last)
		}
		if (last.MaxUpdatedAt == nil) != (updated == nil) {
			t.Fatalf("unexpected update watermark: %+v", last)
		}
	}
	mock.ExpectQuery("FROM .*last_successful_datasets_sync").WillReturnError(sql.ErrNoRows)
	last, err := getLastSuccessfulDatasetSync(db, 1, "empty")
	if err != nil || last != nil {
		t.Fatalf("last=%+v err=%v", last, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

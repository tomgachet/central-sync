package main

import (
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

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

package main

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSyncSubmissionBatchReportsCommit(t *testing.T) {
	writeErr := errors.New("database write failed")
	for _, test := range []struct {
		name         string
		commitErr    error
		detailErr    error
		wantExitCode int
	}{
		{"commit succeeds", nil, nil, 0},
		{"detail fails after commit", nil, writeErr, 1},
		{"commit fails after insert", writeErr, nil, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT EXISTS.*FROM "central_submissions"\."observations"`).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectExec(`INSERT INTO "central_submissions"\."observations"`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit().WillReturnError(test.commitErr)
			if test.commitErr == nil {
				detail := mock.ExpectExec(`INSERT INTO "central_metadata"\.sync_runs_detail`)
				if test.detailErr != nil {
					detail.WillReturnError(test.detailErr)
				} else {
					detail.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}

			row := &SubmissionRowRef{
				FormTable: FormTable{ODataName: "Submissions", SQLName: "observations", IsRoot: true},
				Row:       map[string]interface{}{"__system": map[string]interface{}{}},
				Shape:     &SubmissionRowShape{Kind: SubmissionTableRoot, RowUUID: "sample"},
			}
			batch := &SubmissionBatch{RootSubmissionUUID: "sample", RootRow: row, Rows: []*SubmissionRowRef{row}}
			stats, committed, err := syncSubmissionBatch(db, nil, 42, 7, "observations", SyncModeAppendOnly, false, false, nil, batch)
			if stats == nil || stats.RowsInserted != 1 {
				t.Fatalf("expected an insert before commit, got %+v (%v)", stats, err)
			}
			if mockErr := mock.ExpectationsWereMet(); mockErr != nil {
				t.Fatal(mockErr)
			}
			var outcome syncOutcome
			outcome.record(committed, err)
			if got := outcome.exitCode(); got != test.wantExitCode {
				t.Fatalf("exit status = %d, want %d (committed = %t, err = %v)", got, test.wantExitCode, committed, err)
			}
		})
	}
}

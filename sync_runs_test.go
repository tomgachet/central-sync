package main

import (
	"strings"
	"testing"
)

func TestFinishSyncRunPersistsFailedRowCount(t *testing.T) {
	executor := &recordingSQLExecutor{}
	err := finishSyncRun(executor, SyncRunFinishParams{
		RunID:        42,
		SyncStatus:   "partial_success",
		RowsFetched:  10,
		RowsInserted: 4,
		RowsUpdated:  2,
		RowsSkipped:  3,
		RowsFailed:   1,
	})
	if err != nil {
		t.Fatalf("finishSyncRun returned error: %v", err)
	}
	if !strings.Contains(executor.query, "rows_failed = $8") {
		t.Fatalf("finish query does not update rows_failed: %s", executor.query)
	}
	if len(executor.args) != 9 || executor.args[7] != 1 {
		t.Fatalf("unexpected finish arguments: %#v", executor.args)
	}
}

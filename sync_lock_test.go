package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeSyncRunLocker struct {
	name        string
	locked      bool
	blockingRun *BlockingSyncRun
	err         error
	unlocks     int
	closes      int
}

func (l *fakeSyncRunLocker) databaseName() string { return l.name }

func (l *fakeSyncRunLocker) tryLock() (bool, *BlockingSyncRun, error) {
	if l.err != nil {
		return false, nil, l.err
	}
	return l.locked, l.blockingRun, nil
}

func (l *fakeSyncRunLocker) unlock() error {
	l.unlocks++
	return nil
}

func (l *fakeSyncRunLocker) close() error {
	l.closes++
	return nil
}

func TestActiveProjectDatabaseNamesReturnsSortedUniqueActiveDatabases(t *testing.T) {
	projects := []ProjectMapping{
		{DatabaseName: "z_db", Datasets: []DatasetMapping{{Name: "species", TableName: "species", Sync: true}}},
		{DatabaseName: "unused_db", Datasets: []DatasetMapping{{Name: "sites", TableName: "sites", Sync: false}}},
		{DatabaseName: "a_db", Forms: []FormMapping{{XMLFormID: "survey", TableName: "survey", Sync: true}}},
		{DatabaseName: "z_db", Forms: []FormMapping{{XMLFormID: "other", TableName: "other", Sync: true}}},
	}

	got := activeProjectDatabaseNames(projects)
	want := []string{"a_db", "z_db"}

	if len(got) != len(want) {
		t.Fatalf("expected %d names, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestAcquireSyncRunLockersSucceedsAndReleasesLocks(t *testing.T) {
	first := &fakeSyncRunLocker{name: "a_db", locked: true}
	second := &fakeSyncRunLocker{name: "b_db", locked: true}

	set, err := acquireSyncRunLockers([]syncRunLocker{first, second})
	if err != nil {
		t.Fatalf("acquireSyncRunLockers returned error: %v", err)
	}

	if err := set.Release(); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	if first.unlocks != 1 || second.unlocks != 1 {
		t.Fatalf("expected both locks to be released once, got first=%d second=%d", first.unlocks, second.unlocks)
	}
	if first.closes != 1 || second.closes != 1 {
		t.Fatalf("expected both lockers to be closed once, got first=%d second=%d", first.closes, second.closes)
	}
}

func TestAcquireSyncRunLockersFailsWhenAnotherRunIsActive(t *testing.T) {
	first := &fakeSyncRunLocker{name: "a_db", locked: true}
	second := &fakeSyncRunLocker{
		name:   "b_db",
		locked: false,
		blockingRun: &BlockingSyncRun{
			RunID:        42,
			ProjectID:    7,
			ObjectType:   "form",
			ObjectName:   "site_visit",
			SQLTableName: "site_visit",
			StartedAt:    "2026-07-03 12:00:00+00",
		},
	}

	_, err := acquireSyncRunLockers([]syncRunLocker{first, second})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "run_id=42") || !strings.Contains(err.Error(), "database b_db") {
		t.Fatalf("expected blocking run details in error, got %v", err)
	}

	if first.unlocks != 1 || first.closes != 1 {
		t.Fatalf("expected previously acquired locker to be cleaned up, got unlocks=%d closes=%d", first.unlocks, first.closes)
	}
	if second.unlocks != 1 || second.closes != 1 {
		t.Fatalf("expected blocked locker to be closed, got unlocks=%d closes=%d", second.unlocks, second.closes)
	}
}

func TestAcquireSyncRunLockersCleansUpAfterLockError(t *testing.T) {
	first := &fakeSyncRunLocker{name: "a_db", locked: true}
	second := &fakeSyncRunLocker{name: "b_db", err: errors.New("database failed")}

	_, err := acquireSyncRunLockers([]syncRunLocker{first, second})
	if err == nil {
		t.Fatalf("expected error")
	}

	if first.unlocks != 1 || first.closes != 1 {
		t.Fatalf("expected previously acquired locker to be cleaned up, got unlocks=%d closes=%d", first.unlocks, first.closes)
	}
	if second.unlocks != 1 || second.closes != 1 {
		t.Fatalf("expected failing locker to be closed, got unlocks=%d closes=%d", second.unlocks, second.closes)
	}
}

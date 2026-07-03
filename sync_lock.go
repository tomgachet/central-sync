package main

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

const (
	syncRunLockKey1 int64 = 1961790019
	syncRunLockKey2 int64 = 20260703
)

type SyncRunLockSet struct {
	lockers []syncRunLocker
}

type syncRunLocker interface {
	databaseName() string
	tryLock() (bool, *BlockingSyncRun, error)
	unlock() error
	close() error
}

type BlockingSyncRun struct {
	RunID        int64
	ProjectID    int
	FormXMLID    *string
	ObjectType   string
	ObjectName   string
	SQLTableName string
	StartedAt    string
}

type postgresSyncRunLocker struct {
	database string
	db       *sql.DB
	conn     *sql.Conn
}

func acquireSyncRunLocks(projects []ProjectMapping) (*SyncRunLockSet, error) {
	databaseNames := activeProjectDatabaseNames(projects)
	if len(databaseNames) == 0 {
		logInfo("[SYNC_LOCK] no active project database requires a sync lock")
	}

	lockers := make([]syncRunLocker, 0, len(databaseNames))

	for _, databaseName := range databaseNames {
		locker, err := newPostgresSyncRunLocker(databaseName)
		if err != nil {
			closeSyncRunLockers(lockers)
			return nil, err
		}
		lockers = append(lockers, locker)
	}

	return acquireSyncRunLockers(lockers)
}

func activeProjectDatabaseNames(projects []ProjectMapping) []string {
	seen := make(map[string]bool)
	var names []string

	for _, project := range projects {
		if len(getDatasetsToSync(project)) == 0 && len(getFormsToSync(project)) == 0 {
			continue
		}

		name := strings.TrimSpace(project.DatabaseName)
		if name == "" || seen[name] {
			continue
		}

		seen[name] = true
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func newPostgresSyncRunLocker(databaseName string) (*postgresSyncRunLocker, error) {
	db, err := connectProjectDatabase(databaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database %s for sync lock: %w", databaseName, err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to reserve database connection for sync lock on %s: %w", databaseName, err)
	}

	return &postgresSyncRunLocker{
		database: databaseName,
		db:       db,
		conn:     conn,
	}, nil
}

func acquireSyncRunLockers(lockers []syncRunLocker) (*SyncRunLockSet, error) {
	acquired := make([]syncRunLocker, 0, len(lockers))

	for _, locker := range lockers {
		logInfo("[SYNC_LOCK] database=%s checking concurrent sync lock", locker.databaseName())

		locked, blockingRun, err := locker.tryLock()
		if err != nil {
			closeSyncRunLockers(append(acquired, locker))
			return nil, err
		}

		if !locked {
			err := buildSyncRunAlreadyRunningError(locker.databaseName(), blockingRun)
			logWarn("[SYNC_LOCK] database=%s concurrent sync blocked: %v", locker.databaseName(), err)
			closeSyncRunLockers(append(acquired, locker))
			return nil, err
		}

		logInfo("[SYNC_LOCK] database=%s lock acquired", locker.databaseName())
		acquired = append(acquired, locker)
	}

	return &SyncRunLockSet{lockers: acquired}, nil
}

func (l *postgresSyncRunLocker) databaseName() string {
	return l.database
}

func (l *postgresSyncRunLocker) tryLock() (bool, *BlockingSyncRun, error) {
	var locked bool
	err := l.conn.QueryRowContext(
		context.Background(),
		`SELECT pg_try_advisory_lock($1, $2)`,
		syncRunLockKey1,
		syncRunLockKey2,
	).Scan(&locked)
	if err != nil {
		return false, nil, fmt.Errorf("failed to acquire sync lock on database %s: %w", l.database, err)
	}

	if locked {
		return true, nil, nil
	}

	blockingRun, err := l.findBlockingSyncRun()
	if err != nil {
		return false, nil, err
	}

	return false, blockingRun, nil
}

func (l *postgresSyncRunLocker) findBlockingSyncRun() (*BlockingSyncRun, error) {
	query := fmt.Sprintf(`
		SELECT
			run_id,
			project_id,
			form_xml_id,
			object_type,
			object_name,
			COALESCE(sql_table_name, ''),
			started_at::TEXT
		FROM %s.sync_runs
		WHERE sync_status = 'running'
		ORDER BY started_at DESC, run_id DESC
		LIMIT 1
	`, quoteIdentifier(syncMetadataSchema))

	var run BlockingSyncRun
	var formXMLID sql.NullString
	err := l.conn.QueryRowContext(context.Background(), query).Scan(
		&run.RunID,
		&run.ProjectID,
		&formXMLID,
		&run.ObjectType,
		&run.ObjectName,
		&run.SQLTableName,
		&run.StartedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to inspect running sync on database %s: %w", l.database, err)
	}

	if formXMLID.Valid {
		run.FormXMLID = &formXMLID.String
	}

	return &run, nil
}

func (l *postgresSyncRunLocker) unlock() error {
	var unlocked bool
	err := l.conn.QueryRowContext(
		context.Background(),
		`SELECT pg_advisory_unlock($1, $2)`,
		syncRunLockKey1,
		syncRunLockKey2,
	).Scan(&unlocked)
	if err != nil {
		return fmt.Errorf("failed to release sync lock on database %s: %w", l.database, err)
	}
	if !unlocked {
		return fmt.Errorf("sync lock on database %s was not held by this connection", l.database)
	}
	return nil
}

func (l *postgresSyncRunLocker) close() error {
	var firstErr error

	if l.conn != nil {
		if err := l.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if l.db != nil {
		if err := l.db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (s *SyncRunLockSet) Release() error {
	if s == nil {
		return nil
	}

	var firstErr error
	for i := len(s.lockers) - 1; i >= 0; i-- {
		locker := s.lockers[i]
		logInfo("[SYNC_LOCK] database=%s releasing lock", locker.databaseName())
		if err := locker.unlock(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := locker.close(); err != nil && firstErr == nil {
			firstErr = err
		}
		if firstErr == nil {
			logInfo("[SYNC_LOCK] database=%s lock released", locker.databaseName())
		}
	}
	s.lockers = nil

	return firstErr
}

func closeSyncRunLockers(lockers []syncRunLocker) {
	for i := len(lockers) - 1; i >= 0; i-- {
		_ = lockers[i].unlock()
		_ = lockers[i].close()
	}
}

func buildSyncRunAlreadyRunningError(databaseName string, blockingRun *BlockingSyncRun) error {
	if blockingRun == nil {
		return fmt.Errorf("another central-sync run is already in progress for database %s", databaseName)
	}

	target := blockingRun.ObjectName
	if blockingRun.SQLTableName != "" {
		target = fmt.Sprintf("%s/%s", blockingRun.ObjectName, blockingRun.SQLTableName)
	}

	return fmt.Errorf(
		"another central-sync run is already in progress for database %s: run_id=%d project_id=%d object_type=%s object=%s started_at=%s",
		databaseName,
		blockingRun.RunID,
		blockingRun.ProjectID,
		blockingRun.ObjectType,
		target,
		blockingRun.StartedAt,
	)
}

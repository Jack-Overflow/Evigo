package database

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Jack-Overflow/Evigo/internal/database/queries"
	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
)

//go:embed schema/003_pending.sql
var pendingSchema string

//go:embed schema/004_history.sql
var historySchema string

var schemaSteps = []struct {
	version int
	sql     string
}{
	{3, pendingSchema},
	{4, historySchema},
}

//go:embed journal.sql
var journal string

const (
	applicationID           = 0x45564731
	schemaVersion           = 4
	oldestUpgradableVersion = 3
	keptHistoryEvents       = 10000
	operationTimeout        = 3 * time.Second
)

var errNotSQLite = errors.New("state file is not a SQLite database; preserve it and restore a valid backup")

type DB struct {
	sqlite *sql.DB
	lock   *os.File
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("state directory: %w", err)
	}

	lock, err := acquireLock(path + ".lock")
	if err != nil {
		return nil, err
	}

	sqliteDatabase, err := openDatabase(path)
	if err != nil {
		releaseLock(lock)

		return nil, err
	}

	return &DB{sqlite: sqliteDatabase, lock: lock}, nil
}

func (db *DB) Close() error {
	err := db.sqlite.Close()
	releaseLock(db.lock)

	return err
}

func (db *DB) LoadPending(ctx context.Context) (map[string]recovery.Progress, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	rows, err := queries.New(db.sqlite).ListPending(ctx)
	if err != nil {
		return nil, fmt.Errorf("load pending devices: %w", err)
	}

	pending := make(map[string]recovery.Progress, len(rows))
	for _, row := range rows {
		progress := recovery.Progress{
			Attempts:  int(row.Attempts),
			TimeLeft:  time.Duration(row.TimeLeftNs),
			GraceLeft: time.Duration(row.GraceLeftNs),
		}
		if row.LastAttemptNs != 0 {
			progress.LastAttempt = time.Unix(0, row.LastAttemptNs)
		}

		pending[row.DeviceID] = progress
	}

	return pending, nil
}

func (db *DB) SavePending(ctx context.Context, pending map[string]recovery.Progress) error {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	transaction, err := db.sqlite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save pending devices: %w", err)
	}

	defer transaction.Rollback()
	statements := queries.New(transaction)
	if err = statements.DeletePending(ctx); err != nil {
		return fmt.Errorf("save pending devices: %w", err)
	}

	for deviceID, progress := range pending {
		row := queries.InsertPendingParams{
			DeviceID:    deviceID,
			Attempts:    int64(progress.Attempts),
			TimeLeftNs:  int64(progress.TimeLeft),
			GraceLeftNs: int64(progress.GraceLeft),
		}
		if !progress.LastAttempt.IsZero() {
			row.LastAttemptNs = progress.LastAttempt.UnixNano()
		}

		if err = statements.InsertPending(ctx, row); err != nil {
			return fmt.Errorf("save pending devices: %w", err)
		}
	}

	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("save pending devices: %w", err)
	}

	return nil
}

func (db *DB) AppendHistory(ctx context.Context, events []history.Event) error {
	if len(events) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	transaction, err := db.sqlite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("append history: %w", err)
	}

	defer transaction.Rollback()
	statements := queries.New(transaction)
	for _, event := range events {
		details, err := json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("append history: %w", err)
		}

		row := queries.InsertHistoryParams{
			AtNs:     event.At.UnixNano(),
			Kind:     string(event.Kind),
			DeviceID: event.DeviceID,
			Details:  string(details),
		}
		if err = statements.InsertHistory(ctx, row); err != nil {
			return fmt.Errorf("append history: %w", err)
		}
	}

	if err = statements.TrimHistory(ctx, keptHistoryEvents); err != nil {
		return fmt.Errorf("append history: %w", err)
	}

	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("append history: %w", err)
	}

	return nil
}

func (db *DB) History(ctx context.Context, limit int, beforeID int64) (history.Page, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	statements := queries.New(db.sqlite)
	var rows []queries.History
	var err error
	if beforeID == 0 {
		rows, err = statements.ListLatestHistory(ctx, int64(limit)+1)
	} else {
		rows, err = statements.ListHistoryBefore(ctx, queries.ListHistoryBeforeParams{BeforeID: beforeID, PageSize: int64(limit) + 1})
	}

	if err != nil {
		return history.Page{}, fmt.Errorf("load history: %w", err)
	}

	var page history.Page
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextBefore = rows[limit-1].ID
	}

	for _, row := range rows {
		event := history.Event{
			ID:       row.ID,
			At:       time.Unix(0, row.AtNs),
			Kind:     history.Kind(row.Kind),
			DeviceID: row.DeviceID,
		}
		if err = json.Unmarshal([]byte(row.Details), &event.Details); err != nil {
			return history.Page{}, fmt.Errorf("load history: event %d has invalid details", row.ID)
		}

		page.Events = append(page.Events, event)
	}

	return page, nil
}

func acquireLock(path string) (*os.File, error) {
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()

		return nil, errors.New("state is locked by another Evigo process or the filesystem does not support locking")
	}

	return lock, nil
}

func releaseLock(lock *os.File) {
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	lock.Close()
}

func openDatabase(path string) (*sql.DB, error) {
	if err := requireSQLiteOrEmpty(path); err != nil {
		return nil, err
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	address := url.URL{Scheme: "file", Path: absolute, RawQuery: "_pragma=busy_timeout(1000)&_pragma=synchronous(FULL)"}
	sqliteDatabase, err := sql.Open("sqlite", address.String())
	if err != nil {
		return nil, err
	}

	sqliteDatabase.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	if err = prepare(ctx, sqliteDatabase); err != nil {
		sqliteDatabase.Close()

		return nil, err
	}

	if _, err = sqliteDatabase.ExecContext(ctx, journal); err != nil {
		sqliteDatabase.Close()

		return nil, err
	}

	return sqliteDatabase, nil
}

func requireSQLiteOrEmpty(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}

	defer file.Close()
	header := make([]byte, 16)
	read, err := file.ReadAt(header, 0)
	if read == 0 {
		return nil
	}

	if err != nil || !bytes.Equal(header, []byte("SQLite format 3\x00")) {
		return errNotSQLite
	}

	return nil
}

func prepare(ctx context.Context, sqliteDatabase *sql.DB) error {
	var storedApplication, storedVersion, tables int
	err := sqliteDatabase.QueryRowContext(ctx, `SELECT (SELECT application_id FROM pragma_application_id),
 (SELECT user_version FROM pragma_user_version),
 (SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%')`).Scan(&storedApplication, &storedVersion, &tables)
	switch {
	case err != nil:
		return fmt.Errorf("cannot read SQLite database: %w", err)
	case storedApplication == 0 && storedVersion == 0 && tables == 0:
		return applySchema(ctx, sqliteDatabase, 0)
	case storedApplication != applicationID || storedVersion < oldestUpgradableVersion || storedVersion > schemaVersion:
		return errors.New("incompatible state database; move it aside to start fresh")
	case !readable(ctx, sqliteDatabase, storedVersion):
		return errors.New("damaged state database; preserve it and restore a valid backup, or move it aside to start fresh")
	case storedVersion < schemaVersion:
		return applySchema(ctx, sqliteDatabase, storedVersion)
	}

	return nil
}

func readable(ctx context.Context, sqliteDatabase *sql.DB, version int) bool {
	var integrity string
	if sqliteDatabase.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity) != nil || integrity != "ok" {
		return false
	}

	if _, err := queries.New(sqliteDatabase).ListPending(ctx); err != nil {
		return false
	}

	if version >= 4 {
		rows, err := sqliteDatabase.QueryContext(ctx, "SELECT id, at_ns, kind, device_id, details FROM history LIMIT 0")
		if err != nil {
			return false
		}

		rows.Close()
	}

	return true
}

func applySchema(ctx context.Context, sqliteDatabase *sql.DB, fromVersion int) error {
	transaction, err := sqliteDatabase.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer transaction.Rollback()
	for _, step := range schemaSteps {
		if step.version <= fromVersion {
			continue
		}

		if _, err = transaction.ExecContext(ctx, step.sql); err != nil {
			return fmt.Errorf("upgrade state database to version %d: %w", step.version, err)
		}
	}

	versioning := fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=%d", applicationID, schemaVersion)
	if _, err = transaction.ExecContext(ctx, versioning); err != nil {
		return err
	}

	return transaction.Commit()
}

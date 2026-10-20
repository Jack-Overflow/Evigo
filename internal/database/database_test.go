package database

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/recovery"
)

func TestGivenPendingDevicesWhenSavedAndReopenedThenProgressIsRestored(t *testing.T) {
	// Given a store holding two pending devices.
	path := filepath.Join(t.TempDir(), "state", "evigo.db")
	opened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]recovery.Progress{
		"server":  {Attempts: 2, LastAttempt: time.Unix(0, 1_700_000_000_000_000_000), TimeLeft: 9 * time.Minute},
		"storage": {TimeLeft: 10 * time.Minute},
	}
	if err = opened.SavePending(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	opened.Close()

	// When the store is reopened and loaded.
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.LoadPending(context.Background())
	// Then every field of every pending device survives.
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d devices", len(got))
	}
	for deviceID, progress := range want {
		loaded := got[deviceID]
		if loaded.Attempts != progress.Attempts || !loaded.LastAttempt.Equal(progress.LastAttempt) ||
			loaded.TimeLeft != progress.TimeLeft {
			t.Fatalf("%s: got %+v, want %+v", deviceID, loaded, progress)
		}
	}
}

func TestGivenSavedDevicesWhenEmptySetSavedThenNothingIsPending(t *testing.T) {
	// Given a store with one pending device.
	opened, err := Open(filepath.Join(t.TempDir(), "evigo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	opened.SavePending(context.Background(), map[string]recovery.Progress{"server": {TimeLeft: time.Minute}})

	// When recovery finishes and an empty set is saved.
	if err = opened.SavePending(context.Background(), map[string]recovery.Progress{}); err != nil {
		t.Fatal(err)
	}

	// Then loading returns no pending devices.
	got, err := opened.LoadPending(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestGivenOpenStoreWhenSecondProcessOpensThenItIsRefused(t *testing.T) {
	// Given a store already open.
	path := filepath.Join(t.TempDir(), "evigo.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	// When the same path is opened again.
	_, err = Open(path)

	// Then the lock refuses it.
	if err == nil {
		t.Fatal("second open succeeded")
	}
}

func TestGivenNonSQLiteFileWhenOpenedThenItIsRefusedAndPreserved(t *testing.T) {
	// Given a state path holding something that is not SQLite.
	path := filepath.Join(t.TempDir(), "evigo.db")
	os.WriteFile(path, []byte("not a database"), 0o600)

	// When the store is opened.
	_, err := Open(path)

	// Then opening fails and the file is left untouched.
	if err == nil {
		t.Fatal("opened a non-SQLite file")
	}
	if contents, _ := os.ReadFile(path); string(contents) != "not a database" {
		t.Fatal("file was modified")
	}
}

func TestGivenDatabaseFromAnotherSchemaWhenOpenedThenItIsRefused(t *testing.T) {
	// Given a SQLite file with an older schema version.
	path := filepath.Join(t.TempDir(), "evigo.db")
	opened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	opened.sqlite.Exec("PRAGMA user_version=2")
	opened.Close()

	// When the store is opened.
	_, err = Open(path)

	// Then it refuses rather than reusing incompatible state.
	if err == nil {
		t.Fatal("opened an incompatible schema")
	}
}

func TestGivenIncompatibleSQLiteFileWhenOpenedThenItsBytesAndJournalModeArePreserved(t *testing.T) {
	for _, kind := range []string{"old Evigo schema", "foreign database"} {
		t.Run(kind, func(t *testing.T) {
			// Given a rollback-journal SQLite file from an old Evigo schema or another application.
			path := filepath.Join(t.TempDir(), "state.db")
			original, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = original.sqlite.Exec("PRAGMA journal_mode=DELETE; PRAGMA user_version=2;"); err != nil {
				t.Fatal(err)
			}
			if kind == "foreign database" {
				if _, err = original.sqlite.Exec("PRAGMA application_id=123;"); err != nil {
					t.Fatal(err)
				}
			}
			if err = original.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			// When Evigo opens it.
			opened, err := Open(path)

			// Then it is refused, and its bytes and journal mode are unchanged.
			if err == nil {
				opened.Close()
				t.Fatal("incompatible database accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("refused database was modified")
			}
			if before[18] != 1 || before[19] != 1 {
				t.Fatal("fixture was not in rollback journal mode")
			}
		})
	}
}

func openTemporary(t *testing.T) *DB {
	t.Helper()
	opened, err := Open(filepath.Join(t.TempDir(), "evigo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { opened.Close() })

	return opened
}

func TestGivenSavedHistoryWhenPagedThenEventsComeNewestFirstWithACursor(t *testing.T) {
	// Given five saved events.
	opened := openTemporary(t)
	start := time.Unix(1_700_000_000, 0)
	var events []history.Event
	for index := range 5 {
		events = append(events, history.Event{
			At:       start.Add(time.Duration(index) * time.Second),
			Kind:     history.WakeSent,
			DeviceID: "server",
			Details:  history.Details{Source: history.Automatic, Attempt: index + 1},
		})
	}
	if err := opened.AppendHistory(context.Background(), events); err != nil {
		t.Fatal(err)
	}

	// When the first two pages of two events are read.
	first, err := opened.History(context.Background(), 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := opened.History(context.Background(), 2, first.NextBefore)
	if err != nil {
		t.Fatal(err)
	}
	last, err := opened.History(context.Background(), 2, second.NextBefore)
	if err != nil {
		t.Fatal(err)
	}

	// Then events arrive newest first, with all details, and the last page has no cursor.
	if first.Events[0].Details.Attempt != 5 || first.Events[1].Details.Attempt != 4 || second.Events[0].Details.Attempt != 3 {
		t.Fatalf("pages %+v %+v", first.Events, second.Events)
	}
	if !first.Events[0].At.Equal(start.Add(4*time.Second)) || first.Events[0].DeviceID != "server" || first.Events[0].Details.Source != history.Automatic {
		t.Fatalf("event %+v", first.Events[0])
	}
	if len(last.Events) != 1 || last.NextBefore != 0 {
		t.Fatalf("last page %+v", last)
	}
}

func TestGivenMoreEventsThanKeptWhenAppendedThenOnlyTheNewestRemain(t *testing.T) {
	// Given a store with more events than it keeps.
	opened := openTemporary(t)
	events := make([]history.Event, keptHistoryEvents+5)
	for index := range events {
		events[index] = history.Event{At: time.Unix(int64(index), 0), Kind: history.UPSStatus}
	}

	// When they are appended.
	if err := opened.AppendHistory(context.Background(), events); err != nil {
		t.Fatal(err)
	}

	// Then the oldest are trimmed.
	var count int
	var oldest int64
	opened.sqlite.QueryRow("SELECT count(*), min(at_ns) FROM history").Scan(&count, &oldest)
	if count != keptHistoryEvents || oldest != time.Unix(5, 0).UnixNano() {
		t.Fatalf("kept %d, oldest %d", count, oldest)
	}
}

func TestGivenVersionThreeDatabaseWhenOpenedThenItIsUpgradedAndPendingProgressIsKept(t *testing.T) {
	// Given a database exactly as Evigo 0.1.0 created it, with a pending device.
	path := filepath.Join(t.TempDir(), "evigo.db")
	address := url.URL{Scheme: "file", Path: path}
	legacy, err := sql.Open("sqlite", address.String())
	if err != nil {
		t.Fatal(err)
	}
	setup := pendingSchema + fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=3;", applicationID) +
		"INSERT INTO pending VALUES ('server', 1, 1700000000000000000, 60000000000, 0);"
	if _, err = legacy.Exec(setup); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	// When the current version opens it.
	opened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()

	// Then pending progress is kept, history works, and the version is current.
	pending, err := opened.LoadPending(context.Background())
	if err != nil || pending["server"].Attempts != 1 || pending["server"].TimeLeft != time.Minute {
		t.Fatalf("pending %+v, err %v", pending, err)
	}
	if err = opened.AppendHistory(context.Background(), []history.Event{{At: time.Now(), Kind: history.Started}}); err != nil {
		t.Fatal(err)
	}
	var version int
	opened.sqlite.QueryRow("PRAGMA user_version").Scan(&version)
	if version != schemaVersion {
		t.Fatalf("version %d", version)
	}
}

func TestGivenDatabaseFromANewerEvigoWhenOpenedThenItIsRefused(t *testing.T) {
	// Given a database written by a future schema version.
	path := filepath.Join(t.TempDir(), "evigo.db")
	opened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	opened.sqlite.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion+1))
	opened.Close()

	// When this version opens it.
	_, err = Open(path)

	// Then it refuses instead of guessing at the newer layout.
	if err == nil {
		t.Fatal("opened a newer schema")
	}
}

func TestGivenVersionThreeDatabaseWithDamagedPendingTableWhenOpenedThenItIsRefusedUnchanged(t *testing.T) {
	for name, setup := range map[string]string{
		"missing table":  "",
		"wrong columns":  "CREATE TABLE pending (device TEXT);",
		"unreadable row": pendingSchema + "INSERT INTO pending VALUES ('server', 'many', 0, 0, 0);",
	} {
		t.Run(name, func(t *testing.T) {
			// Given a file claiming to be an Evigo 0.1.0 database, with a damaged pending table.
			path := filepath.Join(t.TempDir(), "evigo.db")
			address := url.URL{Scheme: "file", Path: path}
			legacy, err := sql.Open("sqlite", address.String())
			if err != nil {
				t.Fatal(err)
			}
			versioning := fmt.Sprintf("PRAGMA journal_mode=DELETE; PRAGMA application_id=%d; PRAGMA user_version=3;", applicationID)
			if _, err = legacy.Exec(versioning + "CREATE TABLE marker (id INTEGER);" + setup); err != nil {
				t.Fatal(err)
			}
			legacy.Close()
			before, _ := os.ReadFile(path)

			// When the current version opens it.
			_, err = Open(path)

			// Then it is refused and its bytes are unchanged, so it was neither upgraded nor switched to WAL.
			if err == nil {
				t.Fatal("opened a damaged database")
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
				t.Fatal("damaged database was modified")
			}
		})
	}
}

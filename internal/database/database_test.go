package database

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"evigo/internal/recovery"
)

func TestGivenPendingDevicesWhenSavedAndReopenedThenProgressIsRestored(t *testing.T) {
	// Given a store holding two pending devices.
	path := filepath.Join(t.TempDir(), "state", "evigo.db")
	opened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]recovery.Progress{
		"server":  {Attempts: 2, LastAttempt: time.Unix(0, 1_700_000_000_000_000_000), TimeLeft: 9 * time.Minute, GraceLeft: 0},
		"storage": {TimeLeft: 10 * time.Minute, GraceLeft: 2 * time.Minute},
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
			loaded.TimeLeft != progress.TimeLeft || loaded.GraceLeft != progress.GraceLeft {
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
			// Given
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

			// When
			opened, err := Open(path)

			// Then
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

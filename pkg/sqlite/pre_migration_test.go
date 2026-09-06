package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// newDatabaseAtVersion builds a real, file-backed database migrated to
// exactly the given schema version (not all the way to appSchemaVersion),
// mirroring an existing installation that has not yet completed its final
// migration. This is the permanent regression fixture for P1A-F020: a
// database in this state must never cause a caller to dereference a nil
// readDB/writeDB, since Open() intentionally leaves them unset when a
// migration is still needed (see Database.Open's needsMigration branch).
func newDatabaseAtVersion(t *testing.T, version uint) *Database {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pre-migration.sqlite")
	db := NewDatabase()
	db.dbPath = path

	migrator, err := NewMigrator(db)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	for step := uint(1); step <= version; step++ {
		if err := migrator.RunMigration(context.Background(), step); err != nil {
			t.Fatalf("running migration to version %d: %v", step, err)
		}
	}
	return db
}

func TestOpenReportsMigrationNeededWithoutOpeningConnections(t *testing.T) {
	if appSchemaVersion < 2 {
		t.Skip("appSchemaVersion too low to construct a pending-migration fixture")
	}
	pendingVersionDB := newDatabaseAtVersion(t, appSchemaVersion-1)
	path := pendingVersionDB.dbPath

	// A fresh Database instance, matching a real process restart against an
	// existing on-disk database that still needs its final migration.
	db := NewDatabase()
	err := db.Open(path)
	var migrationNeeded *MigrationNeededError
	if !errors.As(err, &migrationNeeded) {
		t.Fatalf("Open on a pending-migration database: err=%v, want *MigrationNeededError", err)
	}
	if migrationNeeded.CurrentSchemaVersion != appSchemaVersion-1 || migrationNeeded.RequiredSchemaVersion != appSchemaVersion {
		t.Fatalf("MigrationNeededError versions: got current=%d required=%d, want current=%d required=%d",
			migrationNeeded.CurrentSchemaVersion, migrationNeeded.RequiredSchemaVersion, appSchemaVersion-1, appSchemaVersion)
	}

	if readyErr := db.Ready(); !errors.Is(readyErr, ErrDatabaseNotInitialized) {
		t.Fatalf("Ready() on an unopened pending-migration database: err=%v, want ErrDatabaseNotInitialized", readyErr)
	}
}

func TestBeginOnUnopenedDatabaseFailsSafelyNoPanic(t *testing.T) {
	if appSchemaVersion < 2 {
		t.Skip("appSchemaVersion too low to construct a pending-migration fixture")
	}
	pendingVersionDB := newDatabaseAtVersion(t, appSchemaVersion-1)
	path := pendingVersionDB.dbPath

	db := NewDatabase()
	if err := db.Open(path); err == nil {
		t.Fatal("expected Open to report the pending migration")
	}
	// db.readDB/writeDB are deliberately nil here — this is the exact P1A-F020
	// reproduction. Begin must return a typed error, never panic.
	for _, writable := range []bool{false, true} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Begin(writable=%v) on an unopened database panicked: %v (this is the exact P1A-F020 crash — must return an error instead)", writable, r)
				}
			}()
			_, err := db.Begin(context.Background(), writable)
			var unavailable *DatabaseUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("Begin(writable=%v) err=%v, want *DatabaseUnavailableError", writable, err)
			}
			if !errors.Is(unavailable, ErrDatabaseNotInitialized) {
				t.Fatalf("Begin(writable=%v) error does not unwrap to ErrDatabaseNotInitialized: %v", writable, unavailable)
			}
		}()
	}
}

func TestWithDatabaseOnUnopenedDatabaseFailsSafelyNoPanic(t *testing.T) {
	if appSchemaVersion < 2 {
		t.Skip("appSchemaVersion too low to construct a pending-migration fixture")
	}
	pendingVersionDB := newDatabaseAtVersion(t, appSchemaVersion-1)
	path := pendingVersionDB.dbPath

	db := NewDatabase()
	if err := db.Open(path); err == nil {
		t.Fatal("expected Open to report the pending migration")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WithDatabase on an unopened database panicked: %v", r)
		}
	}()
	_, err := db.WithDatabase(context.Background())
	var unavailable *DatabaseUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("WithDatabase err=%v, want *DatabaseUnavailableError", err)
	}
}

func TestBeginOnFullyMigratedDatabaseStillWorks(t *testing.T) {
	// Regression guard for the fix itself: the new Ready() check must not
	// break the ordinary, fully-migrated case.
	path := filepath.Join(t.TempDir(), "fully-migrated.sqlite")
	db := NewDatabase()
	if err := db.Open(path); err != nil {
		t.Fatalf("Open on a fresh database: %v", err)
	}
	defer db.Close()

	ctx, err := db.Begin(context.Background(), false)
	if err != nil {
		t.Fatalf("Begin on a fully-migrated, opened database: %v", err)
	}
	if err := db.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}

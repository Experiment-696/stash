package task

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/sqlite"
)

type migrationPathTestConfig struct {
	backupPath  string
	defaultPath string
}

func (c migrationPathTestConfig) GetBackupDirectoryPath() string {
	return c.backupPath
}

func (c migrationPathTestConfig) GetBackupDirectoryPathOrDefault() string {
	return c.defaultPath
}

func TestMigrationBackupPathUsesResolvedDatabaseDirectory(t *testing.T) {
	databaseDir := t.TempDir()
	database := sqlite.NewDatabase()
	if err := database.Open(filepath.Join(databaseDir, "bind-mounted.sqlite")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	got := migrationBackupPath(database, migrationPathTestConfig{defaultPath: t.TempDir()}, "")
	if filepath.Dir(got) != databaseDir {
		t.Fatalf("backup path %q is outside resolved database directory %q", got, databaseDir)
	}
}

func openFixtureDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func authoritativeMigrationState(t *testing.T, path string) (version int, dirty bool, completedTables int) {
	t.Helper()
	db := openFixtureDatabase(t, path)
	defer db.Close()
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN
		('cam_completed_recording_imports','cam_completed_recording_audits')`).Scan(&completedTables); err != nil {
		t.Fatal(err)
	}
	return version, dirty, completedTables
}

func TestMigrateJobBindMountedFailureRestoreAndBoundedRetry(t *testing.T) {
	bindRoot := os.Getenv("STASH_TEST_BIND_MOUNT_DIR")
	if bindRoot == "" {
		t.Skip("set STASH_TEST_BIND_MOUNT_DIR to a container bind mount")
	}
	databaseDir, err := os.MkdirTemp(bindRoot, "migration-retry-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(databaseDir) })
	databasePath := filepath.Join(databaseDir, "stash-go.sqlite")

	seed := sqlite.NewDatabase()
	if err := seed.Open(databasePath); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	raw := openFixtureDatabase(t, databasePath)
	if _, err := raw.Exec(`
		PRAGMA foreign_keys=OFF;
		DROP TABLE cam_completed_recording_imports;
		DROP TABLE cam_completed_recording_audits;
		UPDATE schema_migrations SET version=93, dirty=0;
		CREATE TABLE cam_completed_recording_imports (collision integer);
	`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database := sqlite.NewDatabase()
	var migrationNeeded *sqlite.MigrationNeededError
	if err := database.Open(databasePath); !errors.As(err, &migrationNeeded) {
		t.Fatalf("opening v93 fixture: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	config := migrationPathTestConfig{defaultPath: t.TempDir()}
	migrate := &MigrateJob{Config: config, Database: database}

	err = migrate.Execute(context.Background(), &job.Progress{})
	if err == nil || !strings.Contains(err.Error(), "table cam_completed_recording_imports already exists") {
		t.Fatalf("injected migration failure: %v", err)
	}
	version, dirty, tables := authoritativeMigrationState(t, databasePath)
	if version != 93 || dirty || tables != 1 {
		t.Fatalf("authoritative database after restore: version=%d dirty=%v completed_tables=%d, want 93 false 1", version, dirty, tables)
	}
	if database.Version() != 93 {
		t.Fatalf("live database version after restore=%d want 93", database.Version())
	}
	if readyErr := database.Ready(); !errors.Is(readyErr, sqlite.ErrDatabaseNotInitialized) {
		t.Fatalf("restored v93 database should remain migration-only: %v", readyErr)
	}

	raw = openFixtureDatabase(t, databasePath)
	if _, err := raw.Exec(`DROP TABLE cam_completed_recording_imports`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Execute(context.Background(), &job.Progress{}); err != nil {
		t.Fatalf("bounded retry: %v", err)
	}
	version, dirty, tables = authoritativeMigrationState(t, databasePath)
	if version != 94 || dirty || tables != 2 {
		t.Fatalf("authoritative database after retry: version=%d dirty=%v completed_tables=%d, want 94 false 2", version, dirty, tables)
	}
	if database.Version() != 94 {
		t.Fatalf("live database version after retry=%d want 94", database.Version())
	}
	if err := database.Ready(); err != nil {
		t.Fatalf("live database was not reinitialised after retry: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(databaseDir, "stash-go.sqlite.93.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("migration backups were not consumed: %v", matches)
	}
}

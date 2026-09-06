package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func restoreTestDatabaseDir(t *testing.T) string {
	t.Helper()
	bindRoot := os.Getenv("STASH_TEST_BIND_MOUNT_DIR")
	if bindRoot == "" {
		return t.TempDir()
	}
	dir, err := os.MkdirTemp(bindRoot, "database-restore-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func requireCrossMountBoundary(t *testing.T, sourceDir, destinationDir string) {
	t.Helper()
	if os.Getenv("STASH_TEST_BIND_MOUNT_DIR") == "" {
		return
	}
	source := filepath.Join(sourceDir, "cross-mount-probe")
	destination := filepath.Join(destinationDir, "cross-mount-probe")
	if err := os.WriteFile(source, []byte("probe"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(source)
		_ = os.Remove(destination)
	})
	if err := os.Rename(source, destination); err == nil {
		t.Fatal("test database directory is not separated from the backup directory by a mount boundary")
	}
}

func TestRestoreFromBackupReplacesLiveDatabaseAndReinitialises(t *testing.T) {
	databaseDir := restoreTestDatabaseDir(t)
	backupDir := t.TempDir()
	requireCrossMountBoundary(t, backupDir, databaseDir)
	databasePath := filepath.Join(databaseDir, "bind-mounted.sqlite")
	backupPath := filepath.Join(backupDir, "migration-backup.sqlite")

	database := NewDatabase()
	if err := database.Open(databasePath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := os.Chmod(databasePath, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := database.Backup(backupPath); err != nil {
		t.Fatal(err)
	}
	write, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := getTx(write)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`CREATE TABLE restore_must_remove (id INTEGER PRIMARY KEY)`); err != nil {
		database.Rollback(write)
		t.Fatal(err)
	}
	if err := database.Commit(write); err != nil {
		t.Fatal(err)
	}

	if err := database.RestoreFromBackup(backupPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("consumed backup still exists or stat failed: %v", err)
	}
	if database.Version() != database.AppSchemaVersion() {
		t.Fatalf("restored version=%d want %d", database.Version(), database.AppSchemaVersion())
	}
	if err := database.Ready(); err != nil {
		t.Fatalf("restored current-schema database is not ready: %v", err)
	}
	info, err := os.Stat(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("restored database permissions=%#o want 0600", got)
	}

	read, err := database.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dbReader, err := getDBReader(read)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := dbReader.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'restore_must_remove'`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("restore retained a table created only after the backup")
	}
}

func TestRestoreFromBackupRefreshesPendingMigrationState(t *testing.T) {
	if appSchemaVersion < 2 {
		t.Skip("appSchemaVersion too low to construct a pending-migration fixture")
	}

	pending := newDatabaseAtVersion(t, appSchemaVersion-1)
	backupPath := filepath.Join(t.TempDir(), "pending-migration.sqlite")
	if err := pending.Backup(backupPath); err != nil {
		t.Fatal(err)
	}

	target := NewDatabase()
	if err := target.Open(filepath.Join(t.TempDir(), "bind-mounted.sqlite")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })

	if err := target.RestoreFromBackup(backupPath); err != nil {
		t.Fatal(err)
	}
	if target.Version() != appSchemaVersion-1 {
		t.Fatalf("restored in-memory version=%d want %d", target.Version(), appSchemaVersion-1)
	}
	if readyErr := target.Ready(); !errors.Is(readyErr, ErrDatabaseNotInitialized) {
		t.Fatalf("pending-migration restore Ready()=%v want ErrDatabaseNotInitialized", readyErr)
	}

	migrator, err := NewMigrator(target)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if got := migrator.CurrentSchemaVersion(); got != appSchemaVersion-1 {
		t.Fatalf("authoritative restored path version=%d want %d", got, appSchemaVersion-1)
	}
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestCS08P1AUpgradePreservesStableIdentityAndCreatesAllCamTables(t *testing.T) {
	const (
		p1aVersion = uint(88)
		userID     = int64(700001)
		tagID      = int64(700002)
	)
	fixture := newDatabaseAtVersion(t, p1aVersion)
	path := fixture.DatabasePath()
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO users(id,username,normalized_username,password_hash,role,status,created_at,updated_at)
		VALUES(?, 'cs08-p1a-owner', 'cs08-p1a-owner', NULL, 'ADMIN', 'ACTIVE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, userID); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO tags(id,name,created_at,updated_at) VALUES(?, 'CS08 stable tag', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, tagID); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if output := os.Getenv("CS08_P1A_FIXTURE_OUTPUT"); output != "" {
		if err := fixture.Backup(output); err != nil {
			t.Fatalf("exporting disposable P1A fixture: %v", err)
		}
	}

	database := NewDatabase()
	var migrationNeeded *MigrationNeededError
	if err := database.Open(path); !errors.As(err, &migrationNeeded) || migrationNeeded.CurrentSchemaVersion != p1aVersion || migrationNeeded.RequiredSchemaVersion != appSchemaVersion {
		t.Fatalf("P1A open=%v migration=%+v", err, migrationNeeded)
	}
	migrator, err := NewMigrator(database)
	if err != nil {
		t.Fatal(err)
	}
	for version := p1aVersion + 1; version <= appSchemaVersion; version++ {
		if err := migrator.RunMigration(context.Background(), version); err != nil {
			migrator.Close()
			t.Fatalf("migration %d: %v", version, err)
		}
	}
	if err := migrator.PostMigrate(context.Background()); err != nil {
		migrator.Close()
		t.Fatal(err)
	}
	migrator.Close()
	if err := database.ReInitialise(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx, err := database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(ctx)
	var gotUserID, gotTagID int64
	if err := dbWrapper.Get(ctx, &gotUserID, `SELECT id FROM users WHERE username='cs08-p1a-owner'`); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(ctx, &gotTagID, `SELECT id FROM tags WHERE name='CS08 stable tag'`); err != nil {
		t.Fatal(err)
	}
	if gotUserID != userID || gotTagID != tagID {
		t.Fatalf("stable IDs user=%d tag=%d", gotUserID, gotTagID)
	}

	expected := []string{
		"cam_completed_recording_audits", "cam_completed_recording_imports",
		"cam_model_accounts", "cam_model_aliases", "cam_model_profile_provenance",
		"cam_model_social_profiles", "cam_model_user_state", "cam_models",
		"cam_show_classification_rule_tags", "cam_show_classification_rules",
		"cam_show_links", "cam_show_models", "cam_show_sites", "cam_shows",
		"cam_sites", "cam_sync_changes",
	}
	for _, table := range expected {
		var found int
		if err := dbWrapper.Get(ctx, &found, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table); err != nil || found != 1 {
			t.Fatalf("Cam table %s found=%d err=%v", table, found, err)
		}
	}
	var state struct {
		Version int  `db:"version"`
		Dirty   bool `db:"dirty"`
	}
	if err := dbWrapper.Get(ctx, &state, `SELECT version,dirty FROM schema_migrations`); err != nil {
		t.Fatal(err)
	}
	if state.Version != int(appSchemaVersion) || state.Dirty || database.Version() != appSchemaVersion {
		t.Fatalf("schema version=%d dirty=%v live=%d", state.Version, state.Dirty, database.Version())
	}
}

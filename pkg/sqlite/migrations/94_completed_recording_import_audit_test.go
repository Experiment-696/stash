package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newCompletedImportMigrationDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON; CREATE TABLE scenes(id integer primary key); CREATE TABLE performers(id integer primary key); CREATE TABLE users(id integer primary key); INSERT INTO scenes VALUES(1); INSERT INTO users VALUES(1);"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	for _, name := range []string{"89_cam_shows_foundation.up.sql", "90_cam_show_classification_rules.up.sql", "91_cam_model_profile_provenance.up.sql", "92_cam_show_domain_correction.up.sql", "93_cam_show_hour_precision.up.sql"} {
		body, err := os.ReadFile(name)
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			db.Close()
			t.Fatalf("%s: %v", name, err)
		}
	}
	return db
}

func Test94CompletedRecordingImportAuditSchemaConstraints(t *testing.T) {
	db := newCompletedImportMigrationDB(t)
	defer db.Close()
	body, err := os.ReadFile("94_completed_recording_import_audit.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO cam_sites(id,name,enabled,created_at,updated_at) VALUES(1,'Site',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP); INSERT INTO cam_models(id,display_name,status,created_at,updated_at) VALUES(1,'Model','ACTIVE',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP); INSERT INTO cam_shows(id,scene_id,category,show_type,sync_state,created_at,updated_at) VALUES(1,1,'LIVE','LIVE_PUBLIC','LOCAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	const insert = `INSERT INTO cam_completed_recording_imports(id,scene_id,show_id,site_id,model_id,configured_root_id,relative_path_hash,fingerprint_size,fingerprint_mtime_ns,fingerprint_mode,fingerprint_device,fingerprint_inode,parser_version,captured_at,captured_timezone,captured_precision,match_state,outcome,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`
	args := []interface{}{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, 1, 1, 1, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", 0, 1, 2, 3, 4, "v1", "2026-07-21T12:00:00Z", "UTC", "SECOND", "EXACT_CURRENT", "APPLIED"}
	if _, err := db.Exec(insert, args...); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert, args...); err == nil {
		t.Fatal("duplicate stable import identity accepted")
	}
	bad := append([]interface{}(nil), args...)
	bad[0] = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	bad[17] = "REVIEW_REQUIRED"
	if _, err := db.Exec(insert, bad...); err == nil {
		t.Fatal("non-applied import row accepted")
	}
	if _, err := db.Exec("INSERT INTO cam_completed_recording_audits(actor_user_id,preview_id,candidate_id,relative_path_hash,outcome,review_reason_code,created_at) VALUES('1','preview','candidate','hash','REVIEW_REQUIRED','HISTORICAL_ALIAS_REUSED',CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO cam_completed_recording_audits(actor_user_id,preview_id,candidate_id,relative_path_hash,outcome,review_reason_code,created_at) VALUES('1','p','c','h','REVIEW_REQUIRED','FUZZY_GUESS',CURRENT_TIMESTAMP)"); err == nil {
		t.Fatal("unknown review reason accepted")
	}
}

func Test94CompletedRecordingMigrationRollsBackDDL(t *testing.T) {
	db := newCompletedImportMigrationDB(t)
	defer db.Close()
	body, err := os.ReadFile("94_completed_recording_import_audit.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO table_that_does_not_exist VALUES(1)"); err == nil {
		t.Fatal("injected migration failure did not fail")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('cam_completed_recording_imports','cam_completed_recording_audits')"); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("migration DDL survived rollback: %d tables", count)
	}
}

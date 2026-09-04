package migrations

import (
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"testing"
)

func Test93CamShowHourPrecisionPreservesRowsAndConstraint(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA foreign_keys=ON; CREATE TABLE scenes(id integer primary key); CREATE TABLE performers(id integer primary key); CREATE TABLE users(id integer primary key); INSERT INTO scenes VALUES(1),(2);"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"89_cam_shows_foundation.up.sql", "90_cam_show_classification_rules.up.sql", "91_cam_model_profile_provenance.up.sql", "92_cam_show_domain_correction.up.sql"} {
		migration, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(migration)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := db.Exec("INSERT INTO cam_shows(scene_id,category,show_type,captured_precision,sync_state,created_at,updated_at) VALUES(1,'LIVE','LIVE_PUBLIC','MINUTE','LOCAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),(2,'PRIVATE','PRIVATE_CALL',NULL,'LOCAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("93_cam_show_hour_precision.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	var rows, minute int
	_ = db.Get(&rows, "SELECT count(*) FROM cam_shows")
	_ = db.Get(&minute, "SELECT count(*) FROM cam_shows WHERE captured_precision='MINUTE'")
	if rows != 2 || minute != 1 {
		t.Fatalf("rows=%d minute=%d", rows, minute)
	}
	if _, err := db.Exec("UPDATE cam_shows SET captured_precision='HOUR' WHERE id=1"); err != nil {
		t.Fatalf("HOUR rejected: %v", err)
	}
	if _, err := db.Exec("UPDATE cam_shows SET captured_precision='WEEK' WHERE id=1"); err == nil {
		t.Fatal("invalid precision accepted")
	}
}

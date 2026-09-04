package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func Test89CamShowsFoundationIsRestartSafeAndPreservesBridges(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE scenes(id integer primary key); CREATE TABLE performers(id integer primary key); CREATE TABLE users(id integer primary key); INSERT INTO scenes VALUES(1); INSERT INTO performers VALUES(2); INSERT INTO users VALUES(3);`); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("89_cam_shows_foundation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cam_sites(name,enabled,created_at,updated_at) VALUES('Example',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
		INSERT INTO cam_models(display_name,status,performer_id,created_at,updated_at) VALUES('Model','ACTIVE',2,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
		INSERT INTO cam_shows(scene_id,category,site_id,sync_state,created_at,updated_at) VALUES(1,'RECORDED',1,'LOCAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
		INSERT INTO cam_show_models VALUES(1,1,0,'FEATURED');`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("restart retry: %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT count(*) FROM cam_shows WHERE scene_id=1`); err != nil || count != 1 {
		t.Fatalf("show count=%d err=%v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO cam_shows(scene_id,category,sync_state,created_at,updated_at) VALUES(1,'OTHER','LOCAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("duplicate Scene bridge accepted")
	}
	if _, err := db.Exec(`INSERT INTO cam_model_accounts(model_id,site_id,handle,normalized_handle,status,source,created_at,updated_at) VALUES(1,1,'Model','model','ACTIVE','MANUAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP), (1,1,'MODEL','model','ACTIVE','MANUAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("duplicate active site handle accepted")
	}
	if _, err := db.Exec(`DELETE FROM scenes WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&count, `SELECT count(*) FROM cam_shows`); err != nil || count != 0 {
		t.Fatalf("Scene cascade count=%d err=%v", count, err)
	}
	if _, err := db.Exec(`DELETE FROM performers WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	var performerID *int64
	if err := db.Get(&performerID, `SELECT performer_id FROM cam_models WHERE id=1`); err != nil || performerID != nil {
		t.Fatalf("Performer bridge was not cleared: %v err=%v", performerID, err)
	}
}

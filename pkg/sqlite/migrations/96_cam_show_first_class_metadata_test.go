package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func Test96CamShowFirstClassMetadataAndPersonalRatings(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;
		CREATE TABLE users(id integer primary key);
		CREATE TABLE scenes(id integer primary key);
		CREATE TABLE cam_shows(id integer primary key, scene_id integer references scenes(id) on delete cascade, notes text);
		INSERT INTO users VALUES(1), (2);
		INSERT INTO scenes VALUES(10);
		INSERT INTO cam_shows VALUES(20, 10, 'legacy extras');`); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("96_cam_show_first_class_metadata.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	var extras string
	if err := db.Get(&extras, `SELECT extras FROM cam_shows WHERE id=20`); err != nil || extras != "legacy extras" {
		t.Fatalf("legacy extras=%q err=%v", extras, err)
	}
	if _, err := db.Exec(`UPDATE cam_shows SET rate=12.50, request='custom request' WHERE id=20;
		INSERT INTO cam_show_user_state(user_id,show_id,rating,updated_at) VALUES(1,20,87,CURRENT_TIMESTAMP),(2,20,63,CURRENT_TIMESTAMP);`); err != nil {
		t.Fatal(err)
	}
	var average float64
	if err := db.Get(&average, `SELECT AVG(rating) FROM cam_show_user_state WHERE show_id=20`); err != nil || average != 75 {
		t.Fatalf("average=%v err=%v", average, err)
	}
	if _, err := db.Exec(`INSERT INTO cam_show_user_state(user_id,show_id,rating,updated_at) VALUES(1,20,101,CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("out-of-range rating accepted")
	}
	if _, err := db.Exec(`DELETE FROM cam_shows WHERE id=20`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, `SELECT count(*) FROM cam_show_user_state`); err != nil || count != 0 {
		t.Fatalf("cascade rows=%d err=%v", count, err)
	}
}

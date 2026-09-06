package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func Test95ScenePersonalRatingsMigratesOnlyConvertedOwner(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;
		CREATE TABLE users(id integer primary key);
		CREATE TABLE scenes(id integer primary key, rating integer);
		CREATE TABLE user_audit_events(id integer primary key, actor_user_id integer, event_type text);
		INSERT INTO users VALUES(1), (2);
		INSERT INTO scenes VALUES(10, 80), (11, NULL);
		INSERT INTO user_audit_events VALUES(1, 1, 'legacy_identity_converted');`); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("89_scene_personal_ratings.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, `SELECT count(*) FROM user_scene_state WHERE user_id=1 AND scene_id=10 AND rating=80`); err != nil || count != 1 {
		t.Fatalf("converted rating count=%d err=%v", count, err)
	}
	if err := db.Get(&count, `SELECT count(*) FROM user_scene_state WHERE user_id=2 OR scene_id=11`); err != nil || count != 0 {
		t.Fatalf("unowned rating rows=%d err=%v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO user_scene_state(user_id,scene_id,rating,updated_at) VALUES(2,10,40,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM scenes WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&count, `SELECT count(*) FROM user_scene_state`); err != nil || count != 0 {
		t.Fatalf("cascade rows=%d err=%v", count, err)
	}
}

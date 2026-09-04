package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func Test87RepairsStale86PreservesDataAndIsRetrySafe(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;
		CREATE TABLE users (id integer primary key, role text);
		CREATE TABLE scenes (id integer primary key);
		CREATE TABLE images (id integer primary key);
		CREATE TABLE performers (id integer primary key, favorite boolean not null default 0, rating integer);
		CREATE TABLE user_audit_events (id integer primary key, actor_user_id integer, event_type text);
		INSERT INTO users VALUES (12, 'ADMIN'), (13, 'ADMIN');
		INSERT INTO scenes VALUES (21);
		INSERT INTO images VALUES (31);
		INSERT INTO performers VALUES (41, 1, 90), (42, 0, NULL);
		INSERT INTO user_audit_events VALUES (1, 12, 'legacy_identity_converted');`); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("87_multiuser_personal_state_repair.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_scene_activity(user_id,scene_id,resume_time,play_duration,updated_at)
		VALUES (12,21,4,8,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	// Direct retry proves every repair DDL/DML statement is idempotent.
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("retry: %v", err)
	}
	for _, table := range []string{"user_scene_activity", "user_scene_history", "user_image_activity", "user_performer_state"} {
		var count int
		if err := db.Get(&count, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table); err != nil || count != 1 {
			t.Fatalf("table %s count=%d err=%v", table, count, err)
		}
	}
	var owner, rating int
	var favorite bool
	if err := db.QueryRowx(`SELECT user_id,favorite,rating FROM user_performer_state WHERE performer_id=41`).Scan(&owner, &favorite, &rating); err != nil {
		t.Fatal(err)
	}
	if owner != 12 || !favorite || rating != 90 {
		t.Fatalf("backfill owner=%d favorite=%v rating=%d", owner, favorite, rating)
	}
	var activityCount int
	if err := db.Get(&activityCount, `SELECT count(*) FROM user_scene_activity WHERE user_id=12 AND scene_id=21`); err != nil || activityCount != 1 {
		t.Fatalf("preserved activity count=%d err=%v", activityCount, err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id=12`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&activityCount, `SELECT count(*) FROM user_scene_activity WHERE user_id=12`); err != nil || activityCount != 0 {
		t.Fatalf("foreign-key cascade count=%d err=%v", activityCount, err)
	}
}

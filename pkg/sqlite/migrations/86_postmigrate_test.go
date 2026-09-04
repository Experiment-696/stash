package migrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestConvertLegacyIdentityIsIdempotent(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE saved_filters (id integer primary key, name text not null, mode text not null, filter blob not null);
		CREATE TABLE performers (id integer primary key, favorite boolean not null default 0, rating integer);`); err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("86_multiuser_foundation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO saved_filters (name, mode, filter) VALUES ('mine', 'SCENES', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO performers (id, favorite, rating) VALUES (7, 1, 80), (8, 0, NULL)`); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	identity := legacyIdentity{Username: "Admin", PasswordHash: "$2a$10$existing", APIKey: "legacy-secret"}
	for i := 0; i < 2; i++ {
		if err := convertLegacyIdentity(context.Background(), db, identity, now); err != nil {
			t.Fatalf("conversion %d: %v", i+1, err)
		}
	}

	for table, want := range map[string]int{"users": 1, "user_api_tokens": 1, "user_audit_events": 1} {
		var got int
		if err := db.Get(&got, `SELECT count(*) FROM `+table); err != nil || got != want {
			t.Fatalf("%s count=%d err=%v want=%d", table, got, err, want)
		}
	}
	var ownerID int
	if err := db.Get(&ownerID, `SELECT user_id FROM saved_filters WHERE name = 'mine'`); err != nil || ownerID == 0 {
		t.Fatalf("saved filter owner=%d err=%v", ownerID, err)
	}
	digest := sha256.Sum256([]byte(identity.APIKey))
	var storedHash string
	var expires time.Time
	if err := db.QueryRowx(`SELECT secret_hash, expires_at FROM user_api_tokens`).Scan(&storedHash, &expires); err != nil {
		t.Fatal(err)
	}
	if storedHash != hex.EncodeToString(digest[:]) || storedHash == identity.APIKey {
		t.Fatal("legacy API key was not stored only as its SHA-256 digest")
	}
	if !expires.Equal(now.Add(90 * 24 * time.Hour)) {
		t.Fatalf("expires=%s", expires)
	}
	var favorite bool
	var rating int
	if err := db.QueryRowx(`SELECT favorite, rating FROM user_performer_state WHERE performer_id = 7 AND user_id = ?`, ownerID).Scan(&favorite, &rating); err != nil {
		t.Fatal(err)
	}
	if !favorite || rating != 80 {
		t.Fatalf("legacy performer state favorite=%v rating=%d", favorite, rating)
	}
	var emptyStateCount int
	if err := db.Get(&emptyStateCount, `SELECT count(*) FROM user_performer_state WHERE performer_id = 8`); err != nil || emptyStateCount != 0 {
		t.Fatalf("empty legacy performer state count=%d err=%v", emptyStateCount, err)
	}
}

func TestConvertLegacyIdentityNoCredentialsIsNoop(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := convertLegacyIdentity(context.Background(), db, legacyIdentity{}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateLegacyDefaultFiltersOnlyUnambiguousMatch(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE users (id integer primary key);
		CREATE TABLE saved_filters (id integer primary key, user_id integer, mode text, find_filter text, object_filter text, ui_options text);
		CREATE TABLE user_preferences (user_id integer, key text, value_json text, updated_at datetime, primary key(user_id, key));
		INSERT INTO users(id) VALUES (1);
		INSERT INTO saved_filters(id,user_id,mode,find_filter,object_filter,ui_options) VALUES
			(10,1,'SCENES','{"sort":"title"}','{"rating":5}',''),
			(20,1,'IMAGES','{"sort":"title"}','{}',''),
			(21,1,'IMAGES','{"sort":"title"}','{}','');`); err != nil {
		t.Fatal(err)
	}
	ui := map[string]interface{}{"defaultFilters": map[string]interface{}{
		"scenes": map[string]interface{}{"find_filter": map[string]interface{}{"sort": "title"}, "object_filter": map[string]interface{}{"rating": float64(5)}},
		"images": map[string]interface{}{"find_filter": map[string]interface{}{"sort": "title"}, "object_filter": map[string]interface{}{}},
	}}
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyDefaultFilters(context.Background(), tx, 1, ui, time.Now().UTC()); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var sceneID string
	if err := db.Get(&sceneID, `SELECT value_json FROM user_preferences WHERE user_id=1 AND key='default_filter:SCENES'`); err != nil || sceneID != "10" {
		t.Fatalf("scene default=%q err=%v", sceneID, err)
	}
	var ambiguous int
	if err := db.Get(&ambiguous, `SELECT count(*) FROM user_preferences WHERE key='default_filter:IMAGES'`); err != nil || ambiguous != 0 {
		t.Fatalf("ambiguous default count=%d err=%v", ambiguous, err)
	}
}

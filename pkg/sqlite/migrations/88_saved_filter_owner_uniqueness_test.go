package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func Test88ScopesSavedFilterNamesToOwner(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE saved_filters (
			id integer primary key,
			user_id integer,
			mode varchar(20) not null,
			name varchar(255) not null
		);
		CREATE UNIQUE INDEX index_saved_filters_on_mode_name_unique
		ON saved_filters (mode, name);
		INSERT INTO saved_filters VALUES (1, 10, 'SCENES', 'Favorites');
	`); err != nil {
		t.Fatal(err)
	}

	schema, err := os.ReadFile("88_saved_filter_owner_uniqueness.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
		INSERT INTO saved_filters VALUES (2, 11, 'SCENES', 'Favorites');
	`); err != nil {
		t.Fatalf("different owners could not reuse a filter name: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO saved_filters VALUES (3, 10, 'SCENES', 'Favorites');
	`); err == nil {
		t.Fatal("same owner created a duplicate mode/name filter")
	}
}

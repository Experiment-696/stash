package migrations

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func Test92CamShowDomainBackfillPreservesSceneBridges(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE scenes(id integer primary key); CREATE TABLE performers(id integer primary key); CREATE TABLE users(id integer primary key); INSERT INTO scenes VALUES(1),(2);`); err != nil {
		t.Fatal(err)
	}
	foundation, _ := os.ReadFile("89_cam_shows_foundation.up.sql")
	if _, err := db.Exec(string(foundation)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cam_sites(name,enabled,created_at,updated_at) VALUES('Site',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP); INSERT INTO cam_shows(scene_id,category,site_id,source_url,sync_state,created_at,updated_at) VALUES(1,'LIVE CAPTURE',1,'https://example.test/show/1','LOCAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),(2,'RECORDED',NULL,NULL,'LOCAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);`); err != nil {
		t.Fatal(err)
	}
	migration, _ := os.ReadFile("92_cam_show_domain_correction.up.sql")
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	var shows, scenes, sites, links, live, custom int
	_ = db.Get(&shows, `SELECT count(*) FROM cam_shows`)
	_ = db.Get(&scenes, `SELECT count(DISTINCT scene_id) FROM cam_shows`)
	_ = db.Get(&sites, `SELECT count(*) FROM cam_show_sites`)
	_ = db.Get(&links, `SELECT count(*) FROM cam_show_links`)
	_ = db.Get(&live, `SELECT count(*) FROM cam_shows WHERE show_type="LIVE_PUBLIC"`)
	_ = db.Get(&custom, `SELECT count(*) FROM cam_shows WHERE show_type="CUSTOM_VIDEO"`)
	if shows != 2 || scenes != 2 || sites != 1 || links != 1 || live != 1 || custom != 1 {
		t.Fatalf("shows=%d scenes=%d sites=%d links=%d live=%d custom=%d", shows, scenes, sites, links, live, custom)
	}
	if _, err := db.Exec(`UPDATE cam_shows SET duration_override_seconds=12 WHERE id=1`); err == nil {
		t.Fatal("unjustified duration override accepted")
	}
	if _, err := db.Exec(`INSERT INTO cam_show_links(show_id,link_type,url,source,created_at,updated_at) VALUES(1,'SOCIAL','https://example.test/social','MANUAL',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("untyped/social Show link accepted")
	}
}

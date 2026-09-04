package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestCamSnapshotV92NewTablesRoundTripRollbackAndIdempotency(t *testing.T) {
	config.InitializeEmpty()
	open := func(name string) *Database {
		db := NewDatabase()
		if err := db.Open(filepath.Join(t.TempDir(), name)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	seed := func(db *Database) (*CamSnapshot, int64) {
		ctx, err := db.Begin(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		site, err := db.CamShow.CreateSite(ctx, "V92 Site", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		model, err := db.CamShow.CreateModel(ctx, "V92 Model", nil)
		if err != nil {
			t.Fatal(err)
		}
		scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := db.Scene.Create(ctx, scene, nil); err != nil {
			t.Fatal(err)
		}
		show, err := db.CamShow.CreateShow(ctx, int64(scene.ID), "LIVE", &site.ID)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		if _, err := dbWrapper.Exec(ctx, `INSERT INTO cam_show_sites(show_id,site_id,created_at) VALUES(?,?,?)`, show.ID, site.ID, now); err != nil {
			t.Fatal(err)
		}
		if _, err := dbWrapper.Exec(ctx, `INSERT INTO cam_show_links(show_id,site_id,link_type,url,label,source,created_at,updated_at) VALUES(?,?,'SHOW','https://example.test/show','Archived show','MANUAL',?,?)`, show.ID, site.ID, now, now); err != nil {
			t.Fatal(err)
		}
		if _, err := dbWrapper.Exec(ctx, `INSERT INTO cam_model_social_profiles(model_id,platform,icon,handle,url,status,valid_from,source,confidence,provenance,created_at,updated_at) VALUES(?,'X','x','v92_model','https://x.test/v92_model','ACTIVE',?,'MANUAL',1,'owner fixture',?,?)`, model.ID, now, now, now); err != nil {
			t.Fatal(err)
		}
		snapshot, err := db.CamShow.ExportSnapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return snapshot, int64(scene.ID)
	}

	source := open("source.sqlite")
	snapshot, sceneID := seed(source)
	for _, table := range []string{"cam_show_sites", "cam_show_links", "cam_model_social_profiles"} {
		if camSnapshotRowCount(t, snapshot, table) != 1 {
			t.Fatalf("%s not seeded", table)
		}
	}

	for _, corruptTable := range []string{"cam_show_sites", "cam_show_links", "cam_model_social_profiles"} {
		t.Run("rollback_"+corruptTable, func(t *testing.T) {
			target := open("target-" + corruptTable + ".sqlite")
			ctx, err := target.Begin(context.Background(), true)
			if err != nil {
				t.Fatal(err)
			}
			scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if err := target.Scene.Create(ctx, scene, nil); err != nil {
				t.Fatal(err)
			}
			if int64(scene.ID) != sceneID {
				t.Fatalf("scene id=%d want=%d", scene.ID, sceneID)
			}
			encoded, _ := json.Marshal(snapshot)
			var invalid CamSnapshot
			if err := json.Unmarshal(encoded, &invalid); err != nil {
				t.Fatal(err)
			}
			for i := range invalid.Tables {
				if invalid.Tables[i].Name == corruptTable {
					invalid.Tables[i].Rows[0][1] = float64(999999)
				}
			}
			if err := target.CamShow.ImportSnapshot(ctx, invalid); err == nil {
				t.Fatal("corrupt FK imported")
			}
			for _, name := range []string{"cam_sites", "cam_models", "cam_shows", "cam_show_sites", "cam_show_links", "cam_model_social_profiles"} {
				var count int
				if err := dbWrapper.Get(ctx, &count, "SELECT count(*) FROM "+name); err != nil || count != 0 {
					t.Fatalf("%s survived rollback: count=%d err=%v", name, count, err)
				}
			}
			if err := target.CamShow.ImportSnapshot(ctx, *snapshot); err != nil {
				t.Fatal(err)
			}
			if err := target.CamShow.ImportSnapshot(ctx, *snapshot); err != nil {
				t.Fatalf("idempotent replay: %v", err)
			}
			restored, err := target.CamShow.ExportSnapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snapshot, restored) {
				t.Fatal("v92 round trip differs")
			}
			if err := target.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

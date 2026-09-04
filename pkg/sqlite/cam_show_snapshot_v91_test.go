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

func TestCamSnapshotV91ProvenanceRoundTripRollbackAndIdempotency(t *testing.T) {
	config.InitializeEmpty()
	open := func(name string) *Database {
		db := NewDatabase()
		if err := db.Open(filepath.Join(t.TempDir(), name)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	source := open("source-v91.sqlite")
	write, err := source.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	site, err := source.CamShow.CreateSite(write, "Snapshot Site", nil, stringPointer("snapshot-site"))
	if err != nil {
		t.Fatal(err)
	}
	model, err := source.CamShow.CreateModel(write, "Snapshot Model", nil)
	if err != nil {
		t.Fatal(err)
	}
	account, err := source.CamShow.AddManualModelAccount(write, CamModelAccountInput{ModelID: model.ID, SiteID: site.ID, Handle: "snapshot_name"})
	if err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := source.Scene.Create(write, scene, nil); err != nil {
		t.Fatal(err)
	}
	show, err := source.CamShow.CreateShow(write, int64(scene.ID), "LIVE CAPTURE", &site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.CamShow.LinkModelWithRole(write, show.ID, model.ID, 0, "PRIMARY"); err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(write, `INSERT INTO cam_show_sites(show_id,site_id,created_at) VALUES(?,?,?)`, show.ID, site.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(write, `INSERT INTO cam_show_links(show_id,site_id,link_type,url,source,created_at,updated_at) VALUES(?,?,'SHOW','https://example.test/show','MANUAL',?,?)`, show.ID, site.ID, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(write, `INSERT INTO cam_model_social_profiles(model_id,platform,handle,url,status,source,created_at,updated_at) VALUES(?,'TELEGRAM','snapshot','https://t.me/snapshot','ACTIVE','MANUAL',?,?)`, model.ID, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, err = source.CamShow.IngestModelProvenance(write, CamModelProvenanceInput{ModelID: model.ID, AccountID: &account.ID, Provider: "snapshot-provider", EvidenceKey: "snapshot-evidence", ObservedAt: time.Now().UTC().Truncate(time.Second), PayloadJSON: `{"source":"fixture"}`})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.CamShow.ExportSnapshot(write)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Commit(write); err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != currentCamSnapshotVersion || camSnapshotRowCount(t, snapshot, "cam_model_profile_provenance") != 1 || camSnapshotRowCount(t, snapshot, "cam_show_sites") != 1 || camSnapshotRowCount(t, snapshot, "cam_show_links") != 1 || camSnapshotRowCount(t, snapshot, "cam_model_social_profiles") != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}

	target := open("target-v91.sqlite")
	ctx, err := target.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	// Scenes remain outside the Cam snapshot contract; establish the media bridge
	// that an importing Stash database must already contain.
	targetScene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := target.Scene.Create(ctx, targetScene, nil); err != nil {
		t.Fatal(err)
	}
	if targetScene.ID != scene.ID {
		t.Fatalf("scene bridge id = %d, want %d", targetScene.ID, scene.ID)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var invalid CamSnapshot
	if err := json.Unmarshal(encoded, &invalid); err != nil {
		t.Fatal(err)
	}
	for i := range invalid.Tables {
		if invalid.Tables[i].Name == "cam_model_profile_provenance" {
			invalid.Tables[i].Rows[0][2] = float64(account.ID + 999)
		}
	}
	if err := target.CamShow.ImportSnapshot(ctx, invalid); err == nil {
		t.Fatal("incoherent provenance imported")
	}
	if sites, err := target.CamShow.ListSites(ctx); err != nil || len(sites) != 0 {
		t.Fatalf("failed import did not roll back sites: sites=%+v err=%v", sites, err)
	}
	if err := target.CamShow.ImportSnapshot(ctx, *snapshot); err != nil {
		t.Fatal(err)
	}
	if err := target.CamShow.ImportSnapshot(ctx, *snapshot); err != nil {
		t.Fatalf("idempotent import: %v", err)
	}
	restored, err := target.CamShow.ExportSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, restored) {
		t.Fatalf("round trip differs:\nsource=%+v\ntarget=%+v", snapshot, restored)
	}
	if err := target.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

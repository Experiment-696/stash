package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
	"github.com/stashapp/stash/pkg/models"
)

func TestCamSnapshotV94CompletedImportAuditRoundTripRollbackAndIdempotency(t *testing.T) {
	config.InitializeEmpty()
	open := func(name string) *Database {
		db := NewDatabase()
		if err := db.Open(filepath.Join(t.TempDir(), name)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	source := open("source-v94.sqlite")
	setup, err := source.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := source.Scene.Create(setup, scene, nil); err != nil {
		t.Fatal(err)
	}
	site, err := source.CamShow.CreateSite(setup, "V94 Site", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model, err := source.CamShow.CreateModel(setup, "V94 Model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.CamShow.CreateShow(setup, int64(scene.ID), "LIVE", nil); err != nil {
		t.Fatal(err)
	}
	if err := source.Commit(setup); err != nil {
		t.Fatal(err)
	}
	item := completedRepositoryItem(int64(scene.ID), site.ID, model.ID, "v94/model.mp4")
	repo := NewCompletedImportRepository(source)
	if err := repo.WithCompletedImportTransaction(context.Background(), func(_ context.Context, tx cammodel.CompletedImportTx) error {
		if _, err := tx.LinkCamShowMetadata(context.Background(), item); err != nil {
			return err
		}
		return tx.WriteCompletedImportAudit(context.Background(), completedRepositoryAudit(item, cammodel.CompletedApplied, "metadata only"))
	}); err != nil {
		t.Fatal(err)
	}
	read, _ := source.Begin(context.Background(), false)
	snapshot, err := source.CamShow.ExportSnapshot(read)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Commit(read); err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != 94 || camSnapshotRowCount(t, snapshot, "cam_completed_recording_imports") != 1 ||
		camSnapshotRowCount(t, snapshot, "cam_completed_recording_audits") != 1 {
		t.Fatalf("snapshot version/tables=%#v", snapshot)
	}

	target := open("target-v94.sqlite")
	tx, err := target.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	targetScene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := target.Scene.Create(tx, targetScene, nil); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(snapshot)
	var corrupt CamSnapshot
	if err := json.Unmarshal(encoded, &corrupt); err != nil {
		t.Fatal(err)
	}
	for i := range corrupt.Tables {
		if corrupt.Tables[i].Name == "cam_completed_recording_imports" {
			corrupt.Tables[i].Rows[0][2] = float64(999999)
		}
	}
	if err := target.CamShow.ImportSnapshot(tx, corrupt); err == nil {
		t.Fatal("corrupt import FK accepted")
	}
	for _, table := range []string{"cam_sites", "cam_models", "cam_shows", "cam_completed_recording_imports", "cam_completed_recording_audits"} {
		var count int
		if err := dbWrapper.Get(tx, &count, "SELECT count(*) FROM "+table); err != nil || count != 0 {
			t.Fatalf("%s survived snapshot rollback count=%d err=%v", table, count, err)
		}
	}
	if err := target.CamShow.ImportSnapshot(tx, *snapshot); err != nil {
		t.Fatal(err)
	}
	if err := target.CamShow.ImportSnapshot(tx, *snapshot); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	restored, err := target.CamShow.ExportSnapshot(tx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, restored) {
		t.Fatal("v94 snapshot round trip differs")
	}
	if err := target.Commit(tx); err != nil {
		t.Fatal(err)
	}
}

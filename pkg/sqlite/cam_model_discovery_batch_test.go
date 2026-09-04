package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
)

func TestDiscoveryMixedBatchReplayAndPersistenceRollback(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "discovery-batch.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	setup, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	platform := "cb"
	if _, err = db.CamShow.CreateSite(setup, "Chaturbate", nil, &platform); err != nil {
		t.Fatal(err)
	}
	model, err := db.CamShow.CreateModel(setup, "Target", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Commit(setup); err != nil {
		t.Fatal(err)
	}
	valid := cammodel.ProfileObservation{Provider: "camgirlfinder", EvidenceKey: "valid", Platform: "cb", Username: "selected", ObservedAt: time.Now().UTC().Truncate(time.Second), PayloadJSON: `{"name":"selected"}`}
	invalid := valid
	invalid.EvidenceKey = "unsupported"
	invalid.Platform = "missing"
	write, _ := db.Begin(context.Background(), true)
	if _, err = db.CamShow.IngestDiscoveryReview(write, model.ID, valid); err != nil {
		t.Fatal(err)
	}
	if _, err = db.CamShow.IngestDiscoveryReview(write, model.ID, invalid); !errors.Is(err, ErrCamModelDiscoverySite) {
		t.Fatalf("unsupported=%v", err)
	}
	if err = db.Commit(write); err != nil {
		t.Fatal(err)
	}
	replay, _ := db.Begin(context.Background(), true)
	got, err := db.CamShow.IngestDiscoveryReview(replay, model.ID, valid)
	if err != nil || got.EvidenceStatus != CamModelProvenanceUnchanged {
		t.Fatalf("replay=%+v err=%v", got, err)
	}
	if err = db.Commit(replay); err != nil {
		t.Fatal(err)
	}
	rollback, _ := db.Begin(context.Background(), true)
	fresh := valid
	fresh.EvidenceKey = "rollback"
	if _, err = db.CamShow.IngestDiscoveryReview(rollback, model.ID, fresh); err != nil {
		t.Fatal(err)
	}
	conflict := fresh
	conflict.PayloadJSON = `{"name":"changed"}`
	if _, err = db.CamShow.IngestDiscoveryReview(rollback, model.ID, conflict); !errors.Is(err, ErrCamModelProvenanceConflict) {
		t.Fatalf("persistence conflict=%v", err)
	}
	if err = db.Rollback(rollback); err != nil {
		t.Fatal(err)
	}
	read, _ := db.WithDatabase(context.Background())
	var evidence, changes int
	if err = dbWrapper.Get(read, &evidence, "SELECT COUNT(*) FROM cam_model_profile_provenance"); err != nil {
		t.Fatal(err)
	}
	if err = dbWrapper.Get(read, &changes, "SELECT COUNT(*) FROM cam_sync_changes"); err != nil {
		t.Fatal(err)
	}
	if evidence != 1 || changes != 1 {
		t.Fatalf("rollback failed evidence=%d changes=%d", evidence, changes)
	}
}

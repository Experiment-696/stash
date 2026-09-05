package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
)

func TestDiscoveryApplyRollsBackWithCallerTransaction(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "discovery-rollback.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	setup, _ := db.Begin(context.Background(), true)
	platform := "cb"
	_, _ = db.CamShow.CreateSite(setup, "Chaturbate", nil, &platform)
	model, _ := db.CamShow.CreateModel(setup, "Target", nil)
	if err := db.Commit(setup); err != nil {
		t.Fatal(err)
	}
	write, _ := db.Begin(context.Background(), true)
	image := "https://images.example/rollback.jpg"
	value := cammodel.ProfileObservation{Provider: "camgirlfinder", Platform: platform, Username: "rollback_alias", ImageURL: &image, ObservedAt: time.Now().UTC()}
	if _, err := db.CamShow.ApplyDiscoveryMetadata(write, model.ID, value); err != nil {
		t.Fatal(err)
	}
	if err := db.Rollback(write); err != nil {
		t.Fatal(err)
	}
	read, _ := db.WithDatabase(context.Background())
	var accounts, aliases int
	if err := dbWrapper.Get(read, &accounts, "SELECT COUNT(*) FROM cam_model_accounts WHERE model_id=?", model.ID); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(read, &aliases, "SELECT COUNT(*) FROM cam_model_aliases WHERE model_id=?", model.ID); err != nil {
		t.Fatal(err)
	}
	stored, _ := db.CamShow.FindModel(read, model.ID)
	if accounts != 0 || aliases != 0 || stored.Image != nil {
		t.Fatalf("rollback residue accounts=%d aliases=%d image=%v", accounts, aliases, stored.Image)
	}
}

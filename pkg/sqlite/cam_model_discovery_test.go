package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
)

func TestDiscoveryAppliesMetadataDirectlyAndIdempotently(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "discovery.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	platform := "cb"
	_, err = db.CamShow.CreateSite(ctx, "Chaturbate", nil, &platform)
	if err != nil {
		t.Fatal(err)
	}
	target, err := db.CamShow.CreateModel(ctx, "Target", nil)
	if err != nil {
		t.Fatal(err)
	}
	source, image := "https://chaturbate.com/Alice_CB/", "https://images.example/alice.jpg"
	o := cammodel.ProfileObservation{Provider: "camgirlfinder", Platform: platform, Username: "Alice_CB", SourceURL: &source, ImageURL: &image, ObservedAt: time.Now().UTC()}
	first, err := db.CamShow.ApplyDiscoveryMetadata(ctx, target.ID, o)
	if err != nil || first.Disposition != "CREATED" || !first.ImageApplied {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	replay, err := db.CamShow.ApplyDiscoveryMetadata(ctx, target.ID, o)
	if err != nil || replay.Disposition != "EXISTING" || replay.ImageApplied || replay.AccountID != first.AccountID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	var accounts, aliases, evidence, changes int
	for query, out := range map[string]*int{
		"SELECT COUNT(*) FROM cam_model_accounts WHERE model_id=" + strconv.FormatInt(target.ID, 10): &accounts,
		"SELECT COUNT(*) FROM cam_model_aliases WHERE model_id=" + strconv.FormatInt(target.ID, 10):  &aliases,
		"SELECT COUNT(*) FROM cam_model_profile_provenance":                                          &evidence,
		"SELECT COUNT(*) FROM cam_sync_changes":                                                      &changes,
	} {
		if err := dbWrapper.Get(ctx, out, query); err != nil {
			t.Fatal(err)
		}
	}
	if accounts != 1 || aliases != 1 || evidence != 0 || changes != 0 {
		t.Fatalf("accounts=%d aliases=%d evidence=%d changes=%d", accounts, aliases, evidence, changes)
	}
	stored, err := db.CamShow.FindModel(ctx, target.ID)
	if err != nil || stored.Image == nil || *stored.Image != image {
		t.Fatalf("model=%+v err=%v", stored, err)
	}
	var accountSource, aliasSource, handle, profileURL string
	if err := dbWrapper.Get(ctx, &accountSource, "SELECT source FROM cam_model_accounts WHERE id=?", first.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(ctx, &aliasSource, "SELECT source FROM cam_model_aliases WHERE account_id=?", first.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(ctx, &handle, "SELECT handle FROM cam_model_accounts WHERE id=?", first.AccountID); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(ctx, &profileURL, "SELECT profile_url FROM cam_model_accounts WHERE id=?", first.AccountID); err != nil {
		t.Fatal(err)
	}
	if accountSource != "CAMGIRLFINDER" || aliasSource != "CAMGIRLFINDER" || handle != "Alice_CB" || profileURL != source {
		t.Fatalf("account metadata source=%q aliasSource=%q handle=%q url=%q", accountSource, aliasSource, handle, profileURL)
	}
	if err := db.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryPreservesImageAndRejectsConflicts(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "conflict.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, _ := db.Begin(context.Background(), true)
	platform := "cb"
	site, _ := db.CamShow.CreateSite(ctx, "Chaturbate", nil, &platform)
	target, _ := db.CamShow.CreateModel(ctx, "Target", nil)
	other, _ := db.CamShow.CreateModel(ctx, "Other", nil)
	if _, err := dbWrapper.Exec(ctx, "UPDATE cam_models SET image='https://mine.example/keep.jpg' WHERE id=?", target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CamShow.AddManualModelAccount(ctx, CamModelAccountInput{ModelID: other.ID, SiteID: site.ID, Handle: "Taken"}); err != nil {
		t.Fatal(err)
	}
	image := "https://provider.example/replace.jpg"
	o := cammodel.ProfileObservation{Provider: "camgirlfinder", Platform: platform, Username: "taken", ImageURL: &image, ObservedAt: time.Now().UTC()}
	if _, err := db.CamShow.ApplyDiscoveryMetadata(ctx, target.ID, o); err == nil {
		t.Fatal("expected cross-model site username conflict")
	}
	o.Username = "available"
	got, err := db.CamShow.ApplyDiscoveryMetadata(ctx, target.ID, o)
	if err != nil || got.ImageApplied {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	stored, _ := db.CamShow.FindModel(ctx, target.ID)
	if stored.Image == nil || *stored.Image != "https://mine.example/keep.jpg" {
		t.Fatalf("existing image overwritten: %+v", stored.Image)
	}
	o.Platform = "missing"
	if _, err := db.CamShow.ApplyDiscoveryMetadata(ctx, target.ID, o); !errors.Is(err, ErrCamModelDiscoverySite) {
		t.Fatalf("missing site err=%v", err)
	}
	_ = db.Rollback(ctx)
}

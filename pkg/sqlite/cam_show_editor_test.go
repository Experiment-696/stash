package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestCamShowEditorAndSocialProfileValidationAndRollback(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "editor.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(ctx)
	scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Scene.Create(ctx, scene, nil); err != nil {
		t.Fatal(err)
	}
	show, err := db.CamShow.CreateShow(ctx, int64(scene.ID), "LIVE", nil)
	if err != nil {
		t.Fatal(err)
	}
	model, err := db.CamShow.CreateModel(ctx, "Social Model", nil)
	if err != nil {
		t.Fatal(err)
	}

	details, zone, precision := "owner notes", "America/Los_Angeles", "MINUTE"
	updated, err := db.CamShow.UpdateShowCore(ctx, CamShowCoreUpdateInput{ID: show.ID, Title: "Owner Show", ShowType: "LIVE_PRIVATE", CapturedTimezone: &zone, CapturedPrecision: &precision, Details: &details})
	if err != nil || updated.Title != "Owner Show" || updated.ShowType != "LIVE_PRIVATE" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	seconds := 12.0
	badPrecision := "WEEK"
	if _, err := db.CamShow.UpdateShowCore(ctx, CamShowCoreUpdateInput{ID: show.ID, Title: "bad", ShowType: "LIVE_PRIVATE", CapturedPrecision: &badPrecision}); err == nil || err.Error() != "captured precision is invalid" {
		t.Fatalf("friendly precision validation missing: %v", err)
	}
	if _, err := db.CamShow.UpdateShowCore(ctx, CamShowCoreUpdateInput{ID: show.ID, Title: "bad", ShowType: "LIVE_PRIVATE", DurationOverrideSeconds: &seconds}); err == nil {
		t.Fatal("unjustified duration accepted")
	}

	handle, provenance := "owner", "manual owner entry"
	social, err := db.CamShow.AddModelSocialProfile(ctx, CamModelSocialProfileInput{ModelID: model.ID, Platform: "X", Handle: &handle, URL: "https://x.example/owner", Source: "MANUAL", Provenance: &provenance})
	if err != nil || social.Status != "ACTIVE" {
		t.Fatalf("social=%+v err=%v", social, err)
	}
	if _, err := db.CamShow.AddModelSocialProfile(ctx, CamModelSocialProfileInput{ModelID: model.ID, Platform: "X", URL: "javascript:alert(1)", Source: "MANUAL"}); err == nil {
		t.Fatal("unsafe URL accepted")
	}
	retired, err := db.CamShow.RetireModelSocialProfile(ctx, social.ID, time.Now().UTC())
	if err != nil || retired.Status != "INACTIVE" || retired.ValidTo == nil {
		t.Fatalf("retired=%+v err=%v", retired, err)
	}
	profile, err := db.CamShow.FindModelProfile(ctx, model.ID)
	if err != nil || len(profile.SocialProfiles) != 1 {
		t.Fatalf("profile=%+v err=%v", profile, err)
	}
	if err := db.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := db.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := dbWrapper.Get(read, &count, `SELECT count(*) FROM cam_model_social_profiles`); err != nil || count != 0 {
		t.Fatalf("rollback count=%d err=%v", count, err)
	}
}

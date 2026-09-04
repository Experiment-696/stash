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

func TestDiscoveryReviewIsIdempotentAndNeverMutatesIdentity(t *testing.T) {
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
	site, err := db.CamShow.CreateSite(ctx, "Chaturbate", nil, &platform)
	if err != nil {
		t.Fatal(err)
	}
	target, err := db.CamShow.CreateModel(ctx, "Target", nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CamShow.CreateModel(ctx, "Other", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CamShow.AddManualModelAccount(ctx, CamModelAccountInput{ModelID: other.ID, SiteID: site.ID, Handle: "Taken"}); err != nil {
		t.Fatal(err)
	}
	o := cammodel.ProfileObservation{Provider: "camgirlfinder", EvidenceKey: "stable-1", Platform: platform, Username: "taken", ObservedAt: time.Now().UTC().Truncate(time.Second), PayloadJSON: `{"platform":"cb","name":"taken"}`}
	first, err := db.CamShow.IngestDiscoveryReview(ctx, target.ID, o)
	if err != nil {
		t.Fatal(err)
	}
	if first.Disposition != "HANDLE_CONFLICT" || first.EvidenceStatus != CamModelProvenanceInserted {
		t.Fatalf("first=%+v", first)
	}
	replay, err := db.CamShow.IngestDiscoveryReview(ctx, target.ID, o)
	if err != nil {
		t.Fatal(err)
	}
	if replay.EvidenceStatus != CamModelProvenanceUnchanged || replay.EvidenceID != first.EvidenceID || replay.ChangeID != first.ChangeID {
		t.Fatalf("replay=%+v", replay)
	}
	var accounts, aliases, changes, evidence int
	for query, out := range map[string]*int{"SELECT COUNT(*) FROM cam_model_accounts": &accounts, "SELECT COUNT(*) FROM cam_model_aliases": &aliases, "SELECT COUNT(*) FROM cam_sync_changes": &changes, "SELECT COUNT(*) FROM cam_model_profile_provenance": &evidence} {
		if err := dbWrapper.Get(ctx, out, query); err != nil {
			t.Fatal(err)
		}
	}
	if accounts != 1 || aliases != 0 || changes != 1 || evidence != 1 {
		t.Fatalf("accounts=%d aliases=%d changes=%d evidence=%d", accounts, aliases, changes, evidence)
	}
	missing := o
	missing.EvidenceKey = "missing"
	missing.Platform = "unknown"
	if _, err := db.CamShow.IngestDiscoveryReview(ctx, target.ID, missing); !errors.Is(err, ErrCamModelDiscoverySite) {
		t.Fatalf("missing site err=%v", err)
	}
	if err := db.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

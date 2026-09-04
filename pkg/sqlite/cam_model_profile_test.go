package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
)

var _ cammodel.PendingObservationStore = (*CamShowStore)(nil)

func TestCamModelProfileMultipleSitesHistoryProvenanceAndReview(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "cam-model-profile.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := db.User.Create(ctx, "profile-reviewer", "password", authz.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	firstSite, err := db.CamShow.CreateSite(ctx, "First Site", nil, stringPointer("first"))
	if err != nil {
		t.Fatal(err)
	}
	secondSite, err := db.CamShow.CreateSite(ctx, "Second Site", nil, stringPointer("second"))
	if err != nil {
		t.Fatal(err)
	}
	model, err := db.CamShow.CreateModel(ctx, "Durable Model", nil)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	oldAccount, err := db.CamShow.AddManualModelAccount(ctx, CamModelAccountInput{ModelID: model.ID, SiteID: firstSite.ID, Handle: "old_name", ValidFrom: &started})
	if err != nil {
		t.Fatal(err)
	}
	closed := started.Add(24 * time.Hour)
	if err := db.CamShow.CloseAccount(ctx, oldAccount.ID, closed); err != nil {
		t.Fatal(err)
	}
	currentFirst, err := db.CamShow.AddManualModelAccount(ctx, CamModelAccountInput{ModelID: model.ID, SiteID: firstSite.ID, Handle: "new_name", ValidFrom: &closed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CamShow.AddManualModelAccount(ctx, CamModelAccountInput{ModelID: model.ID, SiteID: secondSite.ID, Handle: "other_site", ValidFrom: &started}); err != nil {
		t.Fatal(err)
	}
	observed := time.Now().UTC().Truncate(time.Second)
	ingestInput := CamModelProvenanceInput{ModelID: model.ID, AccountID: &currentFirst.ID, Provider: "fixture-discovery", EvidenceKey: "observation-1", ProviderRecordID: stringPointer("unstable-person-7"), SourceURL: stringPointer("https://provider.test/profile"), ObservedAt: observed, PayloadJSON: `{"username":"new_name"}`, Confidence: floatPointer(0.75)}
	ingested, err := db.CamShow.IngestModelProvenance(ctx, ingestInput)
	if err != nil {
		t.Fatal(err)
	}
	if ingested.Status != CamModelProvenanceInserted {
		t.Fatalf("initial ingest status=%q", ingested.Status)
	}
	evidence := &ingested.Provenance
	if evidence.ReviewState != CamModelReviewPending {
		t.Fatalf("initial review state=%q", evidence.ReviewState)
	}
	repeated, err := db.CamShow.IngestModelProvenance(ctx, ingestInput)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Status != CamModelProvenanceUnchanged || repeated.Provenance.ID != evidence.ID {
		t.Fatalf("repeated ingest=%+v", repeated)
	}
	conflicting := ingestInput
	conflicting.PayloadJSON = `{"username":"different"}`
	if _, err := db.CamShow.IngestModelProvenance(ctx, conflicting); !errors.Is(err, ErrCamModelProvenanceConflict) {
		t.Fatalf("conflicting ingest error=%v", err)
	}
	reviewed, err := db.CamShow.ReviewModelProvenance(ctx, evidence.ID, int64(reviewer.ID), CamModelReviewApproved)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.ReviewState != CamModelReviewApproved || reviewed.ReviewedBy == nil || *reviewed.ReviewedBy != int64(reviewer.ID) || reviewed.ReviewedAt == nil {
		t.Fatalf("reviewed evidence=%+v", reviewed)
	}
	if _, err := db.CamShow.ReviewModelProvenance(ctx, evidence.ID, int64(reviewer.ID), CamModelReviewRejected); err == nil {
		t.Fatal("already-reviewed provenance changed state")
	}
	if _, err := db.CamShow.IngestModelProvenance(ctx, CamModelProvenanceInput{ModelID: model.ID, Provider: "fixture", EvidenceKey: "invalid-json", ObservedAt: observed, PayloadJSON: `{`}); err == nil {
		t.Fatal("invalid provenance JSON accepted")
	}
	profile, err := db.CamShow.FindModelProfile(ctx, model.ID)
	if err != nil {
		t.Fatal(err)
	}
	if profile == nil || len(profile.Accounts) != 3 || len(profile.Provenance) != 1 {
		t.Fatalf("profile=%+v", profile)
	}
	var current, historical int
	for _, account := range profile.Accounts {
		if account.ValidTo == nil {
			current++
		} else {
			historical++
		}
	}
	if current != 2 || historical != 1 {
		t.Fatalf("current=%d historical=%d accounts=%+v", current, historical, profile.Accounts)
	}
	for _, account := range profile.Accounts {
		if account.Source != "MANUAL" {
			t.Fatalf("public account seam wrote non-manual source: %+v", account)
		}
	}
	snapshot, err := db.CamShow.ExportSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != currentCamSnapshotVersion || camSnapshotRowCount(t, snapshot, "cam_model_profile_provenance") != 1 {
		t.Fatalf("migration-91 snapshot missing provenance: %+v", snapshot)
	}
	if err := db.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func stringPointer(value string) *string  { return &value }
func floatPointer(value float64) *float64 { return &value }

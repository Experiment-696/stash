package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
	"github.com/stashapp/stash/pkg/models"
)

func completedRepositoryItem(sceneID, siteID, modelID int64, rel string) cammodel.CompletedRecording {
	return cammodel.CompletedRecording{
		RelativePath: rel, ConfiguredRootID: completedImportHash("synthetic-root"),
		ParserVersion: "fixture-v1", CompletedAt: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
		Timezone: "UTC", TimePrecision: cammodel.CompletedTimeSecond,
		Fingerprint: cammodel.CompletedStatFingerprint{Size: 10, ModTimeNS: 20, Mode: 0o600, Device: 30, Inode: uint64(sceneID + 40)},
		SceneID:     sceneID, SiteID: siteID, ModelID: modelID,
		MatchState: cammodel.CompletedAliasCurrent, Outcome: cammodel.CompletedExactReady,
	}
}

func completedRepositoryAudit(item cammodel.CompletedRecording, outcome cammodel.CompletedImportOutcome, reason string) cammodel.CompletedImportAudit {
	return cammodel.CompletedImportAudit{
		ActorID: "1", PreviewID: strings.Repeat("a", 32), CandidateID: strings.Repeat("b", 64),
		RelativePathHash: completedImportHash(item.RelativePath), Outcome: string(outcome), Reason: reason,
		SceneID: item.SceneID, SiteID: item.SiteID, ModelID: item.ModelID,
		At: time.Date(2026, 7, 21, 13, 0, 0, 0, time.UTC),
	}
}

func TestCompletedImportRepositoryIdempotencyConflictRollbackAndRedactedAudit(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "completed-import.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	setup, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	scene1 := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	scene2 := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Scene.Create(setup, scene1, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Scene.Create(setup, scene2, nil); err != nil {
		t.Fatal(err)
	}
	site1, err := db.CamShow.CreateSite(setup, "Site One", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	site2, err := db.CamShow.CreateSite(setup, "Site Two", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model1, err := db.CamShow.CreateModel(setup, "Model One", nil)
	if err != nil {
		t.Fatal(err)
	}
	model2, err := db.CamShow.CreateModel(setup, "Model Two", nil)
	if err != nil {
		t.Fatal(err)
	}
	show1, err := db.CamShow.CreateShow(setup, int64(scene1.ID), "LIVE", nil)
	if err != nil {
		t.Fatal(err)
	}
	show2, err := db.CamShow.CreateShow(setup, int64(scene2.ID), "LIVE", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(setup); err != nil {
		t.Fatal(err)
	}

	repository := NewCompletedImportRepository(db)
	item1 := completedRepositoryItem(int64(scene1.ID), site1.ID, model1.ID, "cb/alice.mp4")
	audit1 := completedRepositoryAudit(item1, cammodel.CompletedApplied, "/private/root/cb/alice.mp4")
	if err := repository.WithCompletedImportTransaction(context.Background(), func(_ context.Context, tx cammodel.CompletedImportTx) error {
		applied, err := tx.LinkCamShowMetadata(context.Background(), item1)
		if err != nil || !applied {
			t.Fatalf("first link applied=%v err=%v", applied, err)
		}
		return tx.WriteCompletedImportAudit(context.Background(), audit1)
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.WithCompletedImportTransaction(context.Background(), func(_ context.Context, tx cammodel.CompletedImportTx) error {
		applied, err := tx.LinkCamShowMetadata(context.Background(), item1)
		if err != nil || applied {
			t.Fatalf("replay applied=%v err=%v", applied, err)
		}
		replay := audit1
		replay.Outcome = string(cammodel.CompletedAlreadyApplied)
		return tx.WriteCompletedImportAudit(context.Background(), replay)
	}); err != nil {
		t.Fatal(err)
	}

	read, err := db.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var imports, audits, sites, modelsCount int
	if err := dbWrapper.Get(read, &imports, "SELECT count(*) FROM cam_completed_recording_imports"); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(read, &audits, "SELECT count(*) FROM cam_completed_recording_audits"); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(read, &sites, "SELECT count(*) FROM cam_show_sites WHERE show_id=?", show1.ID); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(read, &modelsCount, "SELECT count(*) FROM cam_show_models WHERE show_id=?", show1.ID); err != nil {
		t.Fatal(err)
	}
	var stableID, storedReason string
	if err := dbWrapper.Get(read, &stableID, "SELECT id FROM cam_completed_recording_imports"); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(read, &storedReason, "SELECT redacted_reason FROM cam_completed_recording_audits ORDER BY id LIMIT 1"); err != nil {
		t.Fatal(err)
	}
	if imports != 1 || audits != 2 || sites != 1 || modelsCount != 1 || len(stableID) != 64 ||
		strings.Contains(storedReason, "alice") || !strings.HasPrefix(storedReason, "redacted:") {
		t.Fatalf("imports=%d audits=%d sites=%d models=%d stable=%q reason=%q", imports, audits, sites, modelsCount, stableID, storedReason)
	}

	conflict := item1
	conflict.ModelID = model2.ID
	err = repository.WithCompletedImportTransaction(context.Background(), func(_ context.Context, tx cammodel.CompletedImportTx) error {
		_, err := tx.LinkCamShowMetadata(context.Background(), conflict)
		return err
	})
	if !errors.Is(err, ErrCompletedImportConflict) {
		t.Fatalf("stable identity conflict=%v", err)
	}

	write, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(write, "CREATE TRIGGER completed_audit_fail BEFORE INSERT ON cam_completed_recording_audits BEGIN SELECT RAISE(ABORT,'injected audit failure'); END"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(write); err != nil {
		t.Fatal(err)
	}
	item2 := completedRepositoryItem(int64(scene2.ID), site2.ID, model2.ID, "other/bob.mp4")
	err = repository.WithCompletedImportTransaction(context.Background(), func(_ context.Context, tx cammodel.CompletedImportTx) error {
		applied, err := tx.LinkCamShowMetadata(context.Background(), item2)
		if err != nil || !applied {
			return errors.New("second link did not stage")
		}
		return tx.WriteCompletedImportAudit(context.Background(), completedRepositoryAudit(item2, cammodel.CompletedApplied, "staged"))
	})
	if err == nil {
		t.Fatal("injected audit failure committed")
	}
	read, _ = db.WithDatabase(context.Background())
	var show2Sites, show2Models int
	_ = dbWrapper.Get(read, &imports, "SELECT count(*) FROM cam_completed_recording_imports")
	_ = dbWrapper.Get(read, &show2Sites, "SELECT count(*) FROM cam_show_sites WHERE show_id=?", show2.ID)
	_ = dbWrapper.Get(read, &show2Models, "SELECT count(*) FROM cam_show_models WHERE show_id=?", show2.ID)
	if imports != 1 || show2Sites != 0 || show2Models != 0 {
		t.Fatalf("rollback imports=%d sites=%d models=%d", imports, show2Sites, show2Models)
	}

	write, _ = db.Begin(context.Background(), true)
	if _, err := dbWrapper.Exec(write, "DROP TRIGGER completed_audit_fail"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(write); err != nil {
		t.Fatal(err)
	}
	review := completedRepositoryAudit(item2, cammodel.CompletedReviewRequired, "unique historical alias requires review")
	review.ReviewCode = cammodel.CompletedReviewAliasReused
	if err := repository.WithCompletedImportTransaction(context.Background(), func(_ context.Context, tx cammodel.CompletedImportTx) error {
		return tx.WriteCompletedImportAudit(context.Background(), review)
	}); err != nil {
		t.Fatal(err)
	}
	read, _ = db.WithDatabase(context.Background())
	var outcome, code string
	if err := dbWrapper.Get(read, &outcome, "SELECT outcome FROM cam_completed_recording_audits ORDER BY id DESC LIMIT 1"); err != nil {
		t.Fatal(err)
	}
	if err := dbWrapper.Get(read, &code, "SELECT review_reason_code FROM cam_completed_recording_audits ORDER BY id DESC LIMIT 1"); err != nil {
		t.Fatal(err)
	}
	if outcome != "REVIEW_REQUIRED" || code != "HISTORICAL_ALIAS_REUSED" {
		t.Fatalf("review outcome=%q code=%q", outcome, code)
	}
}

package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestCamClassificationPreviewApplyAndConflicts(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "classification.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(ctx)
	tag := &models.Tag{Name: "Cam Show", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Tag.Create(ctx, &models.CreateTagInput{Tag: tag}); err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Scene.Create(ctx, scene, nil); err != nil {
		t.Fatal(err)
	}
	candidate := CamClassificationCandidate{SceneID: int64(scene.ID), Basename: "2026-05-23 18-56-43.MP4", RelativePath: `./captures\\site\\2026-05-23 18-56-43.MP4`}
	if _, err := db.CamShow.CreateClassificationRule(ctx, "bad", "[", CamClassificationTargetBasename, "RECORDED", true, nil); err == nil {
		t.Fatal("invalid regex accepted")
	}
	rule, err := db.CamShow.CreateClassificationRule(ctx, "timestamp", `^\d{4}-\d{2}-\d{2} \d{2}-\d{2}-\d{2}\.mp4$`, CamClassificationTargetBasename, "RECORDED", true, []int{tag.ID, tag.ID})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := db.CamShow.PreviewClassification(ctx, []CamClassificationCandidate{candidate})
	if err != nil || preview.Matched != 1 || preview.Applied != 0 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if show, _ := db.CamShow.FindShowByScene(ctx, int64(scene.ID)); show != nil {
		t.Fatal("dry-run wrote metadata")
	}
	for run := 0; run < 2; run++ {
		got, err := db.CamShow.ApplyClassification(ctx, []CamClassificationCandidate{candidate})
		if err != nil || got.Applied != 1 {
			t.Fatalf("apply %d=%+v err=%v", run, got, err)
		}
	}
	shows, _ := db.CamShow.ListShows(ctx)
	tags, _ := db.Scene.GetTagIDs(ctx, scene.ID)
	if len(shows) != 1 || len(tags) != 1 {
		t.Fatalf("shows=%d tags=%v", len(shows), tags)
	}
	if err := db.CamShow.SetClassificationRuleEnabled(ctx, rule.ID, false); err != nil {
		t.Fatal(err)
	}
	disabled, err := db.CamShow.PreviewClassification(ctx, []CamClassificationCandidate{candidate})
	if err != nil || disabled.Matched != 0 {
		t.Fatalf("disabled=%+v err=%v", disabled, err)
	}

	scene2 := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Scene.Create(ctx, scene2, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CamShow.CreateClassificationRule(ctx, "path-a", `^captures/site/.+\.mp4$`, CamClassificationTargetRelativePath, "RECORDED", true, []int{tag.ID}); err != nil {
		t.Fatal(err)
	}
	conflictingRule, err := db.CamShow.CreateClassificationRule(ctx, "path-b", `^CAPTURES/SITE/.+\.MP4$`, CamClassificationTargetRelativePath, "ARCHIVE", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := db.CamShow.ApplyClassification(ctx, []CamClassificationCandidate{{SceneID: int64(scene2.ID), RelativePath: candidate.RelativePath}})
	if err != nil || conflict.Conflicted != 1 || conflict.Applied != 0 {
		t.Fatalf("conflict=%+v err=%v", conflict, err)
	}
	if show, _ := db.CamShow.FindShowByScene(ctx, int64(scene2.ID)); show != nil {
		t.Fatal("conflict wrote metadata")
	}

	if err := db.CamShow.SetClassificationRuleEnabled(ctx, conflictingRule.ID, false); err != nil {
		t.Fatal(err)
	}
	scene3 := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Scene.Create(ctx, scene3, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CamShow.CreateShow(ctx, int64(scene3.ID), "MANUAL", nil); err != nil {
		t.Fatal(err)
	}
	existingConflict, err := db.CamShow.ApplyClassification(ctx, []CamClassificationCandidate{{SceneID: int64(scene3.ID), RelativePath: candidate.RelativePath}})
	if err != nil || existingConflict.Conflicted != 1 || existingConflict.Applied != 0 {
		t.Fatalf("existing category conflict=%+v err=%v", existingConflict, err)
	}
	existingShow, err := db.CamShow.FindShowByScene(ctx, int64(scene3.ID))
	if err != nil || existingShow == nil || existingShow.Category != "MANUAL" {
		t.Fatalf("existing category mutated: show=%+v err=%v", existingShow, err)
	}
	existingTags, err := db.Scene.GetTagIDs(ctx, scene3.ID)
	if err != nil || len(existingTags) != 0 {
		t.Fatalf("existing category conflict added tags=%v err=%v", existingTags, err)
	}
}

func TestCamClassificationRelativePathIsRootBounded(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "library")
	nested := filepath.Join(root, "nested")
	full := filepath.Join(nested, "captures", "scene.mp4")
	relative, ok := camClassificationRelativePath(full, []string{root, nested})
	if !ok || relative != "captures/scene.mp4" {
		t.Fatalf("relative=%q ok=%v", relative, ok)
	}
	if relative, ok := camClassificationRelativePath(filepath.Join(root, "..", "outside.mp4"), []string{root}); ok || relative != "" {
		t.Fatalf("out-of-root relative=%q ok=%v", relative, ok)
	}
}

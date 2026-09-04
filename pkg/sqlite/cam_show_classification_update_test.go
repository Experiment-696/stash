package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
)

func TestUpdateClassificationRule(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "classification-update.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(ctx)
	rule, err := db.CamShow.CreateClassificationRule(ctx, "before", "^before$", CamClassificationTargetBasename, "RECORDED", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CamShow.UpdateClassificationRule(ctx, rule.ID, "invalid", "[", CamClassificationTargetBasename, "RECORDED", true, nil); err == nil {
		t.Fatal("invalid update regex accepted")
	}
	updated, err := db.CamShow.UpdateClassificationRule(ctx, rule.ID, "after", "^folder/after$", CamClassificationTargetRelativePath, "ARCHIVE", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "after" || updated.Pattern != "^folder/after$" || updated.Target != CamClassificationTargetRelativePath || updated.Category != "ARCHIVE" || updated.Enabled {
		t.Fatalf("updated=%+v", updated)
	}
}

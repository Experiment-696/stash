package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestSceneFilterExcludeCamShowsSeparatesLibraryWithoutBreakingSceneBridge(t *testing.T) {
	config.InitializeEmpty()
	database := NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "scene-show-separation.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	tx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(tx)

	now := time.Now().UTC()
	ordinary := &models.Scene{Title: "Ordinary Scene", CreatedAt: now, UpdatedAt: now}
	showBridge := &models.Scene{Title: "Cam Show Bridge", CreatedAt: now, UpdatedAt: now}
	if err := database.Scene.Create(tx, ordinary, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Scene.Create(tx, showBridge, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CamShow.CreateShow(tx, int64(showBridge.ID), "OTHER", nil); err != nil {
		t.Fatal(err)
	}

	exclude := true
	result, err := database.Scene.Query(tx, models.SceneQueryOptions{
		SceneFilter: &models.SceneFilterType{ExcludeCamShows: &exclude},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.IDs) != 1 || result.IDs[0] != ordinary.ID {
		t.Fatalf("ordinary library IDs=%v want only %d", result.IDs, ordinary.ID)
	}

	include := false
	all, err := database.Scene.Query(tx, models.SceneQueryOptions{
		SceneFilter: &models.SceneFilterType{ExcludeCamShows: &include},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.IDs) != 2 {
		t.Fatalf("unseparated query IDs=%v want both Scene records", all.IDs)
	}

	bridge, err := database.Scene.Find(tx, showBridge.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bridge == nil || bridge.ID != showBridge.ID {
		t.Fatalf("direct Scene player bridge missing: %+v", bridge)
	}
}

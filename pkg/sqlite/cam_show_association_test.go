package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestSetShowAssociationsReplacesSitesAndOrderedModels(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "show-associations.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(tx)

	now := time.Now().UTC()
	scene := &models.Scene{CreatedAt: now, UpdatedAt: now}
	if err := db.Scene.Create(tx, scene, nil); err != nil {
		t.Fatal(err)
	}
	show, err := db.CamShow.CreateShow(tx, int64(scene.ID), "OTHER", nil)
	if err != nil {
		t.Fatal(err)
	}
	siteA, _ := db.CamShow.CreateSite(tx, "Site A", nil, nil)
	siteB, _ := db.CamShow.CreateSite(tx, "Site B", nil, nil)
	modelA, _ := db.CamShow.CreateModel(tx, "Model A", nil)
	modelB, _ := db.CamShow.CreateModel(tx, "Model B", nil)

	first := []CamShowModelAssignment{{ModelID: modelA.ID, Role: "PRIMARY"}, {ModelID: modelB.ID, Role: "GUEST"}}
	if err := db.CamShow.SetShowAssociations(tx, show.ID, []int64{siteA.ID, siteB.ID}, first); err != nil {
		t.Fatal(err)
	}
	second := []CamShowModelAssignment{{ModelID: modelB.ID, Role: "SOLO"}}
	if err := db.CamShow.SetShowAssociations(tx, show.ID, []int64{siteB.ID}, second); err != nil {
		t.Fatal(err)
	}
	shows, err := db.CamShow.ListShowDomain(tx)
	if err != nil || len(shows) != 1 {
		t.Fatalf("shows=%+v err=%v", shows, err)
	}
	if len(shows[0].Sites) != 1 || shows[0].Sites[0].ID != siteB.ID {
		t.Fatalf("sites=%+v", shows[0].Sites)
	}
	if len(shows[0].Models) != 1 || shows[0].Models[0].ModelID != modelB.ID || shows[0].Models[0].Role != "SOLO" {
		t.Fatalf("models=%+v", shows[0].Models)
	}
	if err := db.CamShow.SetShowAssociations(tx, show.ID, nil, []CamShowModelAssignment{{ModelID: modelB.ID, Role: "BAD"}}); err == nil {
		t.Fatal("invalid role was accepted")
	}
	if err := db.CamShow.SetShowAssociations(tx, show.ID, []int64{siteB.ID, siteB.ID}, nil); err == nil {
		t.Fatal("duplicate site was accepted")
	}
}

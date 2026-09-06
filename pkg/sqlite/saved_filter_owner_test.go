package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestSavedFilterStoreOwnerIsolationNamesAndDefaults(t *testing.T) {
	config.InitializeEmpty()
	database := NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "saved-filter-owners.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	tx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := database.User.Create(tx, "saved-filter-owner-one", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.User.Create(tx, "saved-filter-owner-two", "password-two", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	firstFilter := &models.SavedFilter{UserID: &first.ID, Mode: models.FilterModeScenes, Name: "Same name"}
	secondFilter := &models.SavedFilter{UserID: &second.ID, Mode: models.FilterModeScenes, Name: "Same name"}
	if err := database.SavedFilter.Create(tx, firstFilter); err != nil {
		t.Fatal(err)
	}
	if err := database.SavedFilter.Create(tx, secondFilter); err != nil {
		t.Fatalf("same filter name for another owner was rejected: %v", err)
	}
	duplicate := &models.SavedFilter{UserID: &first.ID, Mode: models.FilterModeScenes, Name: "Same name"}
	if err := database.SavedFilter.Create(tx, duplicate); err == nil {
		t.Fatal("same owner was allowed a duplicate mode/name")
	}
	if err := database.SavedFilter.SetDefaultForUser(tx, first.ID, models.FilterModeScenes, &firstFilter.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.SavedFilter.SetDefaultForUser(tx, second.ID, models.FilterModeScenes, &firstFilter.ID); err == nil {
		t.Fatal("foreign filter was accepted as another owner's default")
	}
	if err := database.Commit(tx); err != nil {
		t.Fatal(err)
	}

	read, err := database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	firstOnly, err := database.SavedFilter.AllForUser(read, first.ID)
	if err != nil || len(firstOnly) != 1 || firstOnly[0].ID != firstFilter.ID {
		t.Fatalf("first owner list=%+v err=%v", firstOnly, err)
	}
	if foreign, err := database.SavedFilter.FindForUser(read, secondFilter.ID, first.ID); err != nil || foreign != nil {
		t.Fatalf("foreign lookup returned filter=%+v err=%v", foreign, err)
	}
	def, err := database.SavedFilter.FindDefaultForUser(read, first.ID, models.FilterModeScenes)
	if err != nil || def == nil || def.ID != firstFilter.ID {
		t.Fatalf("first default=%+v err=%v", def, err)
	}
	_ = database.Rollback(read)

	tx, err = database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SavedFilter.SetDefaultForUser(tx, first.ID, models.FilterModeScenes, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Commit(tx); err != nil {
		t.Fatal(err)
	}
	read, err = database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(read)
	if def, err := database.SavedFilter.FindDefaultForUser(read, first.ID, models.FilterModeScenes); err != nil || def != nil {
		t.Fatalf("cleared default=%+v err=%v", def, err)
	}
}

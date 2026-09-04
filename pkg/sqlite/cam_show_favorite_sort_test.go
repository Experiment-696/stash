package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestCamShowFavoriteModelsOrderingIsUserScopedStableAndDuplicateFree(t *testing.T) {
	config.InitializeEmpty()
	database := NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "favorite-show-sort.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	tx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(tx)

	first, err := database.User.Create(tx, "show-sort-first", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.User.Create(tx, "show-sort-second", "password-two", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	favoriteA, err := database.CamShow.CreateModel(tx, "Favorite A", nil)
	if err != nil {
		t.Fatal(err)
	}
	favoriteB, err := database.CamShow.CreateModel(tx, "Favorite B", nil)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := database.CamShow.CreateModel(tx, "Ordinary", nil)
	if err != nil {
		t.Fatal(err)
	}

	createShow := func(title string, date time.Time) *CamShow {
		t.Helper()
		scene := &models.Scene{Title: title, CreatedAt: date, UpdatedAt: date}
		if err := database.Scene.Create(tx, scene, nil); err != nil {
			t.Fatal(err)
		}
		show, err := database.CamShow.CreateShow(tx, int64(scene.ID), "LIVE", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dbWrapper.Exec(tx, `UPDATE cam_shows SET show_date=? WHERE id=?`, date, show.ID); err != nil {
			t.Fatal(err)
		}
		return show
	}
	oldest := createShow("Old favorite", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	middle := createShow("Middle", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	newest := createShow("Newest", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	capturedOnly := createShow("Captured only", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	if _, err := dbWrapper.Exec(tx, `UPDATE cam_shows SET show_date=NULL,captured_at=? WHERE id=?`, time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC), capturedOnly.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.CamShow.LinkModelWithRole(tx, oldest.ID, favoriteA.ID, 0, "PRIMARY"); err != nil {
		t.Fatal(err)
	}
	if err := database.CamShow.LinkModelWithRole(tx, oldest.ID, favoriteB.ID, 1, "GUEST"); err != nil {
		t.Fatal(err)
	}
	if err := database.CamShow.LinkModelWithRole(tx, newest.ID, ordinary.ID, 0, "PRIMARY"); err != nil {
		t.Fatal(err)
	}
	if err := database.CamShow.SetUserState(tx, first.ID, favoriteA.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.CamShow.SetUserState(tx, first.ID, favoriteB.ID, true, nil); err != nil {
		t.Fatal(err)
	}

	assertOrder := func(label string, got []CamShowDomainItem, want ...int64) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s count=%d want=%d", label, len(got), len(want))
		}
		for i := range want {
			if got[i].ID != want[i] {
				t.Fatalf("%s order[%d]=%d want=%d", label, i, got[i].ID, want[i])
			}
		}
	}
	firstSorted, err := database.CamShow.ListShowDomainForUser(tx, first.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder("first user", firstSorted, oldest.ID, newest.ID, middle.ID, capturedOnly.ID)
	if !firstSorted[0].HasFavoriteModel || firstSorted[1].HasFavoriteModel || firstSorted[2].HasFavoriteModel || firstSorted[3].HasFavoriteModel {
		t.Fatalf("first user favorite flags=%v,%v,%v,%v", firstSorted[0].HasFavoriteModel, firstSorted[1].HasFavoriteModel, firstSorted[2].HasFavoriteModel, firstSorted[3].HasFavoriteModel)
	}

	secondSorted, err := database.CamShow.ListShowDomainForUser(tx, second.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder("second user", secondSorted, newest.ID, middle.ID, capturedOnly.ID, oldest.ID)
	normal, err := database.CamShow.ListShowDomainForUser(tx, first.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder("normal", normal, newest.ID, middle.ID, capturedOnly.ID, oldest.ID)
	if _, err := database.CamShow.ListShowDomainForUser(tx, 0, true); err == nil {
		t.Fatal("Favorite Models ordering accepted without a persisted user")
	}
	if _, err := database.CamShow.ListShowDomainForUser(tx, 999999, true); err == nil {
		t.Fatal("Favorite Models ordering accepted a nonexistent persisted user")
	}
}

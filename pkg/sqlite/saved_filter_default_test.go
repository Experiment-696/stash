package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestSavedFilterDefaultsOwnerIsolationCleanupRestartAndConcurrency(t *testing.T) {
	config.InitializeEmpty()
	path := filepath.Join(t.TempDir(), "default-filters.sqlite")
	database := NewDatabase()
	if err := database.Open(path); err != nil {
		t.Fatal(err)
	}

	ctx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := database.User.Create(ctx, "defaults-one", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.User.Create(ctx, "defaults-two", "password-two", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	firstFilter := &models.SavedFilter{UserID: &first.ID, Mode: models.FilterModeScenes, Name: "first"}
	secondFilter := &models.SavedFilter{UserID: &second.ID, Mode: models.FilterModeScenes, Name: "second"}
	legacyNull := &models.SavedFilter{Mode: models.FilterModeScenes, Name: "legacy-null"}
	for _, filter := range []*models.SavedFilter{firstFilter, secondFilter, legacyNull} {
		if err := database.SavedFilter.Create(ctx, filter); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.SavedFilter.SetDefaultForUser(ctx, first.ID, models.FilterModeScenes, &firstFilter.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.SavedFilter.SetDefaultForUser(ctx, second.ID, models.FilterModeScenes, &secondFilter.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.SavedFilter.SetDefaultForUser(ctx, first.ID, models.FilterModeScenes, &secondFilter.ID); err == nil {
		t.Fatal("foreign filter accepted as first user's default")
	}
	if err := database.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	read, err := database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	gotFirst, err := database.SavedFilter.FindDefaultForUser(read, first.ID, models.FilterModeScenes)
	if err != nil || gotFirst == nil || gotFirst.ID != firstFilter.ID {
		t.Fatalf("first default=%v err=%v", gotFirst, err)
	}
	gotSecond, err := database.SavedFilter.FindDefaultForUser(read, second.ID, models.FilterModeScenes)
	if err != nil || gotSecond == nil || gotSecond.ID != secondFilter.ID {
		t.Fatalf("second default=%v err=%v", gotSecond, err)
	}
	_ = database.Rollback(read)
	database.Close()

	database = NewDatabase()
	if err := database.Open(path); err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	read, err = database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	gotFirst, err = database.SavedFilter.FindDefaultForUser(read, first.ID, models.FilterModeScenes)
	if err != nil || gotFirst == nil || gotFirst.ID != firstFilter.ID {
		t.Fatalf("default did not survive restart: %v err=%v", gotFirst, err)
	}
	_ = database.Rollback(read)

	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, beginErr := database.Begin(context.Background(), true)
			if beginErr == nil {
				beginErr = database.SavedFilter.SetDefaultForUser(tx, first.ID, models.FilterModeScenes, &firstFilter.ID)
			}
			if beginErr == nil {
				beginErr = database.Commit(tx)
			} else if tx != nil {
				_ = database.Rollback(tx)
			}
			errs <- beginErr
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	withActivityWrite(t, database, func(tx context.Context) error {
		return database.SavedFilter.DestroyForUser(tx, firstFilter.ID, first.ID)
	})
	read, err = database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(read)
	gotFirst, err = database.SavedFilter.FindDefaultForUser(read, first.ID, models.FilterModeScenes)
	if err != nil || gotFirst != nil {
		t.Fatalf("destroyed default was not cleared: %v err=%v", gotFirst, err)
	}
	gotSecond, err = database.SavedFilter.FindDefaultForUser(read, second.ID, models.FilterModeScenes)
	if err != nil || gotSecond == nil || gotSecond.ID != secondFilter.ID {
		t.Fatalf("first cleanup changed second default: %v err=%v", gotSecond, err)
	}
}

package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func activityTestDatabase(t *testing.T) (*Database, int64, int64, int64) {
	t.Helper()
	config.InitializeEmpty()
	database := NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "activity.sqlite")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	ctx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := database.User.Create(ctx, "activity-one", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.User.Create(ctx, "activity-two", "password-two", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.Scene.Create(ctx, scene, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return database, first.ID, second.ID, int64(scene.ID)
}

func withActivityWrite(t *testing.T, database *Database, fn func(context.Context) error) {
	t.Helper()
	ctx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := fn(ctx); err != nil {
		_ = database.Rollback(ctx)
		t.Fatal(err)
	}
	if err := database.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestActivitySceneHistoryIsolationTimestampsResetAndCascade(t *testing.T) {
	database, first, second, sceneID := activityTestDatabase(t)
	store := database.Activity
	when := time.Date(2026, 7, 15, 12, 34, 56, 0, time.UTC)

	withActivityWrite(t, database, func(ctx context.Context) error {
		if _, err := store.AddSceneHistory(ctx, first, sceneID, SceneHistoryPlay, []time.Time{when, when}); err != nil {
			return err
		}
		_, err := store.AddSceneHistory(ctx, second, sceneID, SceneHistoryPlay, []time.Time{when.Add(time.Hour)})
		return err
	})

	withActivityWrite(t, database, func(ctx context.Context) error {
		remaining, err := store.DeleteSceneHistory(ctx, first, sceneID, SceneHistoryPlay, []time.Time{when})
		if err != nil {
			return err
		}
		if len(remaining) != 1 || !remaining[0].Equal(when) {
			t.Fatalf("timestamp delete removed wrong rows: %v", remaining)
		}
		// Repeated deletion is idempotent and never affects the other owner.
		if _, err := store.DeleteSceneHistory(ctx, first, sceneID, SceneHistoryPlay, []time.Time{when.Add(24 * time.Hour)}); err != nil {
			return err
		}
		if err := store.ResetSceneHistory(ctx, first, sceneID, SceneHistoryPlay); err != nil {
			return err
		}
		return store.ResetSceneHistory(ctx, first, sceneID, SceneHistoryPlay)
	})

	withActivityWrite(t, database, func(ctx context.Context) error {
		other, err := store.SceneHistory(ctx, second, sceneID, SceneHistoryPlay)
		if err != nil {
			return err
		}
		if len(other) != 1 {
			t.Fatalf("first user's reset changed second user's history: %v", other)
		}
		_, err = dbWrapper.Exec(ctx, `DELETE FROM scenes WHERE id = ?`, sceneID)
		if err != nil {
			return err
		}
		other, err = store.SceneHistory(ctx, second, sceneID, SceneHistoryPlay)
		if err == nil && len(other) != 0 {
			t.Fatalf("scene cascade left history: %v", other)
		}
		return err
	})
}

func TestActivitySceneHistoryConcurrentSameScene(t *testing.T) {
	database, userID, _, sceneID := activityTestDatabase(t)
	const writes = 12
	var wg sync.WaitGroup
	errs := make(chan error, writes)
	for i := 0; i < writes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, err := database.Begin(context.Background(), true)
			if err == nil {
				_, err = database.Activity.AddSceneHistory(ctx, userID, sceneID, SceneHistoryO, []time.Time{time.Unix(int64(i+1), 0).UTC()})
			}
			if err == nil {
				err = database.Commit(ctx)
			} else if ctx != nil {
				_ = database.Rollback(ctx)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	ctx, err := database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(ctx)
	history, err := database.Activity.SceneHistory(ctx, userID, sceneID, SceneHistoryO)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != writes {
		t.Fatalf("concurrent history count=%d want=%d", len(history), writes)
	}
}

func TestPersonalOCounterReadsFiltersSortsAndTotals(t *testing.T) {
	database, first, second, firstSceneID := activityTestDatabase(t)
	var secondSceneID, firstImageID, secondImageID int64

	withActivityWrite(t, database, func(ctx context.Context) error {
		now := time.Now().UTC()
		result, err := dbWrapper.Exec(ctx, `INSERT INTO scenes(created_at, updated_at) VALUES (?, ?)`, now, now)
		if err != nil {
			return err
		}
		secondSceneID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		result, err = dbWrapper.Exec(ctx, `INSERT INTO images(created_at, updated_at) VALUES (?, ?)`, now, now)
		if err != nil {
			return err
		}
		firstImageID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		result, err = dbWrapper.Exec(ctx, `INSERT INTO images(created_at, updated_at) VALUES (?, ?)`, now, now)
		if err != nil {
			return err
		}
		secondImageID, err = result.LastInsertId()
		if err != nil {
			return err
		}

		if _, err = database.Activity.AddSceneHistory(ctx, first, firstSceneID, SceneHistoryO, []time.Time{now, now.Add(time.Second)}); err != nil {
			return err
		}
		if _, err = database.Activity.AddSceneHistory(ctx, first, secondSceneID, SceneHistoryO, []time.Time{now}); err != nil {
			return err
		}
		if _, err = database.Activity.AddSceneHistory(ctx, second, secondSceneID, SceneHistoryO, []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}); err != nil {
			return err
		}
		if _, err = database.Activity.ChangeImageO(ctx, first, firstImageID, 4); err != nil {
			return err
		}
		if _, err = database.Activity.ChangeImageO(ctx, first, secondImageID, 1); err != nil {
			return err
		}
		_, err = database.Activity.ChangeImageO(ctx, second, secondImageID, 7)
		return err
	})

	assertOwner := func(t *testing.T, userID int64, wantSceneCounts, wantImageCounts []int, wantSceneTotal, wantImageTotal int) {
		t.Helper()
		principal := authz.Principal{UserID: strconv.FormatInt(userID, 10), Role: authz.RoleUser, Status: authz.StatusActive}
		ctx, err := database.Begin(authz.WithPrincipal(context.Background(), principal), false)
		if err != nil {
			t.Fatal(err)
		}
		defer database.Rollback(ctx)

		counts, err := database.Scene.GetManyOCount(ctx, []int{int(firstSceneID), int(secondSceneID)})
		if err != nil || !reflect.DeepEqual(counts, wantSceneCounts) {
			t.Fatalf("scene counts=%v err=%v want=%v", counts, err, wantSceneCounts)
		}
		total, err := database.Scene.GetAllOCount(ctx)
		if err != nil || total != wantSceneTotal {
			t.Fatalf("scene total=%d err=%v want=%d", total, err, wantSceneTotal)
		}
		for i, imageID := range []int64{firstImageID, secondImageID} {
			got, err := database.Activity.ImageO(ctx, userID, imageID)
			if err != nil || got != wantImageCounts[i] {
				t.Fatalf("image %d count=%d err=%v want=%d", imageID, got, err, wantImageCounts[i])
			}
		}
		imageTotal, err := database.Image.OCount(ctx)
		if err != nil || imageTotal != wantImageTotal {
			t.Fatalf("image total=%d err=%v want=%d", imageTotal, err, wantImageTotal)
		}

		threshold := models.IntCriterionInput{Value: 2, Modifier: models.CriterionModifierGreaterThan}
		sceneResult, err := database.Scene.Query(ctx, models.SceneQueryOptions{SceneFilter: &models.SceneFilterType{OCounter: &threshold}})
		if err != nil {
			t.Fatal(err)
		}
		scenes, err := sceneResult.Resolve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wantFiltered := 0
		for _, count := range wantSceneCounts {
			if count > 2 {
				wantFiltered++
			}
		}
		if len(scenes) != wantFiltered {
			t.Fatalf("filtered scenes=%d want=%d", len(scenes), wantFiltered)
		}

		sort := "o_counter"
		direction := models.SortDirectionEnumDesc
		imageResult, err := database.Image.Query(ctx, models.ImageQueryOptions{QueryOptions: models.QueryOptions{FindFilter: &models.FindFilterType{Sort: &sort, Direction: &direction}}})
		if err != nil {
			t.Fatal(err)
		}
		images, err := imageResult.Resolve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wantFirst := firstImageID
		if wantImageCounts[1] > wantImageCounts[0] {
			wantFirst = secondImageID
		}
		if len(images) < 2 || int64(images[0].ID) != wantFirst {
			t.Fatalf("image sort first id=%v want=%d", images[0].ID, wantFirst)
		}
	}

	assertOwner(t, first, []int{2, 1}, []int{4, 1}, 3, 5)
	assertOwner(t, second, []int{0, 3}, []int{0, 7}, 3, 7)
}

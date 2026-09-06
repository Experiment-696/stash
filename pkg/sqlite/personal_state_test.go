package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

func TestPerformerPersonalStateTwoUserIsolationAggregateAndCascade(t *testing.T) {
	config.InitializeEmpty()
	database := NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "personal-state.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	tx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := database.User.Create(tx, "state-one", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.User.Create(tx, "state-two", "password-two", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	performer := models.NewPerformer()
	performer.Name = "Personal State"
	if err := database.Performer.Create(tx, &models.CreatePerformerInput{Performer: &performer}); err != nil {
		t.Fatal(err)
	}
	firstRating, secondRating := 80, 35
	if err := database.PersonalState.SetPerformerFavorite(tx, first.ID, int64(performer.ID), true); err != nil {
		t.Fatal(err)
	}
	if err := database.PersonalState.SetPerformerRating(tx, first.ID, int64(performer.ID), &firstRating); err != nil {
		t.Fatal(err)
	}
	if err := database.PersonalState.SetPerformerRating(tx, second.ID, int64(performer.ID), &secondRating); err != nil {
		t.Fatal(err)
	}
	if err := database.Commit(tx); err != nil {
		t.Fatal(err)
	}

	read, err := database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	firstState, err := database.PersonalState.Performer(read, first.ID, int64(performer.ID))
	if err != nil {
		t.Fatal(err)
	}
	secondState, err := database.PersonalState.Performer(read, second.ID, int64(performer.ID))
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Rollback(read)
	if !firstState.Favorite || firstState.Rating == nil || *firstState.Rating != firstRating {
		t.Fatalf("first state=%+v", firstState)
	}
	if secondState.Favorite || secondState.Rating == nil || *secondState.Rating != secondRating {
		t.Fatalf("second state=%+v", secondState)
	}
	if firstState.RatingSum != 115 || secondState.RatingSum != 115 {
		t.Fatalf("rating sums first=%d second=%d", firstState.RatingSum, secondState.RatingSum)
	}
	if firstState.RatingCount != 2 || secondState.RatingCount != 2 {
		t.Fatalf("rating counts first=%d second=%d", firstState.RatingCount, secondState.RatingCount)
	}
	if firstState.RatingAverage != 57.5 || secondState.RatingAverage != 57.5 {
		t.Fatalf("rating averages first=%v second=%v", firstState.RatingAverage, secondState.RatingAverage)
	}

	withActivityWrite(t, database, func(ctx context.Context) error {
		_, err := dbWrapper.Exec(ctx, `DELETE FROM performers WHERE id = ?`, performer.ID)
		return err
	})
	read, err = database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(read)
	var count int
	if err := dbWrapper.Get(read, &count, `SELECT count(*) FROM user_performer_state WHERE performer_id = ?`, performer.ID); err != nil || count != 0 {
		t.Fatalf("cascade count=%d err=%v", count, err)
	}
}

func TestScenePersonalStateTwoUserIsolationAggregateClearAndCascade(t *testing.T) {
	config.InitializeEmpty()
	database := NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "scene-personal-state.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	tx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := database.User.Create(tx, "scene-state-one", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.User.Create(tx, "scene-state-two", "password-two", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	scene := models.NewScene()
	if err := database.Scene.Create(tx, &scene, nil); err != nil {
		t.Fatal(err)
	}
	firstRating, secondRating := 90, 35
	if err := database.PersonalState.SetSceneRating(tx, first.ID, int64(scene.ID), &firstRating); err != nil {
		t.Fatal(err)
	}
	if err := database.PersonalState.SetSceneRating(tx, second.ID, int64(scene.ID), &secondRating); err != nil {
		t.Fatal(err)
	}
	if err := database.Commit(tx); err != nil {
		t.Fatal(err)
	}

	read, err := database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	firstState, err := database.PersonalState.Scene(read, first.ID, int64(scene.ID))
	if err != nil {
		t.Fatal(err)
	}
	secondState, err := database.PersonalState.Scene(read, second.ID, int64(scene.ID))
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Rollback(read)
	if firstState.Rating == nil || *firstState.Rating != firstRating {
		t.Fatalf("first state=%+v", firstState)
	}
	if secondState.Rating == nil || *secondState.Rating != secondRating {
		t.Fatalf("second state=%+v", secondState)
	}
	if firstState.RatingCount != 2 || firstState.RatingAverage != 62.5 {
		t.Fatalf("aggregate=%+v", firstState)
	}

	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		return database.PersonalState.SetSceneRating(ctx, first.ID, int64(scene.ID), nil)
	}); err != nil {
		t.Fatal(err)
	}
	read, err = database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := database.PersonalState.Scene(read, first.ID, int64(scene.ID))
	_ = database.Rollback(read)
	if err != nil || cleared.Rating != nil || cleared.RatingCount != 1 || cleared.RatingAverage != 35 {
		t.Fatalf("cleared state=%+v err=%v", cleared, err)
	}

	withActivityWrite(t, database, func(ctx context.Context) error {
		_, err := dbWrapper.Exec(ctx, `DELETE FROM scenes WHERE id = ?`, scene.ID)
		return err
	})
	read, err = database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(read)
	var count int
	if err := dbWrapper.Get(read, &count, `SELECT count(*) FROM user_scene_state WHERE scene_id = ?`, scene.ID); err != nil || count != 0 {
		t.Fatalf("cascade count=%d err=%v", count, err)
	}
}

func TestPersonalRatingValidation(t *testing.T) {
	for _, rating := range []int{0, 101} {
		if err := validatePersonalRating(&rating); err == nil {
			t.Fatalf("rating %d accepted", rating)
		}
	}
}

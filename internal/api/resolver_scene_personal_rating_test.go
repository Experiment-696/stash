package api

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

func TestScenePersonalRatingResolversIsolateUsersAndDenyReducedPrincipals(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	first := createResolverUser(t, database, "scene-rating-one", authz.RoleUser)
	second := createResolverUser(t, database, "scene-rating-two", authz.RoleUser)
	scene := models.NewScene()
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		return database.Scene.Create(ctx, &scene, nil)
	}); err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{database: database, repository: database.Repository()}
	mutations := &mutationResolver{Resolver: resolver}
	fields := &sceneResolver{Resolver: resolver}
	sceneID := strconv.Itoa(scene.ID)
	firstCtx := authz.WithPrincipal(context.Background(), first)
	secondCtx := authz.WithPrincipal(context.Background(), second)
	firstRating, secondRating := 95, 40
	if _, err := mutations.SceneSetRating(firstCtx, sceneID, &firstRating); err != nil {
		t.Fatal(err)
	}
	if _, err := mutations.SceneSetRating(secondCtx, sceneID, &secondRating); err != nil {
		t.Fatal(err)
	}
	firstPersonalRating, err := fields.Rating100(firstCtx, &scene)
	if err != nil || firstPersonalRating == nil || *firstPersonalRating != firstRating {
		t.Fatalf("first rating=%v err=%v", firstPersonalRating, err)
	}
	secondPersonalRating, err := fields.Rating100(secondCtx, &scene)
	if err != nil || secondPersonalRating == nil || *secondPersonalRating != secondRating {
		t.Fatalf("second rating=%v err=%v", secondPersonalRating, err)
	}
	average, err := fields.Rating100Average(firstCtx, &scene)
	if err != nil || average != 67.5 {
		t.Fatalf("average=%v err=%v", average, err)
	}
	count, err := fields.Rating100Count(secondCtx, &scene)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}

	disabled := first
	disabled.Status = authz.StatusDisabled
	if _, err := mutations.SceneSetRating(authz.WithPrincipal(context.Background(), disabled), sceneID, nil); err == nil {
		t.Fatal("disabled principal changed rating")
	}
	reduced := first
	reduced.TokenScopes = map[authz.Capability]struct{}{authz.LibraryRead: {}}
	if _, err := mutations.SceneSetRating(authz.WithPrincipal(context.Background(), reduced), sceneID, nil); err == nil {
		t.Fatal("reduced token changed rating")
	}
}

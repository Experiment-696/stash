package api

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

func TestPerformerPersonalStateResolversIsolateUsersAndDenyReducedPrincipals(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	first := createResolverUser(t, database, "performer-state-one", authz.RoleUser)
	second := createResolverUser(t, database, "performer-state-two", authz.RoleUser)
	performer := models.NewPerformer()
	performer.Name = "Resolver Personal State"
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		return database.Performer.Create(ctx, &models.CreatePerformerInput{Performer: &performer})
	}); err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{database: database, repository: database.Repository()}
	mutations := &mutationResolver{Resolver: resolver}
	fields := &performerResolver{Resolver: resolver}
	performerID := strconv.Itoa(performer.ID)
	firstCtx := authz.WithPrincipal(context.Background(), first)
	secondCtx := authz.WithPrincipal(context.Background(), second)
	firstRating, secondRating := 90, 40
	if _, err := mutations.PerformerSetFavorite(firstCtx, performerID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := mutations.PerformerSetRating(firstCtx, performerID, &firstRating); err != nil {
		t.Fatal(err)
	}
	if _, err := mutations.PerformerSetRating(secondCtx, performerID, &secondRating); err != nil {
		t.Fatal(err)
	}
	firstFavorite, err := fields.Favorite(firstCtx, &performer)
	if err != nil || !firstFavorite {
		t.Fatalf("first favorite=%v err=%v", firstFavorite, err)
	}
	secondFavorite, err := fields.Favorite(secondCtx, &performer)
	if err != nil || secondFavorite {
		t.Fatalf("second favorite=%v err=%v", secondFavorite, err)
	}
	firstPersonalRating, err := fields.Rating100(firstCtx, &performer)
	if err != nil || firstPersonalRating == nil || *firstPersonalRating != firstRating {
		t.Fatalf("first rating=%v err=%v", firstPersonalRating, err)
	}
	sum, err := fields.Rating100Sum(firstCtx, &performer)
	if err != nil || sum != 130 {
		t.Fatalf("sum=%d err=%v", sum, err)
	}
	count, err := fields.Rating100Count(secondCtx, &performer)
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	average, err := fields.Rating100Average(firstCtx, &performer)
	if err != nil || average != 65 {
		t.Fatalf("average=%v err=%v", average, err)
	}

	disabled := first
	disabled.Status = authz.StatusDisabled
	if _, err := mutations.PerformerSetFavorite(authz.WithPrincipal(context.Background(), disabled), performerID, false); err == nil {
		t.Fatal("disabled principal changed favorite")
	}
	reduced := first
	reduced.TokenScopes = map[authz.Capability]struct{}{authz.LibraryRead: {}}
	if _, err := mutations.PerformerSetRating(authz.WithPrincipal(context.Background(), reduced), performerID, nil); err == nil {
		t.Fatal("reduced token changed rating")
	}
}

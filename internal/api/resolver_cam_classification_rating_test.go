package api

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

// TestCamShowSetRatingIsRegisteredOwnerScopedAndClearable guards the GraphQL
// resolver + authorization path for personal Cam Show ratings. The store-layer
// test (pkg/sqlite) does not exercise the authz surface registry, which is where
// commit 6a729d1 regressed (camShowSetRating was never added to graphql_policy.json).
func TestCamShowSetRatingIsRegisteredOwnerScopedAndClearable(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	userA := createResolverUser(t, database, "cam-show-rater-a", authz.RoleUser)
	userB := createResolverUser(t, database, "cam-show-rater-b", authz.RoleUser)

	var showID int64
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		now := time.Now().UTC()
		scene := &models.Scene{Title: "Rating Bridge", CreatedAt: now, UpdatedAt: now}
		if err := database.Scene.Create(ctx, scene, nil); err != nil {
			return err
		}
		show, err := database.CamShow.CreateShow(ctx, int64(scene.ID), "LIVE", nil)
		if err == nil {
			showID = show.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(showID, 10)
	m := &mutationResolver{Resolver: &Resolver{database: database}}

	rating := func(v int) *int { return &v }

	if _, err := m.CamShowSetRating(context.Background(), id, rating(80)); err == nil {
		t.Fatal("unauthenticated rating was accepted")
	}

	ctxA := authz.WithPrincipal(context.Background(), userA)
	ctxB := authz.WithPrincipal(context.Background(), userB)

	if _, err := m.CamShowSetRating(ctxA, id, rating(80)); err != nil {
		t.Fatalf("user A set rating: %v", err)
	}
	if _, err := m.CamShowSetRating(ctxB, id, rating(40)); err != nil {
		t.Fatalf("user B set rating: %v", err)
	}

	got, err := m.CamShowSetRating(ctxA, id, rating(80))
	if err != nil {
		t.Fatalf("user A re-set rating: %v", err)
	}
	if got.Rating100 == nil || *got.Rating100 != 80 {
		t.Fatalf("user A personal rating = %v, want 80", got.Rating100)
	}
	if got.Rating100Count != 2 || got.Rating100Average != 60 {
		t.Fatalf("aggregate = avg %v count %d, want avg 60 count 2", got.Rating100Average, got.Rating100Count)
	}

	cleared, err := m.CamShowSetRating(ctxA, id, nil)
	if err != nil {
		t.Fatalf("clear rating: %v", err)
	}
	if cleared.Rating100 != nil {
		t.Fatalf("personal rating not cleared: %v", cleared.Rating100)
	}
	if cleared.Rating100Count != 1 || cleared.Rating100Average != 40 {
		t.Fatalf("aggregate after clear = avg %v count %d, want avg 40 count 1", cleared.Rating100Average, cleared.Rating100Count)
	}

	if _, err := m.CamShowSetRating(ctxA, id, rating(0)); err == nil {
		t.Fatal("rating 0 was accepted")
	}
	if _, err := m.CamShowSetRating(ctxA, id, rating(101)); err == nil {
		t.Fatal("rating 101 was accepted")
	}
}

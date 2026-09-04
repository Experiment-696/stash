package api

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/txn"
)

func TestCamModelUserStateResolverIsolationScopesIdempotencyAndRollback(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	first := createResolverUser(t, database, "cam-favorite-one", authz.RoleUser)
	second := createResolverUser(t, database, "cam-favorite-two", authz.RoleUser)
	var modelID int64
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		model, err := database.CamShow.CreateModel(ctx, "Favorite Model", nil)
		if model != nil {
			modelID = model.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	resolver := &Resolver{database: database}
	query := &queryResolver{Resolver: resolver}
	mutation := &mutationResolver{Resolver: resolver}
	firstCtx := authz.WithPrincipal(context.Background(), first)
	secondCtx := authz.WithPrincipal(context.Background(), second)
	id := strconv.FormatInt(modelID, 10)
	if _, err := query.CamModelProfiles(context.Background(), false); err == nil {
		t.Fatal("anonymous browsed state")
	}
	readToken := first
	readToken.TokenScopes = map[authz.Capability]struct{}{authz.LibraryRead: {}}
	if profiles, err := query.CamModelProfiles(authz.WithPrincipal(context.Background(), readToken), false); err != nil || len(profiles) != 1 {
		t.Fatalf("read token profiles=%+v err=%v", profiles, err)
	}
	if _, err := mutation.CamModelSetUserState(context.Background(), id, true, nil); err == nil {
		t.Fatal("anonymous mutated state")
	}
	if _, err := mutation.CamModelSetUserState(authz.WithPrincipal(context.Background(), readToken), id, true, nil); err == nil {
		t.Fatal("library-only token mutated state")
	}
	preferenceToken := first
	preferenceToken.TokenScopes = map[authz.Capability]struct{}{authz.PreferenceSelfWrite: {}}
	rating := 91
	for run := 0; run < 2; run++ {
		got, err := mutation.CamModelSetUserState(authz.WithPrincipal(context.Background(), preferenceToken), id, true, &rating)
		if err != nil || !got.Favorite || got.Rating100 == nil || *got.Rating100 != rating {
			t.Fatalf("run=%d got=%+v err=%v", run, got, err)
		}
	}
	firstProfiles, err := query.CamModelProfiles(firstCtx, true)
	if err != nil || len(firstProfiles) != 1 || !firstProfiles[0].Favorite {
		t.Fatalf("first=%+v err=%v", firstProfiles, err)
	}
	secondProfiles, err := query.CamModelProfiles(secondCtx, true)
	if err != nil || len(secondProfiles) != 0 {
		t.Fatalf("second=%+v err=%v", secondProfiles, err)
	}
	if _, err := mutation.CamModelSetUserState(firstCtx, "999999", true, nil); err == nil {
		t.Fatal("missing model mutation succeeded")
	}
	if err := txn.WithReadTxn(context.Background(), database, func(ctx context.Context) error {
		firstID, parseErr := strconv.ParseInt(first.UserID, 10, 64)
		if parseErr != nil {
			return parseErr
		}
		state, err := database.CamShow.GetUserState(ctx, firstID, 999999)
		if err != nil {
			return err
		}
		if state != nil {
			t.Fatal("failed mutation left state")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

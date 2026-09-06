package api

import (
	"context"
	"strconv"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

func TestSavedFilterResolversDenyCrossUserAndNullOwnership(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	first := createResolverUser(t, database, "filter-first", authz.RoleUser)
	second := createResolverUser(t, database, "filter-second", authz.RoleUser)
	firstID, _ := persistedPrincipalUserID(first)
	secondID, _ := persistedPrincipalUserID(second)
	firstFilter := models.SavedFilter{Name: "first", Mode: models.FilterModeScenes, UserID: &firstID}
	secondFilter := models.SavedFilter{Name: "second", Mode: models.FilterModeScenes, UserID: &secondID}
	unassigned := models.SavedFilter{Name: "unassigned", Mode: models.FilterModeScenes}
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		for _, filter := range []*models.SavedFilter{&firstFilter, &secondFilter, &unassigned} {
			if err := database.SavedFilter.Create(ctx, filter); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{database: database, repository: database.Repository()}
	queries := &queryResolver{Resolver: resolver}
	mutations := &mutationResolver{Resolver: resolver}
	firstCtx := authz.WithPrincipal(context.Background(), first)
	secondCtx := authz.WithPrincipal(context.Background(), second)
	filters, err := queries.FindSavedFilters(firstCtx, nil)
	if err != nil || len(filters) != 1 || filters[0].ID != firstFilter.ID {
		t.Fatalf("owner list=%+v err=%v", filters, err)
	}
	for _, foreignID := range []int{secondFilter.ID, unassigned.ID} {
		found, err := queries.FindSavedFilter(firstCtx, strconv.Itoa(foreignID))
		if err != nil || found != nil {
			t.Fatalf("foreign/null filter disclosed: %+v err=%v", found, err)
		}
		if ok, err := mutations.DestroySavedFilter(firstCtx, DestroyFilterInput{ID: strconv.Itoa(foreignID)}); err == nil || ok {
			t.Fatalf("foreign/null filter destroyed: ok=%v err=%v", ok, err)
		}
	}
	foreignID := strconv.Itoa(secondFilter.ID)
	if updated, err := mutations.SaveFilter(firstCtx, SaveFilterInput{
		ID: &foreignID, Name: "stolen", Mode: models.FilterModeScenes,
	}); err == nil || updated != nil {
		t.Fatalf("foreign filter updated: filter=%+v err=%v", updated, err)
	}
	found, err := queries.FindSavedFilter(secondCtx, foreignID)
	if err != nil || found == nil || found.Name != "second" {
		t.Fatalf("foreign update changed owner filter: filter=%+v err=%v", found, err)
	}

	sharedUI := map[string]any{"display_mode": "grid"}
	firstNamed, err := mutations.SaveFilter(firstCtx, SaveFilterInput{Name: "shared name", Mode: models.FilterModeImages, UIOptions: sharedUI})
	if err != nil {
		t.Fatal(err)
	}
	secondNamed, err := mutations.SaveFilter(secondCtx, SaveFilterInput{Name: "shared name", Mode: models.FilterModeImages, UIOptions: sharedUI})
	if err != nil || secondNamed == nil || secondNamed.ID == firstNamed.ID {
		t.Fatalf("same name across owners failed: first=%+v second=%+v err=%v", firstNamed, secondNamed, err)
	}
	if duplicate, err := mutations.SaveFilter(firstCtx, SaveFilterInput{Name: "shared name", Mode: models.FilterModeImages, UIOptions: sharedUI}); err == nil || duplicate != nil {
		t.Fatalf("same-owner duplicate accepted: filter=%+v err=%v", duplicate, err)
	}

	if ok, err := mutations.SetDefaultFilter(firstCtx, SetDefaultFilterInput{
		Mode: models.FilterModeImages, FindFilter: firstNamed.FindFilter,
		ObjectFilter: firstNamed.ObjectFilter, UIOptions: firstNamed.UIOptions,
	}); err != nil || !ok {
		t.Fatalf("owner default select failed: ok=%v err=%v", ok, err)
	}
	defaultFilter, err := queries.FindDefaultFilter(firstCtx, models.FilterModeImages)
	if err != nil || defaultFilter == nil || defaultFilter.ID != firstNamed.ID {
		t.Fatalf("owner default=%+v err=%v", defaultFilter, err)
	}
	defaultFilter, err = queries.FindDefaultFilter(secondCtx, models.FilterModeImages)
	if err != nil || defaultFilter != nil {
		t.Fatalf("first owner's selection changed second default=%+v err=%v", defaultFilter, err)
	}
	if ok, err := mutations.SetDefaultFilter(secondCtx, SetDefaultFilterInput{
		Mode: models.FilterModeImages, FindFilter: firstNamed.FindFilter,
		ObjectFilter: firstNamed.ObjectFilter, UIOptions: firstNamed.UIOptions,
	}); err != nil || !ok {
		t.Fatalf("second owner default select failed: ok=%v err=%v", ok, err)
	}
	defaultFilter, err = queries.FindDefaultFilter(secondCtx, models.FilterModeImages)
	if err != nil || defaultFilter == nil || defaultFilter.ID != secondNamed.ID {
		t.Fatalf("second owner default=%+v err=%v", defaultFilter, err)
	}
	if ok, err := mutations.SetDefaultFilter(firstCtx, SetDefaultFilterInput{Mode: models.FilterModeImages}); err != nil || !ok {
		t.Fatalf("owner default clear failed: ok=%v err=%v", ok, err)
	}
	defaultFilter, err = queries.FindDefaultFilter(firstCtx, models.FilterModeImages)
	if err != nil || defaultFilter != nil {
		t.Fatalf("cleared default=%+v err=%v", defaultFilter, err)
	}
	defaultFilter, err = queries.FindDefaultFilter(secondCtx, models.FilterModeImages)
	if err != nil || defaultFilter == nil || defaultFilter.ID != secondNamed.ID {
		t.Fatalf("first clear changed second default=%+v err=%v", defaultFilter, err)
	}
	if ok, err := mutations.DestroySavedFilter(firstCtx, DestroyFilterInput{ID: strconv.Itoa(firstFilter.ID)}); err != nil || !ok {
		t.Fatalf("owner destroy failed: ok=%v err=%v", ok, err)
	}
}

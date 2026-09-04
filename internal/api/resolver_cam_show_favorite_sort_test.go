package api

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func TestCamShowsFavoriteModelsSortDerivesPersistedPrincipalAndIsolatesUsers(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	first := createResolverUser(t, database, "show-gql-first", authz.RoleUser)
	second := createResolverUser(t, database, "show-gql-second", authz.RoleUser)

	var oldestID, middleID, newestID int64
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		favoriteA, err := database.CamShow.CreateModel(ctx, "Favorite A", nil)
		if err != nil {
			return err
		}
		favoriteB, err := database.CamShow.CreateModel(ctx, "Favorite B", nil)
		if err != nil {
			return err
		}
		secondFavorite, err := database.CamShow.CreateModel(ctx, "Second Favorite", nil)
		if err != nil {
			return err
		}
		createShow := func(title string, date time.Time) (int64, error) {
			scene := &models.Scene{Title: title, CreatedAt: date, UpdatedAt: date}
			if err := database.Scene.Create(ctx, scene, nil); err != nil {
				return 0, err
			}
			show, err := database.CamShow.CreateShow(ctx, int64(scene.ID), "LIVE", nil)
			if err != nil {
				return 0, err
			}
			if _, err := database.CamShow.UpdateShowCore(ctx, sqlite.CamShowCoreUpdateInput{ID: show.ID, Title: title, ShowType: "LIVE_PUBLIC", ShowDate: &date}); err != nil {
				return 0, err
			}
			return show.ID, nil
		}
		oldestID, err = createShow("Old favorite", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			return err
		}
		middleID, err = createShow("Middle", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			return err
		}
		newestID, err = createShow("Newest", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			return err
		}
		if err := database.CamShow.LinkModelWithRole(ctx, oldestID, favoriteA.ID, 0, "PRIMARY"); err != nil {
			return err
		}
		if err := database.CamShow.LinkModelWithRole(ctx, oldestID, favoriteB.ID, 1, "GUEST"); err != nil {
			return err
		}
		if err := database.CamShow.LinkModelWithRole(ctx, middleID, secondFavorite.ID, 0, "PRIMARY"); err != nil {
			return err
		}
		firstID, _ := strconv.ParseInt(first.UserID, 10, 64)
		secondID, _ := strconv.ParseInt(second.UserID, 10, 64)
		if err := database.CamShow.SetUserState(ctx, firstID, favoriteA.ID, true, nil); err != nil {
			return err
		}
		if err := database.CamShow.SetUserState(ctx, firstID, favoriteB.ID, true, nil); err != nil {
			return err
		}
		return database.CamShow.SetUserState(ctx, secondID, secondFavorite.ID, true, nil)
	}); err != nil {
		t.Fatal(err)
	}

	query := &queryResolver{Resolver: &Resolver{database: database}}
	assertOrder := func(label string, got []*CamShowLibraryItem, want ...int64) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s count=%d want=%d", label, len(got), len(want))
		}
		for i := range want {
			if got[i].ID != strconv.FormatInt(want[i], 10) {
				t.Fatalf("%s order[%d]=%s want=%d", label, i, got[i].ID, want[i])
			}
		}
	}
	firstCtx := authz.WithPrincipal(context.Background(), first)
	firstSorted, err := query.CamShows(firstCtx, CamShowSortModeFavoriteModelsFirst)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder("first", firstSorted, oldestID, newestID, middleID)
	secondSorted, err := query.CamShows(authz.WithPrincipal(context.Background(), second), CamShowSortModeFavoriteModelsFirst)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder("second", secondSorted, middleID, newestID, oldestID)
	defaultSorted, err := query.CamShows(firstCtx, CamShowSortModeDefault)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder("default", defaultSorted, newestID, middleID, oldestID)

	if _, err := query.CamShows(context.Background(), CamShowSortModeFavoriteModelsFirst); err == nil {
		t.Fatal("missing principal used Favorite Models sort")
	}
	nonPersisted := authz.Principal{UserID: "999999", Role: authz.RoleUser, Status: authz.StatusActive}
	if _, err := query.CamShows(authz.WithPrincipal(context.Background(), nonPersisted), CamShowSortModeFavoriteModelsFirst); err == nil {
		t.Fatal("non-persisted principal used Favorite Models sort")
	}
	reduced := first
	reduced.TokenScopes = map[authz.Capability]struct{}{authz.PreferenceSelfWrite: {}}
	if _, err := query.CamShows(authz.WithPrincipal(context.Background(), reduced), CamShowSortModeFavoriteModelsFirst); err == nil {
		t.Fatal("principal without library read used Favorite Models sort")
	}
}

func TestCamShowsFavoriteModelsGraphQLContractExposesSortButNoUserID(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	schema := read("../../graphql/schema/types/cam-show-classification.graphql")
	operation := read("../../ui/v2.5/graphql/cam-classification.graphql")
	for _, required := range []string{"enum CamShowSortMode", "FAVORITE_MODELS_FIRST", "camShows(sort: CamShowSortMode! = DEFAULT)"} {
		if !strings.Contains(schema, required) {
			t.Errorf("schema missing %q", required)
		}
	}
	for _, required := range []string{"$sort: CamShowSortMode! = DEFAULT", "camShows(sort: $sort)"} {
		if !strings.Contains(operation, required) {
			t.Errorf("operation missing %q", required)
		}
	}
	for path, source := range map[string]string{"schema": schema, "operation": operation} {
		camShows := source[strings.Index(source, "camShows"):]
		if end := strings.Index(camShows, "{"); end >= 0 {
			camShows = camShows[:end]
		}
		if strings.Contains(strings.ToLower(camShows), "user") {
			t.Fatalf("%s camShows arguments expose client user identity: %q", path, camShows)
		}
	}
}

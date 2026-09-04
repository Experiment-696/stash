package api

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

const defaultHomepageRoute = "/"

var safeHomepageRoutes = map[string]struct{}{
	"/":           {},
	"/scenes":     {},
	"/performers": {},
	"/studios":    {},
	"/galleries":  {},
	"/images":     {},
	"/groups":     {},
	"/tags":       {},
}

type MyPreferences struct {
	HomepageRoute string  `json:"homepageRoute"`
	ThemeID       *string `json:"themeID"`
}

type SelectableTheme struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func selectableThemesFromPlugins(plugins []*plugin.Plugin) []*SelectableTheme {
	ret := make([]*SelectableTheme, 0)
	for _, candidate := range plugins {
		if candidate == nil || !candidate.Enabled || candidate.ID == "" || len(candidate.UI.CSS) == 0 {
			continue
		}
		name := candidate.Name
		if name == "" {
			name = candidate.ID
		}
		ret = append(ret, &SelectableTheme{ID: candidate.ID, Name: name})
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].Name == ret[j].Name {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].Name < ret[j].Name
	})
	return ret
}

func (r *Resolver) selectableThemes() []*SelectableTheme {
	if r.themeCatalog != nil {
		return selectableThemesFromPlugins(r.themeCatalog())
	}
	return selectableThemesFromPlugins(manager.ListPluginsIfInitialized())
}

func themeIsSelectable(themes []*SelectableTheme, id string) bool {
	for _, theme := range themes {
		if theme.ID == id {
			return true
		}
	}
	return false
}

func validateHomepageRoute(route string) error {
	if _, ok := safeHomepageRoutes[route]; !ok {
		return errors.New("homepage route is not allowed")
	}
	return nil
}

func preferencePrincipal(ctx context.Context, capability authz.Capability) (authz.Principal, int64, error) {
	principal, err := authz.RequireContext(ctx, capability)
	if err != nil {
		return authz.Principal{}, 0, err
	}
	userID, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || userID <= 0 {
		return authz.Principal{}, 0, authz.UnauthenticatedError{}
	}
	return principal, userID, nil
}

func (r *queryResolver) MyPreferences(ctx context.Context) (*MyPreferences, error) {
	principal, userID, err := preferencePrincipal(ctx, authz.AccountSelfRead)
	if err != nil {
		return nil, err
	}
	result := &MyPreferences{HomepageRoute: defaultHomepageRoute}
	err = txn.WithReadTxn(ctx, r.tokenDatabase(), func(txCtx context.Context) error {
		value, readErr := r.tokenDatabase().Preference.Get(txCtx, userID, sqlite.PreferenceHomepageRoute)
		if readErr != nil {
			return readErr
		}
		if value != nil && validateHomepageRoute(*value) == nil &&
			(*value == defaultHomepageRoute || authz.Require(principal, authz.LibraryRead) == nil) {
			result.HomepageRoute = *value
		}
		themeID, readErr := r.tokenDatabase().Preference.Get(txCtx, userID, sqlite.PreferenceThemeID)
		if readErr != nil {
			return readErr
		}
		if themeID != nil && themeIsSelectable(r.selectableThemes(), *themeID) {
			result.ThemeID = themeID
		}
		return nil
	})
	return result, err
}

func (r *queryResolver) AvailableThemes(ctx context.Context) ([]*SelectableTheme, error) {
	if _, _, err := preferencePrincipal(ctx, authz.AccountSelfRead); err != nil {
		return nil, err
	}
	return r.selectableThemes(), nil
}

func (r *mutationResolver) SetMyHomepageRoute(ctx context.Context, route string) (*MyPreferences, error) {
	principal, userID, err := preferencePrincipal(ctx, authz.PreferenceSelfWrite)
	if err != nil {
		return nil, err
	}
	if err := validateHomepageRoute(route); err != nil {
		return nil, err
	}
	if route != defaultHomepageRoute {
		if err := authz.Require(principal, authz.LibraryRead); err != nil {
			return nil, err
		}
	}
	if err := txn.WithTxn(ctx, r.tokenDatabase(), func(txCtx context.Context) error {
		if route == defaultHomepageRoute {
			return r.tokenDatabase().Preference.Clear(txCtx, userID, sqlite.PreferenceHomepageRoute)
		}
		return r.tokenDatabase().Preference.Set(txCtx, userID, sqlite.PreferenceHomepageRoute, route)
	}); err != nil {
		return nil, err
	}
	prefs, err := (&queryResolver{Resolver: r.Resolver}).MyPreferences(ctx)
	return prefs, err
}

func (r *mutationResolver) SetMyTheme(ctx context.Context, themeID *string) (*MyPreferences, error) {
	_, userID, err := preferencePrincipal(ctx, authz.PreferenceSelfWrite)
	if err != nil {
		return nil, err
	}
	if themeID != nil && !themeIsSelectable(r.selectableThemes(), *themeID) {
		return nil, errors.New("theme is not installed and enabled")
	}
	if err := txn.WithTxn(ctx, r.tokenDatabase(), func(txCtx context.Context) error {
		if themeID == nil {
			return r.tokenDatabase().Preference.Clear(txCtx, userID, sqlite.PreferenceThemeID)
		}
		return r.tokenDatabase().Preference.Set(txCtx, userID, sqlite.PreferenceThemeID, *themeID)
	}); err != nil {
		return nil, err
	}
	prefs, err := (&queryResolver{Resolver: r.Resolver}).MyPreferences(ctx)
	return prefs, err
}

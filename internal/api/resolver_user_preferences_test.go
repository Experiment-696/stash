package api

import (
	"context"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func TestHomepagePreferenceSelfIsolationValidationAndFallback(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	first := createResolverUser(t, database, "homepage-first", authz.RoleUser)
	second := createResolverUser(t, database, "homepage-second", authz.RoleModerator)
	resolver := &Resolver{database: database}
	query := &queryResolver{Resolver: resolver}
	mutation := &mutationResolver{Resolver: resolver}
	firstCtx := authz.WithPrincipal(context.Background(), first)
	secondCtx := authz.WithPrincipal(context.Background(), second)

	for _, route := range []string{"https://example.com", "//example.com", "/settings", "/setup", "/scenes?sort=date", "/unknown", "scenes"} {
		if got, err := mutation.SetMyHomepageRoute(firstCtx, route); err == nil || got != nil {
			t.Fatalf("unsafe route %q accepted: result=%+v err=%v", route, got, err)
		}
	}
	firstPrefs, err := mutation.SetMyHomepageRoute(firstCtx, "/scenes")
	if err != nil || firstPrefs.HomepageRoute != "/scenes" {
		t.Fatalf("first write=%+v err=%v", firstPrefs, err)
	}
	secondPrefs, err := mutation.SetMyHomepageRoute(secondCtx, "/performers")
	if err != nil || secondPrefs.HomepageRoute != "/performers" {
		t.Fatalf("second write=%+v err=%v", secondPrefs, err)
	}
	firstPrefs, err = query.MyPreferences(firstCtx)
	if err != nil || firstPrefs.HomepageRoute != "/scenes" {
		t.Fatalf("first read=%+v err=%v", firstPrefs, err)
	}
	secondPrefs, err = query.MyPreferences(secondCtx)
	if err != nil || secondPrefs.HomepageRoute != "/performers" {
		t.Fatalf("second read=%+v err=%v", secondPrefs, err)
	}
	restricted := first
	restricted.TokenScopes = map[authz.Capability]struct{}{
		authz.AccountSelfRead:     {},
		authz.PreferenceSelfWrite: {},
	}
	restrictedCtx := authz.WithPrincipal(context.Background(), restricted)
	restrictedPrefs, err := query.MyPreferences(restrictedCtx)
	if err != nil || restrictedPrefs.HomepageRoute != defaultHomepageRoute {
		t.Fatalf("newly unavailable route did not fall back: prefs=%+v err=%v", restrictedPrefs, err)
	}
	if got, err := mutation.SetMyHomepageRoute(restrictedCtx, "/images"); err == nil || got != nil {
		t.Fatalf("unavailable route accepted: prefs=%+v err=%v", got, err)
	}

	firstID, _ := persistedPrincipalUserID(first)
	if err := txn.WithTxn(context.Background(), database, func(txCtx context.Context) error {
		return database.Preference.Set(txCtx, firstID, sqlite.PreferenceHomepageRoute, "/settings")
	}); err != nil {
		t.Fatal(err)
	}
	firstPrefs, err = query.MyPreferences(firstCtx)
	if err != nil || firstPrefs.HomepageRoute != defaultHomepageRoute {
		t.Fatalf("invalid stored route did not fall back: prefs=%+v err=%v", firstPrefs, err)
	}
	if got, err := query.MyPreferences(context.Background()); err == nil || got != nil {
		t.Fatalf("anonymous preferences disclosed: prefs=%+v err=%v", got, err)
	}
	if got, err := mutation.SetMyHomepageRoute(context.Background(), "/scenes"); err == nil || got != nil {
		t.Fatalf("anonymous preference write accepted: prefs=%+v err=%v", got, err)
	}
}

func TestThemePreferenceCatalogValidationIsolationAndFallback(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	first := createResolverUser(t, database, "theme-first", authz.RoleUser)
	second := createResolverUser(t, database, "theme-second", authz.RoleModerator)
	dark := &plugin.Plugin{ID: "dark-theme", Name: "Dark Theme", Enabled: true, UI: plugin.PluginUI{CSS: []string{"theme.css"}}}
	light := &plugin.Plugin{ID: "light-theme", Name: "Light Theme", Enabled: true, UI: plugin.PluginUI{CSS: []string{"theme.css"}}}
	disabled := &plugin.Plugin{ID: "disabled-theme", Name: "Disabled", Enabled: false, UI: plugin.PluginUI{CSS: []string{"theme.css"}}}
	javascriptOnly := &plugin.Plugin{ID: "not-a-theme", Name: "Script", Enabled: true, UI: plugin.PluginUI{Javascript: []string{"plugin.js"}}}
	catalog := []*plugin.Plugin{light, disabled, javascriptOnly, dark}
	resolver := &Resolver{database: database, themeCatalog: func() []*plugin.Plugin { return catalog }}
	query := &queryResolver{Resolver: resolver}
	mutation := &mutationResolver{Resolver: resolver}
	firstCtx := authz.WithPrincipal(context.Background(), first)
	secondCtx := authz.WithPrincipal(context.Background(), second)

	themes, err := query.AvailableThemes(firstCtx)
	if err != nil || len(themes) != 2 || themes[0].ID != dark.ID || themes[1].ID != light.ID {
		t.Fatalf("safe catalog=%+v err=%v", themes, err)
	}
	for _, invalid := range []string{"missing", disabled.ID, javascriptOnly.ID, "../theme"} {
		if got, setErr := mutation.SetMyTheme(firstCtx, &invalid); setErr == nil || got != nil {
			t.Fatalf("invalid theme %q accepted: prefs=%+v err=%v", invalid, got, setErr)
		}
	}
	firstPrefs, err := mutation.SetMyTheme(firstCtx, &dark.ID)
	if err != nil || firstPrefs.ThemeID == nil || *firstPrefs.ThemeID != dark.ID {
		t.Fatalf("first theme=%+v err=%v", firstPrefs, err)
	}
	secondPrefs, err := mutation.SetMyTheme(secondCtx, &light.ID)
	if err != nil || secondPrefs.ThemeID == nil || *secondPrefs.ThemeID != light.ID {
		t.Fatalf("second theme=%+v err=%v", secondPrefs, err)
	}
	firstPrefs, err = query.MyPreferences(firstCtx)
	if err != nil || firstPrefs.ThemeID == nil || *firstPrefs.ThemeID != dark.ID {
		t.Fatalf("first isolated read=%+v err=%v", firstPrefs, err)
	}

	// Removing/disabling the selected plugin retains the stored preference but
	// resolves to the global theme until Admin enables it again.
	dark.Enabled = false
	firstPrefs, err = query.MyPreferences(firstCtx)
	if err != nil || firstPrefs.ThemeID != nil {
		t.Fatalf("disabled theme did not fall back: prefs=%+v err=%v", firstPrefs, err)
	}
	dark.Enabled = true
	firstPrefs, err = mutation.SetMyTheme(firstCtx, nil)
	if err != nil || firstPrefs.ThemeID != nil {
		t.Fatalf("global theme reset=%+v err=%v", firstPrefs, err)
	}
	secondPrefs, err = query.MyPreferences(secondCtx)
	if err != nil || secondPrefs.ThemeID == nil || *secondPrefs.ThemeID != light.ID {
		t.Fatalf("first reset affected second: prefs=%+v err=%v", secondPrefs, err)
	}
	if got, queryErr := query.AvailableThemes(context.Background()); queryErr == nil || got != nil {
		t.Fatalf("anonymous catalog disclosed: themes=%+v err=%v", got, queryErr)
	}
	if got, setErr := mutation.SetMyTheme(context.Background(), &dark.ID); setErr == nil || got != nil {
		t.Fatalf("anonymous theme write accepted: prefs=%+v err=%v", got, setErr)
	}
}

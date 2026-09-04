package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/sqlite"
)

func TestAuthorizeGraphQLRoot(t *testing.T) {
	registry, err := authz.NewRegistry([]authz.Surface{
		{Kind: authz.SurfaceGraphQLQuery, Name: "findScenes", Capability: authz.LibraryRead},
		{Kind: authz.SurfaceGraphQLMutation, Name: "sceneUnwiredActivity", Capability: authz.ActivitySelfWrite, OwnerScoped: true},
		{Kind: authz.SurfaceGraphQLSubscription, Name: "jobsSubscribe", Capability: authz.JobRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	user := authz.WithPrincipal(context.Background(), authz.Principal{UserID: "u1", Role: authz.RoleUser, Status: authz.StatusActive})
	admin := authz.WithPrincipal(context.Background(), authz.Principal{UserID: "a1", Role: authz.RoleAdmin, Status: authz.StatusActive})
	for _, test := range []struct {
		name, object, field string
		ctx                 context.Context
		wantErr             bool
	}{
		{"query", "Query", "findScenes", user, false},
		{"owner mutation unresolved", "Mutation", "sceneUnwiredActivity", user, true},
		{"subscription", "Subscription", "jobsSubscribe", admin, false},
		{"missing principal", "Query", "findScenes", context.Background(), true},
		{"missing policy", "Query", "futureField", admin, true},
		{"wrong root kind", "Mutation", "findScenes", admin, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := authorizeGraphQLRoot(test.ctx, registry, test.object, test.field); (err != nil) != test.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
	err = authorizeGraphQLRoot(admin, registry, "Query", "futureField")
	var missing authz.UnregisteredSurfaceError
	if !errors.As(err, &missing) {
		t.Fatalf("missing policy error=%T want UnregisteredSurfaceError", err)
	}
	err = authorizeGraphQLRoot(user, registry, "Mutation", "sceneUnwiredActivity")
	var unresolved authz.OwnerResolutionRequiredError
	if !errors.As(err, &unresolved) {
		t.Fatalf("owner-scoped error=%T want OwnerResolutionRequiredError", err)
	}
}

func TestBootstrapWindowUsesSocketPeerNotForwardingHeaders(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	requestContext := func(remote string, headers map[string]string) context.Context {
		r := httptest.NewRequest("POST", "/graphql", nil)
		r.RemoteAddr = remote
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		return session.SetLocalRequest(r).Context()
	}
	if !bootstrapWindowOpen(requestContext("127.0.0.1:1234", nil), database, "Query", "bootstrapConfiguration") {
		t.Fatal("IPv4 loopback denied")
	}
	if !bootstrapWindowOpen(requestContext("[::1]:1234", nil), database, "Query", "bootstrapConfiguration") {
		t.Fatal("IPv6 loopback denied")
	}
	spoofed := requestContext("203.0.113.9:1234", map[string]string{
		"Forwarded": "for=127.0.0.1", "X-Forwarded-For": "127.0.0.1", "X-Real-IP": "127.0.0.1",
	})
	if bootstrapWindowOpen(spoofed, database, "Query", "bootstrapConfiguration") {
		t.Fatal("remote peer opened bootstrap window with forwarding headers")
	}
	createResolverUser(t, database, "window-closed-admin", authz.RoleAdmin)
	if bootstrapWindowOpen(requestContext("127.0.0.1:1234", nil), database, "Query", "bootstrapConfiguration") {
		t.Fatal("loopback bootstrap remained open after first user")
	}
}

func TestBootstrapConfigurationExposesOnlyNonSensitiveFields(t *testing.T) {
	typeOf := reflect.TypeOf(BootstrapConfiguration{})
	got := map[string]bool{}
	for i := 0; i < typeOf.NumField(); i++ {
		got[typeOf.Field(i).Name] = true
	}
	if len(got) != 2 || !got["Status"] || !got["Os"] {
		t.Fatalf("bootstrap configuration fields=%v; want only status and os", got)
	}
	for _, forbidden := range []string{"Username", "Password", "PasswordHash", "ApiKey", "Token", "DatabasePath", "Stashes", "Plugins", "Scraping", "Proxy", "HomeDir", "WorkingDir", "ConfigPath"} {
		if got[forbidden] {
			t.Fatalf("sensitive bootstrap field exposed: %s", forbidden)
		}
	}
}

func TestGraphQLRootObjectClassification(t *testing.T) {
	for _, object := range []string{"Query", "Mutation", "Subscription"} {
		if !isGraphQLRootObject(object) {
			t.Fatalf("%s not classified as root", object)
		}
	}
	if isGraphQLRootObject("Scene") {
		t.Fatal("nested object classified as root")
	}
}

func TestGraphQLPolicyRepresentativePrincipals(t *testing.T) {
	registry, err := authz.LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	principal := func(role authz.Role, status authz.AccountStatus, scopes ...authz.Capability) context.Context {
		p := authz.Principal{UserID: "42", Role: role, Status: status}
		if scopes != nil {
			p.TokenScopes = map[authz.Capability]struct{}{}
			for _, scope := range scopes {
				p.TokenScopes[scope] = struct{}{}
			}
		}
		return authz.WithPrincipal(context.Background(), p)
	}
	user := principal(authz.RoleUser, authz.StatusActive)
	moderator := principal(authz.RoleModerator, authz.StatusActive)
	admin := principal(authz.RoleAdmin, authz.StatusActive)
	disabled := principal(authz.RoleAdmin, authz.StatusDisabled)
	passwordChange := principal(authz.RoleAdmin, authz.StatusPasswordChangeRequired)
	readToken := principal(authz.RoleAdmin, authz.StatusActive, authz.LibraryRead)

	tests := []struct {
		name, object, field string
		ctx                 context.Context
		allowed             bool
	}{
		{"user read", "Query", "findScenes", user, true},
		{"user metadata denied", "Mutation", "sceneUpdate", user, false},
		{"moderator metadata", "Mutation", "sceneUpdate", moderator, true},
		{"moderator destructive denied", "Mutation", "sceneDestroy", moderator, false},
		{"admin destructive", "Mutation", "sceneDestroy", admin, true},
		{"admin system", "Mutation", "configureGeneral", admin, true},
		{"admin database", "Mutation", "querySQL", admin, true},
		{"admin account manage", "Query", "users", admin, true},
		{"admin audit read", "Query", "auditEvents", admin, true},
		{"user audit read denied", "Query", "auditEvents", user, false},
		{"user account manage denied", "Query", "users", user, false},
		{"disabled denied", "Query", "findScenes", disabled, false},
		{"password change denied", "Query", "findScenes", passwordChange, false},
		{"reduced token read", "Query", "findScenes", readToken, true},
		{"user app shell", "Query", "appShellConfiguration", user, true},
		{"moderator app shell", "Query", "appShellConfiguration", moderator, true},
		{"admin app shell", "Query", "appShellConfiguration", admin, true},
		{"disabled app shell denied", "Query", "appShellConfiguration", disabled, false},
		{"reduced token app shell", "Query", "appShellConfiguration", readToken, true},
		{"user role-safe configuration", "Query", "configuration", user, true},
		{"moderator role-safe configuration", "Query", "configuration", moderator, true},
		{"admin full configuration", "Query", "configuration", admin, true},
		{"reduced token system denied", "Mutation", "configureGeneral", readToken, false},
		{"intrinsic owner self", "Query", "myAPITokens", user, true},
		{"homepage preference self read", "Query", "myPreferences", user, true},
		{"homepage preference self write", "Mutation", "setMyHomepageRoute", user, true},
		{"disabled homepage preference denied", "Mutation", "setMyHomepageRoute", disabled, false},
		{"reduced token homepage preference denied", "Mutation", "setMyHomepageRoute", readToken, false},
		{"resolver-owned saved filter now enforced downstream", "Mutation", "saveFilter", user, true},
		{"resolver-owned default filter read", "Query", "findDefaultFilter", user, true},
		{"resolver-owned default filter write", "Mutation", "setDefaultFilter", user, true},
		{"disabled default filter denied", "Mutation", "setDefaultFilter", disabled, false},
		{"reduced token default filter denied", "Mutation", "setDefaultFilter", readToken, false},
		{"personal favorite write", "Mutation", "performerSetFavorite", user, true},
		{"personal rating write", "Mutation", "performerSetRating", user, true},
		{"personal scene rating write", "Mutation", "sceneSetRating", user, true},
		{"disabled personal rating denied", "Mutation", "performerSetRating", disabled, false},
		{"reduced token personal favorite denied", "Mutation", "performerSetFavorite", readToken, false},
		{"resolver-owned activity", "Mutation", "sceneAddO", user, true},
		{"disabled activity denied", "Mutation", "sceneAddO", disabled, false},
		{"reduced token activity denied", "Mutation", "sceneAddO", readToken, false},
		{"public bootstrap requires window", "Mutation", "bootstrapFirstAdmin", context.Background(), false},
		{"unknown fails closed", "Query", "futureSchemaField", admin, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := authorizeGraphQLRoot(test.ctx, registry, test.object, test.field)
			if (err == nil) != test.allowed {
				t.Fatalf("allowed=%v err=%v", test.allowed, err)
			}
		})
	}
}

func TestCamGraphQLCapabilityMatrixFailsClosed(t *testing.T) {
	registry, err := authz.LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	principal := func(role authz.Role, status authz.AccountStatus, scopes ...authz.Capability) context.Context {
		value := authz.Principal{UserID: "42", Role: role, Status: status}
		if scopes != nil {
			value.TokenScopes = map[authz.Capability]struct{}{}
			for _, scope := range scopes {
				value.TokenScopes[scope] = struct{}{}
			}
		}
		return authz.WithPrincipal(context.Background(), value)
	}
	admin := principal(authz.RoleAdmin, authz.StatusActive)
	denied := map[string]context.Context{
		"missing":        context.Background(),
		"user":           principal(authz.RoleUser, authz.StatusActive),
		"moderator":      principal(authz.RoleModerator, authz.StatusActive),
		"disabled-admin": principal(authz.RoleAdmin, authz.StatusDisabled),
		"reduced-admin":  principal(authz.RoleAdmin, authz.StatusActive, authz.LibraryRead),
	}
	adminMutations := []string{
		"camShowUpdate", "camClassificationApply", "camClassificationRuleCreate",
		"camClassificationRuleSetEnabled", "camClassificationRuleUpdate",
		"camGirlFinderConfigure", "camGirlFinderSearch", "camGirlFinderIngestPending",
		"completedRecordingImportConfigure", "completedRecordingPreview", "completedRecordingApply",
		"camModelProfileCreate", "camModelProfileUpdate", "camModelAccountAdd",
		"camModelAccountRetire", "camModelEvidenceCreate", "camModelEvidenceReview",
		"camModelSocialProfileCreate", "camModelSocialProfileRetire",
	}
	for _, field := range adminMutations {
		if err := authorizeGraphQLRoot(admin, registry, "Mutation", field); err != nil {
			t.Fatalf("Admin denied Mutation.%s: %v", field, err)
		}
		for name, ctx := range denied {
			if err := authorizeGraphQLRoot(ctx, registry, "Mutation", field); err == nil {
				t.Fatalf("%s allowed Mutation.%s", name, field)
			}
		}
	}
	adminQueries := []string{
		"camClassificationRules", "camClassificationPreview", "camGirlFinderConfig",
		"completedRecordingImportConfig",
	}
	for _, field := range adminQueries {
		if err := authorizeGraphQLRoot(admin, registry, "Query", field); err != nil {
			t.Fatalf("Admin denied Query.%s: %v", field, err)
		}
		for name, ctx := range denied {
			if err := authorizeGraphQLRoot(ctx, registry, "Query", field); err == nil {
				t.Fatalf("%s allowed Query.%s", name, field)
			}
		}
	}
	for _, field := range []string{"camShows", "camModelProfiles", "camModelProfile", "camModelSites"} {
		for name, ctx := range map[string]context.Context{
			"user": principal(authz.RoleUser, authz.StatusActive), "moderator": principal(authz.RoleModerator, authz.StatusActive), "admin": admin,
		} {
			if err := authorizeGraphQLRoot(ctx, registry, "Query", field); err != nil {
				t.Fatalf("%s denied library Query.%s: %v", name, field, err)
			}
		}
	}
	for name, ctx := range map[string]context.Context{
		"user": principal(authz.RoleUser, authz.StatusActive), "moderator": principal(authz.RoleModerator, authz.StatusActive), "admin": admin,
	} {
		if err := authorizeGraphQLRoot(ctx, registry, "Mutation", "camModelSetUserState"); err != nil {
			t.Fatalf("%s denied own Cam Model state: %v", name, err)
		}
	}
}

func TestGraphQLBootstrapWindow(t *testing.T) {
	registry, err := authz.LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	database := tokenResolverTestDatabase(t)
	local := localBootstrapContext()
	for _, root := range []struct{ object, name string }{
		{"Query", "bootstrapConfiguration"}, {"Mutation", "setup"},
		{"Mutation", "bootstrapConfigureUI"}, {"Mutation", "bootstrapFirstAdmin"},
	} {
		if err := authorizeGraphQLRootWithBootstrap(local, registry, database, root.object, root.name); err != nil {
			t.Fatalf("local zero-user %s.%s denied: %v", root.object, root.name, err)
		}
	}
	if err := authorizeGraphQLRootWithBootstrap(context.Background(), registry, database, "Query", "bootstrapConfiguration"); err == nil {
		t.Fatal("remote zero-user configuration allowed")
	}
	if err := authorizeGraphQLRootWithBootstrap(local, registry, database, "Mutation", "configureUISetting"); err == nil {
		t.Fatal("arbitrary UI configuration allowed during bootstrap")
	}
	admin := createResolverUser(t, database, "bootstrap-window-admin", authz.RoleAdmin)
	if err := authorizeGraphQLRootWithBootstrap(local, registry, database, "Query", "bootstrapConfiguration"); err == nil {
		t.Fatal("bootstrap query replay allowed after first user")
	}
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	if err := authorizeGraphQLRootWithBootstrap(adminCtx, registry, database, "Query", "configuration"); err != nil {
		t.Fatalf("normal Admin configuration denied: %v", err)
	}
	if err := authorizeGraphQLRootWithBootstrap(adminCtx, registry, database, "Mutation", "configureUISetting"); err != nil {
		t.Fatalf("normal Admin UI configuration denied: %v", err)
	}
}

func TestLegacyMigrationPrincipalHasOnlyMigrationRoots(t *testing.T) {
	registry, err := authz.LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	p := authz.Principal{
		UserID: "migration:legacy", Role: authz.RoleAdmin, Status: authz.StatusActive,
		TokenScopes: map[authz.Capability]struct{}{
			authz.LibraryRead: {}, authz.SystemStatusRead: {}, authz.SystemConfigure: {},
		},
	}
	ctx := context.WithValue(context.Background(), legacyMigrationPrincipalContextKey{}, true)
	for _, allowed := range []struct{ object, field string }{} {
		if err := authorizeGraphQLRootWithBootstrap(ctx, registry, nil, allowed.object, allowed.field); err != nil {
			t.Fatalf("migration root %s.%s denied: %v", allowed.object, allowed.field, err)
		}
	}
	for _, denied := range []struct{ object, field string }{
		{"Query", "appShellConfiguration"}, {"Query", "systemStatus"}, {"Mutation", "migrate"},
		{"Query", "findScenes"}, {"Query", "me"}, {"Query", "users"}, {"Query", "querySQL"},
		{"Query", "configuration"}, {"Mutation", "configureGeneral"}, {"Mutation", "sceneDestroy"},
		{"Mutation", "bootstrapFirstAdmin"},
	} {
		if err := authorizeGraphQLRootWithBootstrap(ctx, registry, nil, denied.object, denied.field); err == nil {
			t.Fatalf("migration principal allowed %s.%s", denied.object, denied.field)
		}
	}
	if p.Owns("1") {
		t.Fatal("migration sentinel matched a persisted resource owner")
	}
}

func TestLocalNoCredentialMigrationRequestIsNarrowAndDisappears(t *testing.T) {
	cfg := config.InitializeEmpty()
	path := filepath.Join(t.TempDir(), "legacy-v0.sqlite")
	seed := sqlite.NewDatabase()
	if err := seed.Open(path); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite3ex", path)
	if err != nil {
		t.Fatal(err)
	}
	targetVersion := seed.AppSchemaVersion() - 1
	if _, err := raw.Exec("UPDATE schema_migrations SET version = ?, dirty = 0", targetVersion); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	database := sqlite.NewDatabase()
	var migrationNeeded *sqlite.MigrationNeededError
	if err := database.Open(path); !errors.As(err, &migrationNeeded) {
		t.Fatalf("legacy database did not enter migration-needed state: %v", err)
	}
	if migrationNeeded.CurrentSchemaVersion != targetVersion {
		t.Fatalf("migration tracker version=%d want=%d", migrationNeeded.CurrentSchemaVersion, targetVersion)
	}

	request := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request = session.SetLocalRequest(request)
	if localNoCredentialMigrationRequest(request, cfg, database) {
		t.Fatal("trusted-local no-credential request was elevated to migration authority")
	}

	for name, mutate := range map[string]func(*http.Request){
		"remote":         func(r *http.Request) { r.RemoteAddr = "203.0.113.5:12345" },
		"authorization":  func(r *http.Request) { r.Header.Set("Authorization", "Bearer forged") },
		"api key header": func(r *http.Request) { r.Header.Set(session.ApiKeyHeader, "forged") },
		"api key query": func(r *http.Request) {
			q := r.URL.Query()
			q.Set(session.ApiKeyParameter, "forged")
			r.URL.RawQuery = q.Encode()
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request.Clone(context.Background())
			mutate(candidate)
			candidate = session.SetLocalRequest(candidate)
			if localNoCredentialMigrationRequest(candidate, cfg, database) {
				t.Fatal("untrusted migration request accepted")
			}
		})
	}

	currentPath := filepath.Join(t.TempDir(), "current.sqlite")
	current := sqlite.NewDatabase()
	if err := current.Open(currentPath); err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	if localNoCredentialMigrationRequest(request, cfg, current) {
		t.Fatal("migration principal remained available after database opened")
	}
}

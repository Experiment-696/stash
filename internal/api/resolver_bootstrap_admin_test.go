package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stashapp/stash/internal/authservice"
	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func localBootstrapContext() context.Context {
	r := httptest.NewRequest("POST", "/graphql", nil)
	r.RemoteAddr = "127.0.0.1:43210"
	return session.SetLocalRequest(r).Context()
}

func TestBootstrapFirstAdminLocalOnlyReplayAndConcurrency(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	resolver := &mutationResolver{Resolver: &Resolver{database: database}}
	input := FirstAdminBootstrapInput{Username: "first-admin", Password: "bootstrap-password"}

	if _, err := resolver.BootstrapFirstAdmin(context.Background(), input); err == nil {
		t.Fatal("remote/unmarked request bootstrapped first Admin")
	}

	const attempts = 8
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := resolver.BootstrapFirstAdmin(localBootstrapContext(), input); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("bootstrap successes=%d want=1", successes.Load())
	}
	if _, err := resolver.BootstrapFirstAdmin(localBootstrapContext(), input); err == nil {
		t.Fatal("bootstrap replay succeeded")
	}
}

func TestNoCredentialMigrationDestinationRequiresFirstAdminThenRemoteLogin(t *testing.T) {
	// A successfully migrated legacy database is open but contains no users.
	// This is the exact destination state of the no-credential migration path.
	database := tokenResolverTestDatabase(t)
	resolver := &mutationResolver{Resolver: &Resolver{database: database}}
	registry, err := authz.LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	local := localBootstrapContext()
	if err := authorizeGraphQLRootWithBootstrap(local, registry, database, "Query", "bootstrapConfiguration"); err != nil {
		t.Fatalf("zero-user post-migration bootstrap window unavailable: %v", err)
	}
	if err := authorizeGraphQLRootWithBootstrap(context.Background(), registry, database, "Mutation", "bootstrapFirstAdmin"); err == nil {
		t.Fatal("remote request could create the first Admin")
	}

	const username = "migrated-first-admin"
	const password = "remote-login-password"
	created, err := resolver.BootstrapFirstAdmin(local, FirstAdminBootstrapInput{
		Username: username,
		Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Role != string(authz.RoleAdmin) || created.Status != string(authz.StatusActive) {
		t.Fatalf("first account is not an active Admin: %#v", created)
	}
	if err := authorizeGraphQLRootWithBootstrap(local, registry, database, "Query", "bootstrapConfiguration"); err == nil {
		t.Fatal("bootstrap window remained open after first Admin creation")
	}

	// Login is intentionally not local-only: once the local bootstrap creates a
	// persisted Admin, a remote browser must be able to authenticate normally.
	principal, _, err := (authservice.LoginService{Database: database}).Login(
		context.Background(), username, password,
	)
	if err != nil {
		t.Fatalf("persisted Admin could not log in remotely: %v", err)
	}
	if principal.Role != authz.RoleAdmin || principal.Status != authz.StatusActive {
		t.Fatalf("remote login resolved wrong principal: %#v", principal)
	}

	adminID, err := parseUserID(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		return database.User.SetAccess(ctx, adminID, authz.RoleUser, authz.StatusActive)
	})
	if !errors.Is(err, sqlite.ErrLastActiveAdmin) {
		t.Fatalf("last Admin invariant not enforced after migration/bootstrap: %v", err)
	}
}

func TestNoCredentialMigrationUIRoutesIntoFirstAdminBootstrap(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	read := func(rel string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	migrate := read("ui/v2.5/src/components/Setup/Migrate.tsx")
	if !strings.Contains(migrate, "window.location.assign(`${baseURL}login`)") {
		t.Fatal("migration completion does not enter the post-migration login/bootstrap dispatcher")
	}
	setup := read("ui/v2.5/src/App.tsx")
	for _, required := range []string{
		`status === GQL.SystemStatusEnum.Setup`,
		`history.push("/setup")`,
	} {
		if !strings.Contains(setup, required) {
			t.Fatalf("zero-user post-migration setup redirect missing %q", required)
		}
	}
	wizard := read("ui/v2.5/src/components/Setup/Setup.tsx")
	for _, required := range []string{
		`setStep(2)`,
		`await mutateBootstrapFirstAdmin`,
		`window.location.assign("/login")`,
	} {
		if !strings.Contains(wizard, required) {
			t.Fatalf("first-Admin bootstrap flow missing %q", required)
		}
	}
}

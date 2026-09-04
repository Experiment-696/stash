package api

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func tokenResolverTestDatabase(t *testing.T) *sqlite.Database {
	t.Helper()
	config.InitializeEmpty()
	database := sqlite.NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "resolver.sqlite")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func createResolverUser(t *testing.T, database *sqlite.Database, name string, role authz.Role) authz.Principal {
	t.Helper()
	var principal authz.Principal
	err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		user, createErr := database.User.Create(ctx, name, "test-password", role)
		if createErr == nil {
			principal = authz.Principal{UserID: strconv.FormatInt(user.ID, 10), Role: user.Role, Status: user.Status}
		}
		return createErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

func TestAPITokenResolversOwnerIsolationAndOneTimeSecret(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	owner := createResolverUser(t, database, "resolver-owner", authz.RoleAdmin)
	other := createResolverUser(t, database, "resolver-other", authz.RoleUser)
	resolver := &Resolver{database: database}
	mutations := &mutationResolver{Resolver: resolver}
	queries := &queryResolver{Resolver: resolver}

	if _, err := queries.MyAPITokens(context.Background()); err == nil {
		t.Fatal("missing principal listed tokens")
	}
	ownerCtx := authz.WithPrincipal(context.Background(), owner)
	created, err := mutations.CreateMyAPIToken(ownerCtx, APITokenCreateInput{Name: "reader", Scopes: []string{string(authz.LibraryRead)}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" || !strings.HasPrefix(created.Secret, created.Token.ID+".") {
		t.Fatalf("one-time bearer credential has unexpected format")
	}
	listed, err := queries.MyAPITokens(ownerCtx)
	if err != nil || len(listed) != 1 || listed[0].ID != created.Token.ID {
		t.Fatalf("owner list=%+v err=%v", listed, err)
	}
	if strings.Contains(listed[0].ID+listed[0].Name, created.Secret) {
		t.Fatal("one-time secret was returned by metadata listing")
	}

	otherCtx := authz.WithPrincipal(context.Background(), other)
	otherList, err := queries.MyAPITokens(otherCtx)
	if err != nil || len(otherList) != 0 {
		t.Fatalf("foreign principal saw owner token: %+v err=%v", otherList, err)
	}
	if _, err := mutations.RevokeMyAPIToken(otherCtx, created.Token.ID); err == nil || err.Error() != "unable to revoke API token" {
		t.Fatalf("foreign revoke did not fail generically: %v", err)
	}
	if ok, err := mutations.RevokeMyAPIToken(ownerCtx, created.Token.ID); err != nil || !ok {
		t.Fatalf("owner revoke failed: ok=%v err=%v", ok, err)
	}
	if ok, err := mutations.RevokeMyAPIToken(ownerCtx, created.Token.ID); err != nil || !ok {
		t.Fatalf("owner revoke replay was not idempotent: ok=%v err=%v", ok, err)
	}
}

func TestAPITokenResolverRejectsScopeEscalationGenerically(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	user := createResolverUser(t, database, "resolver-user", authz.RoleUser)
	resolver := &mutationResolver{Resolver: &Resolver{database: database}}
	ctx := authz.WithPrincipal(context.Background(), user)
	result, err := resolver.CreateMyAPIToken(ctx, APITokenCreateInput{Name: "escalated", Scopes: []string{string(authz.DatabaseSQL)}})
	if result != nil || err == nil || err.Error() != "unable to create API token" {
		t.Fatalf("scope escalation did not fail generically: result=%+v err=%v", result, err)
	}
}

package authz

import (
	"context"
	"errors"
	"testing"
)

func TestRequireSurfaceContext(t *testing.T) {
	registry, err := NewRegistry([]Surface{{Kind: SurfaceGraphQLQuery, Name: "findScenes", Capability: LibraryRead}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPrincipal(context.Background(), Principal{UserID: "u1", Role: RoleUser, Status: StatusActive})
	principal, err := RequireSurfaceContext(ctx, registry, SurfaceGraphQLQuery, "findScenes", "")
	if err != nil || principal.UserID != "u1" {
		t.Fatalf("principal=%+v err=%v", principal, err)
	}
	_, err = RequireSurfaceContext(ctx, registry, SurfaceGraphQLQuery, "missing", "")
	var missing UnregisteredSurfaceError
	if !errors.As(err, &missing) {
		t.Fatalf("expected UnregisteredSurfaceError, got %T: %v", err, err)
	}
}

func TestPrincipalContext(t *testing.T) {
	want := Principal{UserID: "u1", Role: RoleModerator, Status: StatusActive}
	ctx := WithPrincipal(context.Background(), want)
	got, err := PrincipalFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != want.UserID || got.Role != want.Role || got.Status != want.Status {
		t.Fatalf("principal=%#v want %#v", got, want)
	}
	if _, err := RequireContext(ctx, MetadataWrite); err != nil {
		t.Fatal(err)
	}
	if _, err := RequireContext(ctx, DatabaseSQL); err == nil {
		t.Fatal("context guard allowed unavailable capability")
	}
}

func TestPrincipalContextMissingFailsClosed(t *testing.T) {
	for _, ctx := range []context.Context{nil, context.Background()} {
		if _, err := PrincipalFromContext(ctx); err == nil {
			t.Fatal("missing principal did not fail closed")
		}
	}
}

func TestOwnedContext(t *testing.T) {
	ctx := WithPrincipal(context.Background(), Principal{UserID: "u1", Role: RoleAdmin, Status: StatusActive})
	if _, err := RequireOwnedContext(ctx, AccountSelfRead, "u2"); err == nil {
		t.Fatal("Admin bypassed owner context guard")
	}
}

package authz

import (
	"errors"
	"reflect"
	"testing"
)

func TestRegistryFailsClosed(t *testing.T) {
	registry, err := NewRegistry([]Surface{{Kind: SurfaceGraphQLQuery, Name: "findScenes", Capability: LibraryRead}})
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{UserID: "u1", Role: RoleAdmin, Status: StatusActive}
	if err := registry.Require(p, SurfaceGraphQLQuery, "unregistered", ""); err == nil {
		t.Fatal("unregistered operation was authorized")
	} else {
		var unregistered UnregisteredSurfaceError
		if !errors.As(err, &unregistered) {
			t.Fatalf("unexpected error type: %T", err)
		}
	}
}

func TestRegistryRejectsInvalidEntries(t *testing.T) {
	tests := [][]Surface{
		{{Kind: "INVENTED", Name: "x", Capability: LibraryRead}},
		{{Kind: SurfaceGraphQLQuery, Name: " ", Capability: LibraryRead}},
		{{Kind: SurfaceGraphQLQuery, Name: "x", Capability: "invented.action"}},
		{{Kind: SurfaceGraphQLQuery, Name: "x", Capability: LibraryRead}, {Kind: SurfaceGraphQLQuery, Name: "x", Capability: LibraryRead}},
	}
	for _, surfaces := range tests {
		if _, err := NewRegistry(surfaces); err == nil {
			t.Fatalf("invalid registry accepted: %#v", surfaces)
		}
	}
}

func TestRegistryEnforcesCapabilityAndOwnership(t *testing.T) {
	registry, err := NewRegistry([]Surface{
		{Kind: SurfaceGraphQLMutation, Name: "sceneUpdate", Capability: MetadataWrite},
		{Kind: SurfaceGraphQLQuery, Name: "myPreferences", Capability: AccountSelfRead, OwnerScoped: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	user := Principal{UserID: "u1", Role: RoleUser, Status: StatusActive}
	moderator := Principal{UserID: "u1", Role: RoleModerator, Status: StatusActive}
	if err := registry.Require(user, SurfaceGraphQLMutation, "sceneUpdate", ""); err == nil {
		t.Fatal("User obtained metadata.write")
	}
	if err := registry.Require(moderator, SurfaceGraphQLMutation, "sceneUpdate", ""); err != nil {
		t.Fatal(err)
	}
	if err := registry.Require(moderator, SurfaceGraphQLQuery, "myPreferences", "u2"); err == nil {
		t.Fatal("owner substitution succeeded")
	}
}

func TestRegistrySurfacesStableOrderAndCopy(t *testing.T) {
	registry, err := NewRegistry([]Surface{
		{Kind: SurfaceGraphQLMutation, Name: "z", Capability: MetadataWrite},
		{Kind: SurfaceGraphQLQuery, Name: "a", Capability: LibraryRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := registry.Surfaces()
	want := []Surface{
		{Kind: SurfaceGraphQLMutation, Name: "z", Capability: MetadataWrite},
		{Kind: SurfaceGraphQLQuery, Name: "a", Capability: LibraryRead},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("surfaces=%#v want %#v", got, want)
	}
	got[0].Name = "mutated"
	if registry.Surfaces()[0].Name == "mutated" {
		t.Fatal("caller mutated registry state")
	}
}

func TestHTTPAccessModes(t *testing.T) {
	registry, err := NewRegistry([]Surface{
		{Kind: SurfaceHTTPRoute, Name: "GET /login", AccessMode: AccessPublic},
		{Kind: SurfaceHTTPRoute, Name: "GET /", AccessMode: AccessAuthenticated},
		{Kind: SurfaceHTTPRoute, Name: "ANY /graphql", AccessMode: AccessPerOperation},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Require(Principal{}, SurfaceHTTPRoute, "GET /login", ""); err != nil {
		t.Fatalf("public route denied: %v", err)
	}
	if err := registry.Require(Principal{}, SurfaceHTTPRoute, "GET /", ""); err == nil {
		t.Fatal("anonymous principal reached authenticated route")
	}
	user := Principal{UserID: "u1", Role: RoleUser, Status: StatusActive}
	if err := registry.Require(user, SurfaceHTTPRoute, "GET /", ""); err != nil {
		t.Fatalf("authenticated route denied: %v", err)
	}
	if err := registry.Require(Principal{}, SurfaceHTTPRoute, "ANY /graphql", ""); err == nil {
		t.Fatal("per-operation route allowed without downstream operation policy")
	}
}

func TestHTTPOnlyAccessModesRejectedForGraphQL(t *testing.T) {
	if _, err := NewRegistry([]Surface{{Kind: SurfaceGraphQLQuery, Name: "findScenes", AccessMode: AccessPublic}}); err == nil {
		t.Fatal("GraphQL surface accepted HTTP-only access mode")
	}
}

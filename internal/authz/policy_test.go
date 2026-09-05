package authz

import "testing"

func TestLoadGraphQLPolicy(t *testing.T) {
	registry, err := LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	surfaces := registry.Surfaces()
	if len(surfaces) != 266 {
		t.Fatalf("surface count=%d want=266", len(surfaces))
	}
	counts := map[SurfaceKind]int{}
	for _, surface := range surfaces {
		counts[surface.Kind]++
	}
	if counts[SurfaceGraphQLQuery] != 91 || counts[SurfaceGraphQLMutation] != 172 || counts[SurfaceGraphQLSubscription] != 3 {
		t.Fatalf("unexpected surface counts: %#v", counts)
	}
}

func TestCamShowsReadPolicy(t *testing.T) {
	registry, err := LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Require(Principal{}, SurfaceGraphQLQuery, "camShows", ""); err == nil {
		t.Fatal("anonymous allowed camShows")
	}
	if err := registry.Require(Principal{UserID: "1", Role: RoleUser, Status: StatusActive}, SurfaceGraphQLQuery, "camShows", ""); err != nil {
		t.Fatalf("user denied camShows: %v", err)
	}
}

func TestLoadHTTPPolicy(t *testing.T) {
	registry, err := LoadHTTPPolicy()
	if err != nil {
		t.Fatal(err)
	}
	surfaces := registry.Surfaces()
	if len(surfaces) != 54 {
		t.Fatalf("surface count=%d want=54", len(surfaces))
	}
	for _, surface := range surfaces {
		if surface.Kind != SurfaceHTTPRoute {
			t.Fatalf("non-HTTP surface in HTTP policy: %+v", surface)
		}
	}
}

func TestCamModelBrowseAndMutationPolicyMatrix(t *testing.T) {
	registry, err := LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	anonymous := Principal{}
	user := Principal{UserID: "1", Role: RoleUser, Status: StatusActive}
	admin := Principal{UserID: "2", Role: RoleAdmin, Status: StatusActive}
	for _, name := range []string{"camModelProfile", "camModelProfiles", "camModelSites"} {
		if err := registry.Require(anonymous, SurfaceGraphQLQuery, name, ""); err == nil {
			t.Fatalf("anonymous allowed %s", name)
		}
		if err := registry.Require(user, SurfaceGraphQLQuery, name, ""); err != nil {
			t.Fatalf("user denied %s: %v", name, err)
		}
	}
	for _, name := range []string{"camModelProfileCreate", "camModelProfileUpdate", "camModelProfileScrape", "camModelAccountAdd", "camModelAccountRetire", "camModelEvidenceCreate", "camModelEvidenceReview"} {
		if err := registry.Require(anonymous, SurfaceGraphQLMutation, name, ""); err == nil {
			t.Fatalf("anonymous allowed %s", name)
		}
		if err := registry.Require(user, SurfaceGraphQLMutation, name, ""); err == nil {
			t.Fatalf("user allowed %s", name)
		}
		if err := registry.Require(admin, SurfaceGraphQLMutation, name, ""); err != nil {
			t.Fatalf("admin denied %s: %v", name, err)
		}
	}
}

func TestCompletedRecordingPolicyMatrix(t *testing.T) {
	registry, err := LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	principals := []Principal{
		{},
		{UserID: "1", Role: RoleUser, Status: StatusActive},
		{UserID: "2", Role: RoleAdmin, Status: StatusActive, TokenScopes: map[Capability]struct{}{LibraryRead: {}}},
	}
	for _, surface := range []struct {
		kind SurfaceKind
		name string
	}{
		{SurfaceGraphQLQuery, "completedRecordingImportConfig"},
		{SurfaceGraphQLMutation, "completedRecordingImportConfigure"},
		{SurfaceGraphQLMutation, "completedRecordingPreview"},
		{SurfaceGraphQLMutation, "completedRecordingApply"},
	} {
		for _, principal := range principals {
			if err := registry.Require(principal, surface.kind, surface.name, ""); err == nil {
				t.Fatalf("%s allowed for %#v", surface.name, principal)
			}
		}
		if err := registry.Require(Principal{UserID: "3", Role: RoleAdmin, Status: StatusActive}, surface.kind, surface.name, ""); err != nil {
			t.Fatalf("active Admin denied %s: %v", surface.name, err)
		}
	}
}

func TestCamGirlFinderPolicyMatrix(t *testing.T) {
	registry, err := LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	anonymous := Principal{}
	user := Principal{UserID: "1", Role: RoleUser, Status: StatusActive}
	admin := Principal{UserID: "2", Role: RoleAdmin, Status: StatusActive}
	for _, surface := range []struct {
		kind SurfaceKind
		name string
	}{
		{SurfaceGraphQLQuery, "camGirlFinderConfig"},
		{SurfaceGraphQLMutation, "camGirlFinderConfigure"},
		{SurfaceGraphQLMutation, "camGirlFinderSearch"},
		{SurfaceGraphQLMutation, "camGirlFinderIngestPending"},
	} {
		if err := registry.Require(anonymous, surface.kind, surface.name, ""); err == nil {
			t.Fatalf("anonymous allowed %s", surface.name)
		}
		if err := registry.Require(user, surface.kind, surface.name, ""); err == nil {
			t.Fatalf("user allowed %s", surface.name)
		}
		if err := registry.Require(admin, surface.kind, surface.name, ""); err != nil {
			t.Fatalf("admin denied %s: %v", surface.name, err)
		}
	}
}

func TestCamDomainWritePolicy(t *testing.T) {
	registry, err := LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"camShowUpdate", "camModelSocialProfileCreate", "camModelSocialProfileRetire"} {
		for _, principal := range []Principal{{}, {UserID: "2", Role: RoleUser, Status: StatusActive}, {UserID: "3", Role: RoleAdmin, Status: StatusActive, TokenScopes: map[Capability]struct{}{LibraryRead: {}}}} {
			if err := registry.Require(principal, SurfaceGraphQLMutation, name, ""); err == nil {
				t.Fatalf("%s allowed for %#v", name, principal)
			}
		}
		if err := registry.Require(Principal{UserID: "1", Role: RoleAdmin, Status: StatusActive}, SurfaceGraphQLMutation, name, ""); err != nil {
			t.Fatalf("Admin denied %s: %v", name, err)
		}
	}
}

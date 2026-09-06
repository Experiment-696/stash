package authz

import "testing"

func TestLoadGraphQLPolicy(t *testing.T) {
	registry, err := LoadGraphQLPolicy()
	if err != nil {
		t.Fatal(err)
	}
	surfaces := registry.Surfaces()
	if len(surfaces) != 234 {
		t.Fatalf("surface count=%d want=234", len(surfaces))
	}
	counts := map[SurfaceKind]int{}
	for _, surface := range surfaces {
		counts[surface.Kind]++
	}
	if counts[SurfaceGraphQLQuery] != 83 || counts[SurfaceGraphQLMutation] != 148 || counts[SurfaceGraphQLSubscription] != 3 {
		t.Fatalf("unexpected surface counts: %#v", counts)
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

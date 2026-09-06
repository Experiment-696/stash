package authz

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type SurfaceKind string
type AccessMode string

const (
	SurfaceGraphQLQuery        SurfaceKind = "GRAPHQL_QUERY"
	SurfaceGraphQLMutation     SurfaceKind = "GRAPHQL_MUTATION"
	SurfaceGraphQLSubscription SurfaceKind = "GRAPHQL_SUBSCRIPTION"
	SurfaceHTTPRoute           SurfaceKind = "HTTP_ROUTE"
	AccessCapability           AccessMode  = "CAPABILITY"
	AccessPublic               AccessMode  = "PUBLIC"
	AccessAuthenticated        AccessMode  = "AUTHENTICATED"
	AccessPerOperation         AccessMode  = "PER_OPERATION"
)

type Surface struct {
	Kind        SurfaceKind `json:"kind"`
	Name        string      `json:"name"`
	Capability  Capability  `json:"capability"`
	OwnerScoped bool        `json:"owner_scoped"`
	AccessMode  AccessMode  `json:"access_mode,omitempty"`
}

func (s Surface) Key() string { return string(s.Kind) + ":" + s.Name }

type Registry struct {
	entries map[string]Surface
}

func NewRegistry(surfaces []Surface) (*Registry, error) {
	registry := &Registry{entries: make(map[string]Surface, len(surfaces))}
	for _, surface := range surfaces {
		if err := validateSurface(surface); err != nil {
			return nil, err
		}
		key := surface.Key()
		if _, exists := registry.entries[key]; exists {
			return nil, fmt.Errorf("duplicate authorization surface %q", key)
		}
		registry.entries[key] = surface
	}
	return registry, nil
}

func (r *Registry) Lookup(kind SurfaceKind, name string) (Surface, error) {
	if r == nil {
		return Surface{}, errors.New("authorization registry is nil")
	}
	key := string(kind) + ":" + name
	surface, ok := r.entries[key]
	if !ok {
		return Surface{}, UnregisteredSurfaceError{Kind: kind, Name: name}
	}
	return surface, nil
}

func (r *Registry) Require(p Principal, kind SurfaceKind, name, ownerUserID string) error {
	surface, err := r.Lookup(kind, name)
	if err != nil {
		return err
	}
	mode := surface.AccessMode
	if mode == "" {
		mode = AccessCapability
	}
	switch mode {
	case AccessPublic:
		return nil
	case AccessPerOperation:
		return DelegatedAuthorizationRequiredError{Kind: kind, Name: name}
	case AccessAuthenticated:
		if !p.IsAuthenticated() {
			return UnauthenticatedError{}
		}
		return nil
	}
	if surface.OwnerScoped {
		return RequireOwned(p, surface.Capability, ownerUserID)
	}
	return Require(p, surface.Capability)
}

func (r *Registry) Surfaces() []Surface {
	if r == nil {
		return nil
	}
	result := make([]Surface, 0, len(r.entries))
	for _, surface := range r.entries {
		result = append(result, surface)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key() < result[j].Key() })
	return result
}

type UnregisteredSurfaceError struct {
	Kind SurfaceKind
	Name string
}

func (e UnregisteredSurfaceError) Error() string {
	return fmt.Sprintf("authorization surface is not registered: %s:%s", e.Kind, e.Name)
}

func (UnregisteredSurfaceError) Code() string          { return "INTERNAL_POLICY_MISSING" }
func (UnregisteredSurfaceError) HTTPStatus() int       { return 500 }
func (UnregisteredSurfaceError) PublicMessage() string { return "request denied" }

type OwnerResolutionRequiredError struct {
	Kind SurfaceKind
	Name string
}

func (e OwnerResolutionRequiredError) Error() string {
	return fmt.Sprintf("target owner must be resolved before authorizing surface: %s:%s", e.Kind, e.Name)
}

func (OwnerResolutionRequiredError) Code() string          { return "INTERNAL_OWNER_UNRESOLVED" }
func (OwnerResolutionRequiredError) HTTPStatus() int       { return 500 }
func (OwnerResolutionRequiredError) PublicMessage() string { return "request denied" }

// DelegatedAuthorizationRequiredError prevents a PER_OPERATION route from
// becoming an allow-all route when its downstream operation guard is absent.
type DelegatedAuthorizationRequiredError struct {
	Kind SurfaceKind
	Name string
}

func (e DelegatedAuthorizationRequiredError) Error() string {
	return fmt.Sprintf("downstream operation authorization is required for surface: %s:%s", e.Kind, e.Name)
}

func (DelegatedAuthorizationRequiredError) Code() string          { return "INTERNAL_DELEGATED_AUTH_REQUIRED" }
func (DelegatedAuthorizationRequiredError) HTTPStatus() int       { return 500 }
func (DelegatedAuthorizationRequiredError) PublicMessage() string { return "request denied" }

func validateSurface(surface Surface) error {
	switch surface.Kind {
	case SurfaceGraphQLQuery, SurfaceGraphQLMutation, SurfaceGraphQLSubscription, SurfaceHTTPRoute:
	default:
		return fmt.Errorf("unknown authorization surface kind %q", surface.Kind)
	}
	if strings.TrimSpace(surface.Name) == "" || surface.Name != strings.TrimSpace(surface.Name) {
		return errors.New("authorization surface name must be non-empty and trimmed")
	}
	mode := surface.AccessMode
	if mode == "" {
		mode = AccessCapability
	}
	switch mode {
	case AccessCapability:
		if !IsKnownCapability(surface.Capability) {
			return fmt.Errorf("surface %q uses an invalid account capability %q", surface.Key(), surface.Capability)
		}
	case AccessPublic, AccessAuthenticated, AccessPerOperation:
		if surface.Kind != SurfaceHTTPRoute {
			return fmt.Errorf("surface %q uses HTTP-only access mode %q", surface.Key(), mode)
		}
		if surface.Capability != "" || surface.OwnerScoped {
			return fmt.Errorf("surface %q access mode %q cannot declare capability or owner scope", surface.Key(), mode)
		}
	default:
		return fmt.Errorf("surface %q uses unknown access mode %q", surface.Key(), mode)
	}
	return nil
}

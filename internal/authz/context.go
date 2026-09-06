package authz

import "context"

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, error) {
	if ctx == nil {
		return Principal{}, UnauthenticatedError{}
	}
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	if !ok || principal.UserID == "" {
		return Principal{}, UnauthenticatedError{}
	}
	return principal, nil
}

func RequireContext(ctx context.Context, capability Capability) (Principal, error) {
	principal, err := PrincipalFromContext(ctx)
	if err != nil {
		return Principal{}, err
	}
	if err := Require(principal, capability); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

func RequireOwnedContext(ctx context.Context, capability Capability, ownerUserID string) (Principal, error) {
	principal, err := PrincipalFromContext(ctx)
	if err != nil {
		return Principal{}, err
	}
	if err := RequireOwned(principal, capability, ownerUserID); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

// RequireSurfaceContext authorizes a registered API surface using the
// authenticated principal carried by ctx. Unknown surfaces fail closed in the
// registry, giving HTTP and GraphQL integration points one enforcement path.
func RequireSurfaceContext(ctx context.Context, registry *Registry, kind SurfaceKind, name, ownerUserID string) (Principal, error) {
	principal, err := PrincipalFromContext(ctx)
	if err != nil {
		return Principal{}, err
	}
	if err := registry.Require(principal, kind, name, ownerUserID); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

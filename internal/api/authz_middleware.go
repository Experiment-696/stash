package api

import (
	"net/http"

	"github.com/stashapp/stash/internal/authz"
)

type ownerIDResolver func(*http.Request) (string, error)

func requireRoute(registry *authz.Registry, routeName string, owner ownerIDResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ownerID := ""
			var err error
			if owner != nil {
				ownerID, err = owner(r)
			}
			if err == nil {
				_, err = authz.RequireSurfaceContext(r.Context(), registry, authz.SurfaceHTTPRoute, routeName, ownerID)
			}
			if err != nil {
				if !writeAuthzHTTPError(w, err) {
					http.Error(w, "request denied", http.StatusInternalServerError)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requireCapability(capability authz.Capability) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := authz.RequireContext(r.Context(), capability); err != nil {
				if !writeAuthzHTTPError(w, err) {
					http.Error(w, "request denied", http.StatusInternalServerError)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := authz.PrincipalFromContext(r.Context())
		if err != nil || !principal.IsAuthenticated() {
			writeAuthzHTTPError(w, authz.UnauthenticatedError{})
			return
		}
		next.ServeHTTP(w, r)
	})
}

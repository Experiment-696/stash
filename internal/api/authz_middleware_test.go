package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stashapp/stash/internal/authz"
)

func TestRequireRoute(t *testing.T) {
	registry, err := authz.NewRegistry([]authz.Surface{{Kind: authz.SurfaceHTTPRoute, Name: "GET /library", Capability: authz.LibraryRead}})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	handler := requireRoute(registry, "GET /library", nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodGet, "/library", nil)
	request = request.WithContext(authz.WithPrincipal(request.Context(), authz.Principal{UserID: "u1", Role: authz.RoleUser, Status: authz.StatusActive}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !called {
		t.Fatalf("authorized route response=%d called=%v", recorder.Code, called)
	}
}

func TestRequireRouteFailsClosed(t *testing.T) {
	registry, err := authz.NewRegistry([]authz.Surface{{Kind: authz.SurfaceHTTPRoute, Name: "GET /admin", Capability: authz.SystemConfigure}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		principal *authz.Principal
		route     string
		status    int
	}{
		{"anonymous", nil, "GET /admin", http.StatusUnauthorized},
		{"forbidden", &authz.Principal{UserID: "u1", Role: authz.RoleUser, Status: authz.StatusActive}, "GET /admin", http.StatusForbidden},
		{"unregistered", &authz.Principal{UserID: "a1", Role: authz.RoleAdmin, Status: authz.StatusActive}, "GET /missing", http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := requireRoute(registry, test.route, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.principal != nil {
				request = request.WithContext(authz.WithPrincipal(request.Context(), *test.principal))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.status || called {
				t.Fatalf("response=%d called=%v want=%d/false", recorder.Code, called, test.status)
			}
		})
	}
}

func TestRequireRouteOwnerResolution(t *testing.T) {
	registry, err := authz.NewRegistry([]authz.Surface{{Kind: authz.SurfaceHTTPRoute, Name: "GET /mine", Capability: authz.AccountSelfRead, OwnerScoped: true}})
	if err != nil {
		t.Fatal(err)
	}
	owner := func(r *http.Request) (string, error) {
		if value := r.URL.Query().Get("owner"); value != "" {
			return value, nil
		}
		return "", errors.New("owner unavailable")
	}
	handler := requireRoute(registry, "GET /mine", owner)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := httptest.NewRequest(http.MethodGet, "/mine?owner=u2", nil)
	request = request.WithContext(authz.WithPrincipal(request.Context(), authz.Principal{UserID: "u1", Role: authz.RoleAdmin, Status: authz.StatusActive}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("owner substitution response=%d", recorder.Code)
	}
}

func TestCapabilityMiddlewareEnforcesStreamScopes(t *testing.T) {
	handler := requireCapability(authz.MediaStream)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
	}))
	for name, principal := range map[string]authz.Principal{
		"anonymous": {},
		"library-only token": {
			UserID: "1", Role: authz.RoleAdmin, Status: authz.StatusActive,
			TokenScopes: map[authz.Capability]struct{}{authz.LibraryRead: {}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/scene/1/stream", nil)
			request = request.WithContext(authz.WithPrincipal(request.Context(), principal))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusPartialContent {
				t.Fatal("stream capability middleware allowed insufficient principal")
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/scene/1/stream", nil)
	request.Header.Set("Range", "bytes=0-7")
	request = request.WithContext(authz.WithPrincipal(request.Context(), authz.Principal{
		UserID: "1", Role: authz.RoleUser, Status: authz.StatusActive,
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("authorized range request status = %d, want 206", recorder.Code)
	}
}

func TestCapabilityMiddlewareEnforcesAssetAndPluginScopes(t *testing.T) {
	tests := []struct {
		name       string
		capability authz.Capability
		principal  authz.Principal
		want       int
	}{
		{
			name: "library asset permits user", capability: authz.LibraryRead,
			principal: authz.Principal{UserID: "1", Role: authz.RoleUser, Status: authz.StatusActive},
			want:      http.StatusNoContent,
		},
		{
			name: "library asset rejects stream-only token", capability: authz.LibraryRead,
			principal: authz.Principal{
				UserID: "1", Role: authz.RoleAdmin, Status: authz.StatusActive,
				TokenScopes: map[authz.Capability]struct{}{authz.MediaStream: {}},
			},
			want: http.StatusForbidden,
		},
		{
			name: "plugin asset rejects ordinary user", capability: authz.ExtensionRead,
			principal: authz.Principal{UserID: "1", Role: authz.RoleUser, Status: authz.StatusActive},
			want:      http.StatusForbidden,
		},
		{
			name: "plugin asset permits admin", capability: authz.ExtensionRead,
			principal: authz.Principal{UserID: "1", Role: authz.RoleAdmin, Status: authz.StatusActive},
			want:      http.StatusNoContent,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := requireCapability(test.capability)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/asset", nil)
			request = request.WithContext(authz.WithPrincipal(request.Context(), test.principal))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}

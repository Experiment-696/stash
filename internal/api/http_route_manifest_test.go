package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/authz"
)

type routeMountEvidence struct {
	source string
	guard  string
}

// protectedRouteMounts is the machine-checkable bridge between the reviewed
// HTTP policy and actual router source. Prefix entries cover every child route
// in that router and require authorization to be mounted before lookup.
var protectedRouteMounts = map[string]routeMountEvidence{
	"ANY /graphql":       {"internal/api/server.go", "graphqlAuthorizationMiddleware"},
	"ANY /playground":    {"internal/api/server.go", "requireCapability(authz.SystemConfigure)"},
	"GET /performer/":    {"internal/api/routes_performer.go", "requireCapability(authz.LibraryRead), rs.PerformerCtx"},
	"GET /studio/":       {"internal/api/routes_studio.go", "requireCapability(authz.LibraryRead), rs.StudioCtx"},
	"GET /group/":        {"internal/api/routes_group.go", "requireCapability(authz.LibraryRead), rs.GroupCtx"},
	"GET /tag/":          {"internal/api/routes_tag.go", "requireCapability(authz.LibraryRead), rs.TagCtx"},
	"GET /gallery/":      {"internal/api/routes_gallery.go", "requireCapability(authz.LibraryRead), rs.GalleryCtx"},
	"GET /image/":        {"internal/api/routes_image.go", "requireCapability(authz.LibraryRead), rs.ImageCtx"},
	"GET /scene/":        {"internal/api/routes_scene.go", "requireCapability(authz."},
	"GET /downloads/":    {"internal/api/routes_downloads.go", "persistedDownloadPrincipal"},
	"GET /plugin/":       {"internal/api/routes_plugin.go", "requireCapability(authz.ExtensionRead), rs.PluginCtx"},
	"ANY /custom/*":      {"internal/api/routes_custom.go", "r.Use(requireCapability(authz.LibraryRead))"},
	"ANY /javascript":    {"internal/api/server.go", `r.With(requireAuthenticated).HandleFunc("/javascript"`},
	"ANY /customlocales": {"internal/api/server.go", `r.With(requireAuthenticated).HandleFunc("/customlocales"`},
	"POST /logout":       {"internal/api/server.go", "requireCapability(authz.AccountSelfWrite)"},
	"ANY /*":             {"internal/api/authentication.go", "if userID == \"\" && !allowUnauthenticated(r)"},
}

var publicRouteRationales = map[string]struct {
	reason, source, registration string
}{
	"GET /healthz":      {"minimal liveness response registered before authentication", "internal/api/server.go", `r.Use(middleware.Heartbeat("/healthz"))`},
	"GET /login":        {"login entry point", "internal/api/server.go", "r.Get(loginEndpoint, handleLogin())"},
	"POST /login":       {"credential exchange with generic failures and rate limits", "internal/api/server.go", "r.Post(loginEndpoint, handleLoginPost())"},
	"GET /login/*":      {"static login application", "internal/api/server.go", `r.HandleFunc(loginEndpoint+"/*"`},
	"GET /login/locale": {"login localization bootstrap", "internal/api/server.go", "r.Get(loginLocaleEndpoint, handleLoginLocale(cfg))"},
	"ANY /css":          {"login page display-only stylesheet containing no library or account data", "internal/api/server.go", `r.HandleFunc("/css", cssHandler(cfg))`},
	"ANY /favicon.ico":  {"branding-only static asset", "internal/api/server.go", `r.HandleFunc("/favicon.ico"`},
	"GET /assets/*":     {"content-addressed application shell assets containing no library data", "internal/api/authentication.go", `strings.HasPrefix(r.URL.Path, "/assets/")`},
}

func TestHTTPPolicyRoutesHaveMountEvidenceOrPublicRationale(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	registry, err := authz.LoadHTTPPolicy()
	if err != nil {
		t.Fatal(err)
	}
	read := func(rel string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	for _, surface := range registry.Surfaces() {
		if public, ok := publicRouteRationales[surface.Name]; ok {
			if surface.AccessMode != authz.AccessPublic || public.reason == "" {
				t.Errorf("public exception %s lacks matching policy/rationale", surface.Name)
			}
			if !strings.Contains(read(public.source), public.registration) {
				t.Errorf("public exception %s is not registered by %q in %s", surface.Name, public.registration, public.source)
			}
			continue
		}
		var evidence *routeMountEvidence
		for routePrefix, candidate := range protectedRouteMounts {
			if surface.Name == routePrefix || strings.HasPrefix(surface.Name, routePrefix) {
				copy := candidate
				evidence = &copy
				break
			}
		}
		if evidence == nil {
			t.Errorf("HTTP policy route %s has no mount evidence or public rationale", surface.Name)
			continue
		}
		if !strings.Contains(read(evidence.source), evidence.guard) {
			t.Errorf("%s missing guard %q in %s", surface.Name, evidence.guard, evidence.source)
		}
	}
}

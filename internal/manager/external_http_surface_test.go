package manager

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExternalHTTPSurfacesHaveExplicitMultiuserDisposition(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	evidence := map[string]struct {
		file, marker, disposition string
	}{
		"DLNA/UPnP HTTP server": {
			"internal/manager/manager.go",
			"GetDLNADefaultEnabled() && s.dlnaAllowedForCurrentAccountMode()",
			"disabled once any Stash account exists because device requests have no principal/capability binding",
		},
		"plugin in-process GraphQL": {
			"internal/api/server.go",
			"pluginCache.RegisterGQLHandler(gqlHandler)",
			"per-operation GraphQL capability middleware; no independent plugin HTTP listener",
		},
		"generated/static application server": {
			"internal/api/http_route_manifest_test.go",
			"publicRouteRationales",
			"served by the main authenticated/public-exception HTTP manifest",
		},
	}
	for name, item := range evidence {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(item.file)))
		if err != nil {
			t.Fatal(err)
		}
		if item.disposition == "" || !strings.Contains(string(body), item.marker) {
			t.Errorf("%s lacks explicit multiuser disposition evidence %q in %s", name, item.marker, item.file)
		}
	}
}

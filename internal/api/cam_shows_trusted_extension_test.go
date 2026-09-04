package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCamShowsTrustedNavigationContract(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	read := func(rel string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	registry := read("ui/v2.5/src/trustedExtensions.tsx")
	for _, required := range []string{
		`path: "/shows"`,
		`menuKey: "shows"`,
		`afterPath: "/scenes"`,
		`path: "/cam-models"`,
		`menuKey: "cam-models"`,
		`afterPath: "/performers"`,
		`capability: "library.read"`,
		`isTrustedRouteEnabled`,
	} {
		if !strings.Contains(registry, required) {
			t.Errorf("trusted registry missing contract %q", required)
		}
	}
	registryCore := read("ui/v2.5/src/trustedExtensionsRegistry.ts")
	for _, required := range []string{"if (!enabled || !capabilities) return []", "getEnabledTrustedRegistryItems", "isTrustedRegistryRouteEnabled", "Trusted extension duplicate id", "Trusted extension duplicate path", "Trusted extension duplicate hotkey"} {
		if !strings.Contains(registryCore, required) {
			t.Errorf("trusted registry missing observable rejection %q", required)
		}
	}

	navbar := read("ui/v2.5/src/components/MainNavbar.tsx")
	for _, required := range []string{
		`insertTrustedNavItems(`,
		`getTrustedNavItems(meData?.me.capabilities, cfgMenuItems)`,
		`const stockItems = allMenuItems.filter`,
		`"href" in item ? item.href : item.path`,
	} {
		if !strings.Contains(navbar, required) {
			t.Errorf("navbar missing trusted extension integration %q", required)
		}
	}

	app := read("ui/v2.5/src/App.tsx")
	for _, required := range []string{
		`isTrustedRouteEnabled("/shows", meData?.me.capabilities)`,
		`isTrustedRouteEnabled("/cam-models", meData?.me.capabilities)`,
		`enabled={meData?.me.role === "ADMIN"}`,
	} {
		if !strings.Contains(app, required) {
			t.Errorf("app missing route or arbitrary-plugin isolation %q", required)
		}
	}

	publicAPI := read("ui/v2.5/src/pluginApi.tsx")
	publicAPITypes := read("ui/v2.5/src/pluginApi.d.ts")
	for _, forbidden := range []string{"trustedNav", "registerNavItem", "trustedExtension", "insertTrustedRegistryItems", "getEnabledTrustedRegistryItems"} {
		if strings.Contains(publicAPI, forbidden) || strings.Contains(publicAPITypes, forbidden) {
			t.Fatalf("trusted navigation symbol %q must not be exposed to arbitrary plugin JavaScript", forbidden)
		}
	}

	developmentPlugin := read("plugins/cam-shows-lite/cam-shows-lite.js")
	if strings.Contains(developmentPlugin, "register.route") ||
		strings.Contains(developmentPlugin, "MainNavBar.MenuItems") {
		t.Fatal("development plugin must not duplicate bundled routes or navigation")
	}
}

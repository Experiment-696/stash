package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Full configuration contains credentials and installation paths. Any new UI
// callsite must be reviewed and added here with an explicit role-based skip.
func TestFrontendFullConfigurationQueriesAreRoleGuarded(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	src := filepath.Join(root, "ui", "v2.5", "src")
	approved := map[string]string{
		filepath.FromSlash("App.tsx"): "skip: !meData?.me || migrationShell",
		filepath.FromSlash("components/Dialogs/IdentifyDialog/IdentifyDialog.tsx"):             "meLoading || !isAdmin",
		filepath.FromSlash("components/Performers/PerformerDetails/PerformerSubmitButton.tsx"): "useConfigurationQuery({ skip: loading || !isAdmin })",
		filepath.FromSlash("components/Settings/context.tsx"):                                  "useConfiguration(!enabled)",
		filepath.FromSlash("components/TroubleshootingMode/useTroubleshootingMode.ts"):         "useConfiguration(meLoading || !isAdmin)",
	}
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == filepath.FromSlash("core/StashService.ts") || rel == filepath.FromSlash("core/generated-graphql.ts") || rel == "pluginApi.d.ts" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		if !strings.Contains(text, "useConfiguration(") && !strings.Contains(text, "useConfigurationQuery(") {
			return nil
		}
		guard, allowed := approved[rel]
		if !allowed || !strings.Contains(text, guard) {
			t.Errorf("unreviewed or unguarded full-configuration query in %s", rel)
		}
		delete(approved, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for missing := range approved {
		t.Errorf("approved guarded callsite missing: %s", missing)
	}
}

func TestAuthenticatedAppUsesFullConfigurationProvider(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "ui", "v2.5", "src", "App.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)

	for _, required := range []string{
		"const fullConfig = GQL.useConfigurationQuery({",
		"skip: !meData?.me || migrationShell",
		"const config = migrationShell",
		": fullConfig;",
		"!migrationShell && (!meData?.me || !config.data?.configuration)",
		"configuration={config.data!.configuration}",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("authenticated configuration provider contract missing %q", required)
		}
	}

	if strings.Contains(
		source,
		"<ConfigurationProvider configuration={shellConfiguration",
	) {
		t.Error("authenticated application must not use the migration shell stub as its configuration provider")
	}
}

func TestShellAdminOnlyQueriesAreSkippedForLowerRoles(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	assertContains := func(rel string, needles ...string) {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range needles {
			if !strings.Contains(string(body), needle) {
				t.Errorf("%s must contain role guard %q", rel, needle)
			}
		}
	}
	assertContains("ui/v2.5/src/App.tsx", `enabled={meData?.me.role === "ADMIN"}`)
	assertContains("ui/v2.5/src/plugins.tsx", "usePlugins(skip)", "if (!enabled) return")
	assertContains("ui/v2.5/src/components/Settings/Tasks/SettingsTasksPanel.tsx", "{isAdmin && (", "<JobTable />")
}

func TestUserAdminActionsRefreshSecurityAudit(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	path := filepath.Join(root, "ui", "v2.5", "src", "components", "Settings", "SettingsUsersPanel.tsx")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"refetch: refetchAuditEvents",
		"await Promise.all([refetch(), refetchAuditEvents()])",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("user-admin action refresh regression: missing %q", required)
		}
	}
}

func TestSettingsUsersPanelAdminRenderContract(t *testing.T) {
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
	settings := read("ui/v2.5/src/components/Settings/Settings.tsx")
	for _, required := range []string{
		`{isAdmin && (`,
		`<Nav.Link eventKey="users">Users</Nav.Link>`,
		`<Tab.Pane eventKey="users" unmountOnExit>`,
		`<SettingsUsersPanel />`,
	} {
		if !strings.Contains(settings, required) {
			t.Errorf("Admin Users navigation/render contract missing %q", required)
		}
	}
	panel := read("ui/v2.5/src/components/Settings/SettingsUsersPanel.tsx")
	for _, required := range []string{
		`const isAdmin = meData?.me.role === "ADMIN" && meData.me.status === "ACTIVE"`,
		`useUsersQuery({ skip: !isAdmin })`,
		`skip: !isAdmin`,
		`<h2>Users</h2>`,
		`<h3>Create user</h3>`,
		`<h3>Security audit</h3>`,
		`await Promise.all([refetch(), refetchAuditEvents()])`,
		`if (meError || !isAdmin) return`,
	} {
		if !strings.Contains(panel, required) {
			t.Errorf("SettingsUsersPanel render/action contract missing %q", required)
		}
	}
}

func TestProtectedAssetRoutesAuthorizeBeforeLookup(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	expected := map[string][]string{
		"internal/api/routes_performer.go": {`requireCapability(authz.LibraryRead), rs.PerformerCtx`},
		"internal/api/routes_studio.go":    {`requireCapability(authz.LibraryRead), rs.StudioCtx`},
		"internal/api/routes_tag.go":       {`requireCapability(authz.LibraryRead), rs.TagCtx`},
		"internal/api/routes_group.go":     {`requireCapability(authz.LibraryRead), rs.GroupCtx`},
		"internal/api/routes_gallery.go":   {`requireCapability(authz.LibraryRead), rs.GalleryCtx`},
		"internal/api/routes_image.go":     {`requireCapability(authz.LibraryRead), rs.ImageCtx`},
		"internal/api/routes_plugin.go":    {`requireCapability(authz.ExtensionRead), rs.PluginCtx`},
	}
	for rel, needles := range expected {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range needles {
			if !strings.Contains(string(body), needle) {
				t.Errorf("%s does not authorize before resource lookup: missing %q", rel, needle)
			}
		}
	}
}

func TestIntegratedSceneListRoleControlsMatchCapabilities(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "ui", "v2.5", "src", "components", "Scenes", "SceneList.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, needle := range []string{
		`const canEditMetadata = isAdmin || meData?.me.role === "MODERATOR"`,
		`onEdit={canEditMetadata ? onEdit : undefined}`,
		`onDelete={isAdmin ? onDelete : undefined}`,
		`if (hasSelection && canEditMetadata)`,
		`if (hasSelection && isAdmin)`,
	} {
		if !strings.Contains(source, needle) {
			t.Errorf("integrated scene role controls missing %q", needle)
		}
	}
}

func TestSceneDetailRoleControlsMatchCapabilities(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "ui", "v2.5", "src", "components", "Scenes", "SceneDetails", "Scene.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, needle := range []string{
		`const canEditMetadata = isAdmin || meData?.me.role === "MODERATOR"`,
		`if (canEditMetadata) setActiveTabKey("scene-edit-panel")`,
		`if (isAdmin) setIsDeleteAlertOpen(true)`,
		`if (isAdmin) onGenerateScreenshot(getPlayerPosition())`,
		`isAdmin ? (`,
		`{canEditMetadata && (`,
		`isAdmin ? () => setIsDeleteAlertOpen(true) : undefined`,
	} {
		if !strings.Contains(source, needle) {
			t.Errorf("scene detail role controls missing %q", needle)
		}
	}
}

func TestEntityListRoleControlsMatchCapabilities(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	files := []string{
		"ui/v2.5/src/components/Performers/PerformerList.tsx",
		"ui/v2.5/src/components/Studios/StudioList.tsx",
		"ui/v2.5/src/components/Tags/TagList.tsx",
		"ui/v2.5/src/components/Galleries/GalleryList.tsx",
		"ui/v2.5/src/components/Images/ImageList.tsx",
		"ui/v2.5/src/components/Groups/GroupList.tsx",
	}
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		for _, needle := range []string{
			`useRoleCapabilities()`,
			`if (hasSelection && canEditMetadata)`,
			`if (hasSelection && isAdmin)`,
			`onEdit={canEditMetadata ? onEdit : undefined}`,
			`onDelete={isAdmin ? onDelete : undefined}`,
			`text: intl.formatMessage({ id: "actions.select_none" })`,
			`isDisplayed: () => hasSelection,`,
			`isDisplayed: () => hasSelection && isAdmin`,
			`isDisplayed: () => isAdmin`,
		} {
			if !strings.Contains(source, needle) {
				t.Errorf("%s role controls missing %q", rel, needle)
			}
		}
	}
	helper := filepath.Join(root, "ui", "v2.5", "src", "hooks", "RoleCapabilities.ts")
	body, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`const isAdmin = data?.me.role === "ADMIN"`,
		`canEditMetadata: isAdmin || data?.me.role === "MODERATOR"`,
	} {
		if !strings.Contains(string(body), needle) {
			t.Errorf("shared entity role helper missing %q", needle)
		}
	}
}

func TestRepresentativeEntityDetailRoleControlsMatchCapabilities(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	for _, rel := range []string{
		"ui/v2.5/src/components/Images/ImageDetails/Image.tsx",
		"ui/v2.5/src/components/Galleries/GalleryDetails/Gallery.tsx",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		for _, needle := range []string{
			`useRoleCapabilities()`,
			`if (!isAdmin) return null`,
			`{canEditMetadata && (`,
			`if (canEditMetadata) setActiveTabKey`,
			`onDelete={isAdmin ? () => setIsDeleteAlertOpen(true) : undefined}`,
		} {
			if !strings.Contains(source, needle) {
				t.Errorf("%s detail role controls missing %q", rel, needle)
			}
		}
	}
}

func TestRemainingEntityDetailRoleControlsMatchCapabilities(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	files := []string{
		"ui/v2.5/src/components/Performers/PerformerDetails/Performer.tsx",
		"ui/v2.5/src/components/Studios/StudioDetails/Studio.tsx",
		"ui/v2.5/src/components/Tags/TagDetails/Tag.tsx",
		"ui/v2.5/src/components/Groups/GroupDetails/Group.tsx",
	}
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		for _, needle := range []string{
			`useRoleCapabilities()`,
			`if (canEditMetadata) toggleEditing()`,
			`isEditing && canEditMetadata`,
			`) : canEditMetadata ? (`,
			`onDelete={isAdmin ? onDelete : undefined}`,
		} {
			if !strings.Contains(source, needle) {
				t.Errorf("%s detail role controls missing %q", rel, needle)
			}
		}
	}
	performer, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(files[0])))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`if (!isAdmin) return null`,
		`onAutoTag={isAdmin ? onAutoTag : undefined}`,
		`{isAdmin && (`,
	} {
		if !strings.Contains(string(performer), needle) {
			t.Errorf("performer privileged detail controls missing %q", needle)
		}
	}
}

// This is the reviewed manifest for privileged GraphQL families reachable from
// shell, navbar, ordinary metadata pages, and the lower-role-safe Tasks route.
// All enforcement is centralized in StashService so callsites fail closed.
func TestLowerRoleReachablePrivilegedQueryManifest(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "ui", "v2.5", "src", "core", "StashService.ts"))
	if err != nil {
		t.Fatal(err)
	}
	service := string(body)
	operations := map[string]string{
		"configuration (system.configure)":    "useConfigurationQuery({ skip: useAdminOnlySkip(skip) })",
		"plugins (extension.read)":            "usePluginsQuery({ skip: useAdminOnlySkip(skip) })",
		"jobQueue (job.read)":                 "skip: useAdminOnlySkip(skip)",
		"jobsSubscribe (job.read)":            "useJobsSubscribeSubscription({ skip: useAdminOnlySkip(skip) })",
		"listSceneScrapers (scraper.run)":     "useListSceneScrapersQuery({ skip: useAdminOnlySkip(skip) })",
		"listPerformerScrapers (scraper.run)": "useListPerformerScrapersQuery({ skip: useAdminOnlySkip(skip) })",
		"listGroupScrapers (scraper.run)":     "useListGroupScrapersQuery({ skip: useAdminOnlySkip(skip) })",
		"listGalleryScrapers (scraper.run)":   "useListGalleryScrapersQuery({ skip: useAdminOnlySkip(skip) })",
		"listImageScrapers (scraper.run)":     "useListImageScrapersQuery({ skip: useAdminOnlySkip(skip) })",
	}
	for operation, guard := range operations {
		if !strings.Contains(service, guard) {
			t.Errorf("%s lacks centralized Admin skip guard", operation)
		}
	}
	if !strings.Contains(service, `data?.me.role !== "ADMIN"`) {
		t.Error("central Admin query guard is not based on the authenticated principal")
	}
}

func TestPreMigrationSPAOperationManifest(t *testing.T) {
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
	app := read("ui/v2.5/src/App.tsx")
	for _, required := range []string{
		"useAppShellConfigurationQuery", "skip: !shellResult || migrationShell",
		"!setupMatch && <TroubleshootingModeOverlay />",
	} {
		if !strings.Contains(app, required) {
			t.Errorf("pre-migration App guard missing %q", required)
		}
	}
	migrate := read("ui/v2.5/src/components/Setup/Migrate.tsx")
	for _, forbidden := range []string{"useMonitorJob", "useFindJob", "useJobsSubscribe", "usePlugins", "useMeQuery", "useConfiguration", "refetchSystemStatus", "window.location.reload"} {
		if strings.Contains(migrate, forbidden) {
			t.Errorf("pre-migration UI contains forbidden operation family %q", forbidden)
		}
	}
	for _, required := range []string{"useMigrationStatus", "mutateMigrate", "window.location.assign(`${baseURL}login`)"} {
		if !strings.Contains(migrate, required) {
			t.Errorf("pre-migration operation manifest missing %q", required)
		}
	}
	service := read("ui/v2.5/src/core/StashService.ts")
	if !strings.Contains(service, "GQL.useMeQuery({ skip })") {
		t.Error("central Admin query guard can issue Me while its parent query is skipped")
	}
}

func TestAppBootstrapConfigurationReadsAreNullSafe(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "ui", "v2.5", "src", "App.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	app := string(body)
	for _, required := range []string{
		`config.data?.configuration?.ui?.lastNoteSeen`,
		`config.data?.configuration?.ui?.title`,
	} {
		if !strings.Contains(app, required) {
			t.Errorf("App bootstrap read is not null-safe: missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`config.data?.configuration.ui.lastNoteSeen`,
		`config.data?.configuration.ui.title`,
	} {
		if strings.Contains(app, forbidden) {
			t.Errorf("App bootstrap retains unsafe configuration read %q", forbidden)
		}
	}
}

func TestSettingModalDoesNotUnmountBeforeFormSubmit(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "ui", "v2.5", "src", "components", "Settings", "Inputs.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	inputs := string(body)
	if strings.Contains(inputs, `onClick={() => close(currentValue)}`) {
		t.Fatal("SettingModal submit button closes and unmounts the form before submit")
	}
	prevent := strings.Index(inputs, "e.preventDefault();")
	close := strings.Index(inputs, "close(currentValue);")
	if prevent < 0 || close < 0 || prevent > close {
		t.Fatal("SettingModal must prevent native submission before closing the modal")
	}
}

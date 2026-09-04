package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stashapp/stash/ui"
	"github.com/vearutop/statigz"
)

func migrationPOST(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func migrationDocumentRequest(t *testing.T, operationName, query string) *http.Request {
	t.Helper()
	body, err := json.Marshal(migrationGraphQLRequest{Query: query, OperationName: operationName})
	if err != nil {
		t.Fatal(err)
	}
	return migrationPOST(string(body))
}

func TestMigrationGraphQLDocumentAllowlist(t *testing.T) {
	allowed := []string{
		`{"operationName":"MigrationStatus","query":"query MigrationStatus { migrationStatus { status currentSchema requiredSchema backupWillBeCreated } }"}`,
		`{"operationName":"RunMigration","query":"mutation RunMigration { alias: migrate }"}`,
		`{"operationName":"MigrationStatus","query":"query MigrationStatus { ...Safe } fragment Safe on Query { migrationStatus { status } }"}`,
	}
	for _, body := range allowed {
		request := migrationPOST(body)
		if err := validateMigrationGraphQLRequest(request); err != nil {
			t.Fatalf("allowed document denied: %v", err)
		}
		preserved := new(bytes.Buffer)
		_, _ = preserved.ReadFrom(request.Body)
		if preserved.String() != body {
			t.Fatal("validator did not preserve the request body for gqlgen")
		}
	}

	denied := []string{
		`[]`,
		`[{"operationName":"RunMigration","query":"mutation RunMigration { migrate }"}]`,
		`{"query":"mutation { migrate }"}`,
		`{"operationName":"A","query":"query A { migrationStatus { status } } query B { migrationStatus { status } }"}`,
		`{"operationName":"Mixed","query":"mutation Mixed { migrate configureGeneral(input: {}) { databasePath } }"}`,
		`{"operationName":"Status","query":"query Status { migrationStatus { status } systemStatus { databasePath } }"}`,
		`{"operationName":"Introspection","query":"query Introspection { __schema { queryType { name } } }"}`,
		`{"operationName":"Typename","query":"query Typename { __typename }"}`,
		`{"operationName":"Setup","query":"mutation Setup { setup(input: {}) }"}`,
		`{"operationName":"Sub","query":"subscription Sub { jobsSubscribe { id } }"}`,
		`{"operationName":"Wrong","query":"query Status { migrationStatus { status } }"}`,
	}
	for _, body := range denied {
		if err := validateMigrationGraphQLRequest(migrationPOST(body)); err == nil {
			t.Fatalf("forbidden document accepted: %s", body)
		}
	}
}

func TestMigrationGraphQLGETMutationDenied(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/graphql?operationName=RunMigration&query=mutation%20RunMigration%20%7B%20migrate%20%7D", nil)
	if !rejectGraphQLMutationOverGET(httptest.NewRecorder(), request) {
		t.Fatal("GET mutation was not rejected before execution")
	}
}

func TestMigrationFragmentTraversalIsCycleSafeAndBounded(t *testing.T) {
	for name, query := range map[string]string{
		"direct self cycle":  `query Status { ...A } fragment A on Query { ...A }`,
		"two fragment cycle": `query Status { ...A } fragment A on Query { ...B } fragment B on Query { ...A }`,
		"missing fragment":   `query Status { ...Missing }`,
		"repeated expansion": `query Status { ...Safe ...Safe } fragment Safe on Query { migrationStatus { status } }`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateMigrationGraphQLRequest(migrationDocumentRequest(t, "Status", query)); err == nil {
				t.Fatal("unsafe fragment graph was accepted")
			}
		})
	}

	chain := func(fragmentCount int) string {
		var query strings.Builder
		query.WriteString("query Status { ...F0 }")
		for i := 0; i < fragmentCount; i++ {
			fmt.Fprintf(&query, " fragment F%d on Query { ", i)
			if i+1 == fragmentCount {
				query.WriteString("migrationStatus { status }")
			} else {
				fmt.Fprintf(&query, "...F%d", i+1)
			}
			query.WriteString(" }")
		}
		return query.String()
	}
	if err := validateMigrationGraphQLRequest(migrationDocumentRequest(t, "Status", chain(maxMigrationFragmentDepth))); err != nil {
		t.Fatalf("fragment chain at depth limit denied: %v", err)
	}
	if err := validateMigrationGraphQLRequest(migrationDocumentRequest(t, "Status", chain(maxMigrationFragmentDepth+1))); err == nil {
		t.Fatal("fragment chain over depth limit accepted")
	}

	var oversized strings.Builder
	oversized.WriteString("query Status {")
	for i := 0; i <= maxMigrationSelections; i++ {
		fmt.Fprintf(&oversized, " f%d: migrationStatus { status }", i)
	}
	oversized.WriteString(" }")
	if err := validateMigrationGraphQLRequest(migrationDocumentRequest(t, "Status", oversized.String())); err == nil {
		t.Fatal("oversized selection graph accepted")
	}
}

func TestMigrationRequestHTTPRouteAllowlistIsCredentialModeIndependent(t *testing.T) {
	allowed := []string{"/", "/index.html", "/graphql", "/css", "/javascript", "/customlocales", "/favicon.ico", "/manifest.json", "/stash_icon.svg", "/apple-touch-icon.png", "/assets/index-deadbeef.js"}
	denied := []string{
		"/scene/1/stream", "/configuration", "/jobs", "/plugin/example", "/users", "/completed-recording-import",
		"/login", "/setup", "/javascript/", "/javascript/example.js", "/customlocales/", "/customlocales/example.json",
		"/javascript.map", "/javascriptx", "/customlocales.json", "/customlocalesx",
		"/manifest.json/", "/manifest.json.bak", "/manifest.jsonx", "/Manifest.json",
		"/stash_icon.svg/", "/stash_icon.svg.bak", "/stash_icon.svgx", "/Stash_icon.svg",
		"/subpath/stash_icon.svg", "/custom/stash_icon.svg", "/other.svg",
		"/assets/", "/arbitrary.txt", "/stash/graphql", "/stash/javascript",
		"/stash/customlocales", "/stash/manifest.json", "/stash/stash_icon.svg", "/stash/",
	}
	for _, hasCredentials := range []bool{false, true} {
		mode := fmt.Sprintf("has_credentials_%t", hasCredentials)
		t.Run(mode, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !migrationRequestRouteAllowed(r) {
					http.Error(w, "request denied", http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			for _, path := range allowed {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("Forwarded", "for=127.0.0.1")
				request.Header.Set("X-Forwarded-For", "127.0.0.1")
				request.Header.Set("X-Real-IP", "127.0.0.1")
				request.Header.Set("X-Forwarded-Prefix", "/stash")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusNoContent {
					t.Errorf("allowed migration route %q denied in %s", path, mode)
				}
			}
			for _, path := range denied {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Host = "trusted.example"
				request.Header.Set("Forwarded", "for=127.0.0.1")
				request.Header.Set("X-Forwarded-For", "127.0.0.1")
				request.Header.Set("X-Real-IP", "127.0.0.1")
				request.Header.Set("X-Forwarded-Prefix", "/stash")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusForbidden || strings.TrimSpace(response.Body.String()) != "request denied" {
					t.Errorf("forbidden migration route %q not identically denied in %s: %d %q", path, mode, response.Code, response.Body.String())
				}
			}
		})
	}
}

func TestMigrationShellManifestIsEmbeddedValidJSON(t *testing.T) {
	compressed, err := ui.UIBox.Open("manifest.json.gz")
	if err != nil {
		t.Fatalf("reading embedded migration shell manifest: %v", err)
	}
	defer compressed.Close()
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		t.Fatalf("opening embedded migration shell manifest: %v", err)
	}
	defer reader.Close()
	manifest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("decompressing embedded migration shell manifest: %v", err)
	}
	if !json.Valid(manifest) {
		t.Fatal("embedded migration shell manifest is not valid JSON")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(manifest, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"name", "icons"} {
		if _, ok := parsed[field]; !ok {
			t.Fatalf("embedded migration shell manifest is missing %q", field)
		}
	}
}

func TestMigrationShellStashIconIsExactEmbeddedSVG(t *testing.T) {
	compressed, err := ui.UIBox.Open("stash_icon.svg.gz")
	if err != nil {
		t.Fatalf("reading embedded migration shell icon: %v", err)
	}
	defer compressed.Close()
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		t.Fatalf("opening embedded migration shell icon: %v", err)
	}
	defer reader.Close()
	icon, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("decompressing embedded migration shell icon: %v", err)
	}
	if !bytes.Contains(icon, []byte("<svg")) {
		t.Fatal("embedded migration shell icon is not SVG content")
	}

	staticUI := statigz.FileServer(ui.UIBox.(fs.ReadDirFS))
	request := httptest.NewRequest(http.MethodGet, "/stash_icon.svg", nil)
	response := httptest.NewRecorder()
	staticUI.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("embedded migration shell icon status=%d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/svg+xml") {
		t.Fatalf("embedded migration shell icon content type=%q", got)
	}
}

func TestMigrationShellAssetsUseOneCredentialIndependentPredicate(t *testing.T) {
	source, err := os.ReadFile("authentication.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(source, []byte("!migrationRequestRouteAllowed(r)")); got != 2 {
		t.Fatalf("migration route predicate enforcement count=%d want 2 (primary gate and no-credentials DB gate)", got)
	}
	for _, staleDuplicate := range [][]byte{
		[]byte(`migrationShellAsset :=`),
		[]byte(`r.URL.Path == "/favicon.ico" || strings.HasPrefix(r.URL.Path, "/assets/")`),
	} {
		if bytes.Contains(source, staleDuplicate) {
			t.Fatalf("credential-specific migration asset allowlist remains: %q", staleDuplicate)
		}
	}
}

func TestMigrationRouteGatePrecedesCredentialModeBranching(t *testing.T) {
	source, err := os.ReadFile("authentication.go")
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(source, []byte("func authenticateHandler()"))
	if start < 0 {
		t.Fatal("authenticateHandler implementation not found")
	}
	authentication := source[start:]
	gate := bytes.Index(authentication, []byte("if migrationRequest && !migrationRequestRouteAllowed(r)"))
	legacyCredentialBranch := bytes.Index(authentication, []byte("if !c.HasCredentials() && mgr.Database != nil"))
	normalCredentialBranch := bytes.Index(authentication, []byte("authRequired := c.HasCredentials()"))
	if gate < 0 || legacyCredentialBranch < 0 || normalCredentialBranch < 0 {
		t.Fatalf("required route/credential markers missing: gate=%d legacy=%d normal=%d", gate, legacyCredentialBranch, normalCredentialBranch)
	}
	if gate > legacyCredentialBranch || gate > normalCredentialBranch {
		t.Fatalf("migration route gate must precede every credential-mode branch: gate=%d legacy=%d normal=%d", gate, legacyCredentialBranch, normalCredentialBranch)
	}
}

func TestMigrationBootstrapSourceIdentityIsNotConsulted(t *testing.T) {
	source, err := os.ReadFile("authentication.go")
	if err != nil {
		t.Fatal(err)
	}
	windowStart := bytes.Index(source, []byte("func migrationWindowOpen"))
	windowEnd := bytes.Index(source[windowStart:], []byte("func authenticateHandler"))
	if windowStart < 0 || windowEnd < 0 {
		t.Fatal("migration window implementation not found")
	}
	window := string(source[windowStart : windowStart+windowEnd])
	for _, forbidden := range []string{"RemoteAddr", "Forwarded", "X-Forwarded-For", "X-Real-IP", "Host", "IsLocalRequest", "HasCredentials"} {
		if strings.Contains(window, forbidden) {
			t.Fatalf("migration authority consults forbidden source %q", forbidden)
		}
	}
}

func TestMigrationBootstrapSchemaAndUIAreMinimal(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	typeSchema := read("../../graphql/schema/types/migration-bootstrap.graphql")
	for _, required := range []string{"status: SystemStatusEnum!", "currentSchema: Int!", "requiredSchema: Int!", "backupWillBeCreated: Boolean!"} {
		if !strings.Contains(typeSchema, required) {
			t.Fatalf("minimal migration status missing %q", required)
		}
	}
	for _, forbidden := range []string{"Path", "config", "working", "home", "ffmpeg", "ffprobe"} {
		if strings.Contains(typeSchema, forbidden) {
			t.Fatalf("migration status exposes forbidden field family %q", forbidden)
		}
	}
	mutation := read("../../ui/v2.5/graphql/mutations/config.graphql")
	if !strings.Contains(mutation, "mutation Migrate {\n  migrate\n}") {
		t.Fatal("migration mutation is not parameter-free")
	}
	migrateUI := read("../../ui/v2.5/src/components/Setup/Migrate.tsx")
	for _, forbidden := range []string{"useSystemStatus", "appShellConfiguration", "databasePath", "backupPath"} {
		if strings.Contains(migrateUI, forbidden) {
			t.Fatalf("migration UI depends on forbidden data %q", forbidden)
		}
	}
}

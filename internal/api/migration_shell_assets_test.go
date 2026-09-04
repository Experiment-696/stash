package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/session"
)

type unreadableMigrationAssetConfig struct{}

func (unreadableMigrationAssetConfig) GetCSSEnabled() bool {
	panic("migration handler read CSS enabled config")
}
func (unreadableMigrationAssetConfig) GetCSSPath() string {
	panic("migration handler read CSS path")
}
func (unreadableMigrationAssetConfig) GetJavascriptEnabled() bool {
	panic("migration handler read javascript enabled config")
}

func assertMigrationAssetNoStore(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("migration asset Cache-Control=%q want no-store", got)
	}
	if got := response.Header().Get("ETag"); got != "" {
		t.Fatalf("migration asset ETag=%q want empty", got)
	}
}
func (unreadableMigrationAssetConfig) GetDisableCustomizations() bool {
	panic("migration handler read customization config")
}
func (unreadableMigrationAssetConfig) GetJavascriptPath() string {
	panic("migration handler read javascript path")
}
func (unreadableMigrationAssetConfig) GetCustomLocalesEnabled() bool {
	panic("migration handler read custom locales enabled config")
}
func (unreadableMigrationAssetConfig) GetCustomLocalesPath() string {
	panic("migration handler read custom locales path")
}

func migrationAssetContext(t *testing.T) context.Context {
	t.Helper()
	session.ConsumeMigrationToken()
	t.Cleanup(session.ConsumeMigrationToken)
	now := time.Now()
	handoff, _, err := session.EnsureMigrationToken(now, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := os.ReadFile(handoff)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://stash/", nil)
	request.URL.RawQuery = session.MigrationTokenQuery + "=" + strings.TrimSpace(string(bearer))
	request, _, _, exchanged := session.SetMigrationRequest(request, now)
	if !exchanged || !session.IsMigrationRequest(request.Context()) {
		t.Fatal("failed to establish migration request context")
	}
	return request.Context()
}

func TestMigrationShellCustomizationHandlersReturnInertContentWithoutReadingConfig(t *testing.T) {
	ctx := migrationAssetContext(t)
	request := httptest.NewRequest(http.MethodGet, "/css", nil).WithContext(ctx)
	request.Header.Set("If-None-Match", `"configured-customization"`)
	css := httptest.NewRecorder()
	cssHandler(unreadableMigrationAssetConfig{})(css, request)
	if css.Code != http.StatusOK || css.Body.Len() != 0 {
		t.Fatalf("migration CSS response: status=%d body=%q", css.Code, css.Body.String())
	}
	if got := css.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("migration CSS content type=%q", got)
	}
	assertMigrationAssetNoStore(t, css)

	request = httptest.NewRequest(http.MethodGet, "/javascript", nil).WithContext(ctx)
	request.Header.Set("If-None-Match", `"configured-customization"`)
	javascript := httptest.NewRecorder()
	javascriptHandler(unreadableMigrationAssetConfig{})(javascript, request)
	if javascript.Code != http.StatusOK || javascript.Body.Len() != 0 {
		t.Fatalf("migration javascript response: status=%d body=%q", javascript.Code, javascript.Body.String())
	}
	if got := javascript.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Fatalf("migration javascript content type=%q", got)
	}
	assertMigrationAssetNoStore(t, javascript)

	request = httptest.NewRequest(http.MethodGet, "/customlocales", nil).WithContext(ctx)
	request.Header.Set("If-None-Match", `"configured-customization"`)
	locales := httptest.NewRecorder()
	customLocalesHandler(unreadableMigrationAssetConfig{})(locales, request)
	if locales.Code != http.StatusOK || locales.Body.String() != "{}" {
		t.Fatalf("migration locales response: status=%d body=%q", locales.Code, locales.Body.String())
	}
	if got := locales.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("migration locales content type=%q", got)
	}
	assertMigrationAssetNoStore(t, locales)
}

func TestNormalCustomizationHandlersPreserveEnabledConfiguredContent(t *testing.T) {
	dir := t.TempDir()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(dir, "config.yml"))
	cfg.SetBool(config.CSSEnabled, true)
	cfg.SetBool(config.JavascriptEnabled, true)
	cfg.SetBool(config.CustomLocalesEnabled, true)
	cfg.SetBool(config.DisableCustomizations, false)
	cssSentinel := `.migration-confirm { display: none !important; }`
	javascriptSentinel := `window.__MIGRATION_SENTINEL__ = document.querySelector('meta[name="stash-migration-csrf"]');`
	localesSentinel := `{"setup":{"migrate":{"migration_required":"SENTINEL COPY"}}}`
	if err := os.WriteFile(cfg.GetCSSPath(), []byte(cssSentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.GetJavascriptPath(), []byte(javascriptSentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.GetCustomLocalesPath(), []byte(localesSentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	authenticated := authz.WithPrincipal(context.Background(), authz.Principal{
		UserID: "admin",
		Role:   authz.RoleAdmin,
		Status: authz.StatusActive,
	})

	css := httptest.NewRecorder()
	cssRequest := httptest.NewRequest(http.MethodGet, "/css", nil).WithContext(authenticated)
	cssHandler(cfg)(css, cssRequest)
	if css.Code != http.StatusOK || !strings.Contains(css.Body.String(), cssSentinel) {
		t.Fatalf("normal CSS response: status=%d body=%q", css.Code, css.Body.String())
	}
	if got := css.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("normal CSS content type=%q", got)
	}

	javascript := httptest.NewRecorder()
	javascriptRequest := httptest.NewRequest(http.MethodGet, "/javascript", nil).WithContext(authenticated)
	javascriptHandler(cfg)(javascript, javascriptRequest)
	if javascript.Code != http.StatusOK || !strings.Contains(javascript.Body.String(), javascriptSentinel) {
		t.Fatalf("normal javascript response: status=%d body=%q", javascript.Code, javascript.Body.String())
	}
	if got := javascript.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Fatalf("normal javascript content type=%q", got)
	}

	locales := httptest.NewRecorder()
	localesRequest := httptest.NewRequest(http.MethodGet, "/customlocales", nil).WithContext(authenticated)
	customLocalesHandler(cfg)(locales, localesRequest)
	if locales.Code != http.StatusOK || locales.Body.String() != localesSentinel {
		t.Fatalf("normal locales response: status=%d body=%q", locales.Code, locales.Body.String())
	}
	if got := locales.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("normal locales content type=%q", got)
	}
}

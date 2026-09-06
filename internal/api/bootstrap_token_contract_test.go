package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBootstrapTokenContractDoesNotTrustForwardingHeaders(t *testing.T) {
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

	token := read("pkg/session/bootstrap_token.go")
	for _, required := range []string{
		`BootstrapTokenHeader = "X-Stash-Bootstrap-Token"`,
		`BootstrapTokenQuery  = "bootstrap_token"`,
		`BootstrapTokenCookie = "stash_bootstrap"`,
		`subtle.ConstantTimeCompare`,
		`bootstrapTokenTTL`,
		`HttpOnly: true`,
		`SameSite: http.SameSiteStrictMode`,
		`ConsumeBootstrapToken`,
	} {
		if !strings.Contains(token, required) {
			t.Errorf("bootstrap token contract missing %q", required)
		}
	}
	if strings.Contains(token, "X-Forwarded-For") || strings.Contains(token, "Forwarded") {
		t.Fatal("bootstrap authority must not trust forwarding headers")
	}

	auth := read("internal/api/authentication.go")
	for _, required := range []string{
		`session.EnsureBootstrapToken(time.Now())`,
		`session.SetBootstrapRequest(r, time.Now())`,
		`query.Del(session.BootstrapTokenQuery)`,
		`http.StatusSeeOther`,
		`migrationWindowOpen(mgr.Database)`,
	} {
		if !strings.Contains(auth, required) {
			t.Errorf("authentication bootstrap flow missing %q", required)
		}
	}

	resolver := read("internal/api/resolver_user_admin.go")
	if !strings.Contains(resolver, "session.ConsumeBootstrapToken()") {
		t.Fatal("first Admin bootstrap must consume the one-time token")
	}
}

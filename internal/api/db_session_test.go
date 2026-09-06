package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/sqlite"
)

func TestSetDBSessionCookiesSecureFlag(t *testing.T) {
	cases := []struct {
		name           string
		realTLS        bool
		forwardedProto string
		wantSecure     bool
	}{
		{"real TLS, no forwarded header", true, "", true},
		{"no TLS, no forwarded header", false, "", false},
		{"no TLS, spoofed https forwarded header", false, "https", false},
		{"no TLS, spoofed HTTPS (mixed case) forwarded header", false, "HTTPS", false},
		{"real TLS, conflicting http forwarded header", true, "http", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://example.com/login", nil)
			if c.realTLS {
				state := tls.ConnectionState{}
				r.TLS = &state
			}
			if c.forwardedProto != "" {
				r.Header.Set("X-Forwarded-Proto", c.forwardedProto)
			}
			w := httptest.NewRecorder()
			setDBSessionCookies(w, r, &sqlite.SessionCredentials{ID: "id", Secret: "secret", CSRFSecret: "csrf"})
			for _, ck := range w.Result().Cookies() {
				if ck.Secure != c.wantSecure {
					t.Fatalf("cookie=%s Secure=%v want %v (realTLS=%v forwardedProto=%q) — a client-controllable header must never set Secure without real TLS (P1A-F013)", ck.Name, ck.Secure, c.wantSecure, c.realTLS, c.forwardedProto)
				}
			}
		})
	}
}

func TestReadDBSessionCookieMalformed(t *testing.T) {
	cases := []struct {
		name  string
		value string
		ok    bool
	}{
		{"well-formed", "abc123.def456", true},
		{"no dot at all", "abc123def456", false},
		{"empty id half", ".def456", false},
		{"empty secret half", "abc123.", false},
		{"only a dot", ".", false},
		{"empty string", "", false},
		{"extra dot lands in secret half", "abc123.def.456", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
			r.AddCookie(&http.Cookie{Name: dbSessionCookie, Value: c.value})
			id, secret, ok := readDBSessionCookie(r)
			if ok != c.ok {
				t.Fatalf("value=%q id=%q secret=%q ok=%v want ok=%v", c.value, id, secret, ok, c.ok)
			}
		})
	}
}

func TestReadDBSessionCookieMissingCookie(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	if _, _, ok := readDBSessionCookie(r); ok {
		t.Fatal("expected ok=false when no session cookie is present at all")
	}
}

func TestSetDBSessionCookiesPathScoping(t *testing.T) {
	cases := []struct {
		name     string
		prefix   string
		wantPath string
	}{
		{"no proxy prefix", "", "/"},
		{"simple prefix", "/stash", "/stash"},
		{"trailing slash trimmed", "/stash/", "/stash"},
		{"nested prefix", "/proxy/stash", "/proxy/stash"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://example.com/login", nil)
			if c.prefix != "" {
				r.Header.Set("X-Forwarded-Prefix", c.prefix)
			}
			w := httptest.NewRecorder()
			setDBSessionCookies(w, r, &sqlite.SessionCredentials{ID: "id", Secret: "secret", CSRFSecret: "csrf"})
			for _, ck := range w.Result().Cookies() {
				if ck.Path != c.wantPath {
					t.Fatalf("cookie=%s path=%q want %q", ck.Name, ck.Path, c.wantPath)
				}
			}
		})
	}
}

func TestSetDBSessionCookiesCSRFSessionSeparation(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "http://example.com/login", nil)
	w := httptest.NewRecorder()
	setDBSessionCookies(w, r, &sqlite.SessionCredentials{ID: "sid", Secret: "ssecret", CSRFSecret: "csecret"})

	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected exactly 2 cookies, got %d", len(cookies))
	}
	var session, csrf *http.Cookie
	for _, ck := range cookies {
		switch ck.Name {
		case dbSessionCookie:
			session = ck
		case dbCSRFCookie:
			csrf = ck
		}
	}
	if session == nil || csrf == nil {
		t.Fatalf("expected both %q and %q cookies, got %+v", dbSessionCookie, dbCSRFCookie, cookies)
	}
	if !session.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if csrf.HttpOnly {
		t.Fatal("CSRF cookie must NOT be HttpOnly — it must be script-readable for the double-submit pattern")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie SameSite=%v want Lax", session.SameSite)
	}
	if csrf.SameSite != http.SameSiteStrictMode {
		t.Fatalf("CSRF cookie SameSite=%v want Strict", csrf.SameSite)
	}
	if csrf.Value == session.Value {
		t.Fatal("CSRF cookie value must not equal the session cookie value")
	}
	if strings.Contains(session.Value, csrf.Value) {
		t.Fatal("session cookie value must not embed the CSRF secret")
	}
}

func TestSetThenReadDBSessionCookieRoundTrip(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "http://example.com/login", nil)
	w := httptest.NewRecorder()
	creds := &sqlite.SessionCredentials{ID: "roundtrip-id", Secret: "roundtrip-secret", CSRFSecret: "roundtrip-csrf"}
	setDBSessionCookies(w, r, creds)

	next := httptest.NewRequest(http.MethodGet, "http://example.com/graphql", nil)
	for _, ck := range w.Result().Cookies() {
		next.AddCookie(ck)
	}
	id, secret, ok := readDBSessionCookie(next)
	if !ok {
		t.Fatal("round-trip cookie failed to parse")
	}
	if id != creds.ID || secret != creds.Secret {
		t.Fatalf("round-trip mismatch: id=%q secret=%q want id=%q secret=%q", id, secret, creds.ID, creds.Secret)
	}
}

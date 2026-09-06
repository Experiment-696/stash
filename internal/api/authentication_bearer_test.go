package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadDBBearerToken(t *testing.T) {
	tests := []struct {
		name, header, id, secret string
		ok                       bool
	}{
		{name: "valid", header: "Bearer token-id.token-secret", id: "token-id", secret: "token-secret", ok: true},
		{name: "surrounding whitespace", header: "Bearer   token-id.token-secret  ", id: "token-id", secret: "token-secret", ok: true},
		{name: "missing", header: ""},
		{name: "wrong scheme", header: "Basic token-id.token-secret"},
		{name: "missing separator", header: "Bearer token-secret"},
		{name: "empty id", header: "Bearer .token-secret"},
		{name: "empty secret", header: "Bearer token-id."},
		{name: "ambiguous", header: "Bearer token-id.token.secret"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/graphql", nil)
			r.Header.Set("Authorization", test.header)
			id, secret, ok := readDBBearerToken(r)
			if id != test.id || secret != test.secret || ok != test.ok {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", id, secret, ok, test.id, test.secret, test.ok)
			}
		})
	}
}

func TestShouldRecoverInvalidDBSession(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/", true},
		{http.MethodGet, "/scenes/1", true},
		{http.MethodGet, "/index.html", true},
		{http.MethodGet, loginEndpoint, true},
		{http.MethodPost, loginEndpoint, true},
		{http.MethodGet, logoutEndpoint, true},
		{http.MethodPost, gqlEndpoint, false},
		{http.MethodGet, gqlEndpoint, false},
		{http.MethodGet, "/assets/index.js", true},
		{http.MethodHead, "/assets/index.css", true},
		{http.MethodGet, "/css", true},
		{http.MethodHead, "/css", true},
		{http.MethodGet, "/manifest.json", true},
		{http.MethodGet, "/apple-touch-icon.png", true},
		{http.MethodGet, "/scene/1/stream", true},
	}
	for _, test := range tests {
		r := httptest.NewRequest(test.method, test.path, nil)
		if got := shouldRecoverInvalidDBSession(r); got != test.want {
			t.Errorf("%s %s recover=%v want %v", test.method, test.path, got, test.want)
		}
	}
}

func TestLegacyMigrationAuthenticationExcludesAPIKeys(t *testing.T) {
	for _, test := range []struct {
		name, header, query, userID string
		err                         error
		want                        bool
	}{
		{"valid cookie result", "", "", "admin", nil, true},
		{"legacy API header", "secret", "", "admin", nil, false},
		{"legacy API query", "", "secret", "admin", nil, false},
		{"expired cookie", "", "", "", errors.New("expired"), false},
		{"missing identity", "", "", "", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/graphql", nil)
			r.Header.Set("ApiKey", test.header)
			q := r.URL.Query()
			q.Set("apikey", test.query)
			r.URL.RawQuery = q.Encode()
			if got := isLegacyCookieAuthentication(r, test.userID, test.err); got != test.want {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}

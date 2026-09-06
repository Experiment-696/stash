package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
)

func TestRejectGraphQLMutationOverGETAmbiguousDocument(t *testing.T) {
	query := `query A { version { version } } mutation B { stopAllJobs }`
	r := httptest.NewRequest(http.MethodGet, "/graphql?query="+url.QueryEscape(query), nil)
	w := httptest.NewRecorder()
	if !rejectGraphQLMutationOverGET(w, r) || w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ambiguous GET document rejected=%v status=%d", w.Code == http.StatusMethodNotAllowed, w.Code)
	}
}

func TestWebSocketOriginSameOriginOnly(t *testing.T) {
	config.InitializeEmpty()
	for _, test := range []struct {
		name, target, origin string
		tls                  bool
		want                 bool
	}{
		{"http same origin", "http://example.com/graphql", "http://example.com", false, true},
		{"https same origin", "https://example.com/graphql", "https://example.com", true, true},
		{"missing origin", "http://example.com/graphql", "", false, false},
		{"wrong scheme", "http://example.com/graphql", "https://example.com", false, false},
		{"wrong host", "http://example.com/graphql", "http://evil.example", false, false},
		{"wrong port", "http://example.com:9999/graphql", "http://example.com:9998", false, false},
		{"null origin", "http://example.com/graphql", "null", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, test.target, nil)
			if test.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if test.origin != "" {
				r.Header.Set("Origin", test.origin)
			}
			if got := checkWebSocketOrigin(r); got != test.want {
				t.Fatalf("checkWebSocketOrigin=%v want=%v", got, test.want)
			}
		})
	}
}

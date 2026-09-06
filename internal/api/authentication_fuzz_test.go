package api

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func FuzzAuthenticationCredentialClassificationFailsClosed(f *testing.F) {
	for _, seed := range []struct {
		authorization string
		apiHeader     string
		apiQuery      string
	}{
		{"", "", ""},
		{"Bearer token-id.token-secret", "", ""},
		{"Bearer token-id.token.secret", "", ""},
		{"Bearer .secret", "", ""},
		{"Bearer id.", "", ""},
		{"Basic id.secret", "", ""},
		{"Bearer " + strings.Repeat("x", 64<<10), "", ""},
		{"", "forged", ""},
		{"", "", "forged"},
		{"\x00\xffrandom", "plugin_request=true", "plugin_request=true"},
	} {
		f.Add(seed.authorization, seed.apiHeader, seed.apiQuery)
	}

	f.Fuzz(func(t *testing.T, authorization, apiHeader, apiQuery string) {
		request := httptest.NewRequest("POST", "/graphql", nil)
		request.Header.Set("Authorization", authorization)
		request.Header.Set("ApiKey", apiHeader)
		query := request.URL.Query()
		query.Set("apikey", apiQuery)
		request.URL.RawQuery = query.Encode()

		id, secret, ok := readDBBearerToken(request)
		if ok {
			if !strings.HasPrefix(authorization, bearerTokenPrefix) ||
				id == "" || secret == "" || strings.Contains(secret, ".") {
				t.Fatal("malformed Authorization input was accepted as a DB bearer token")
			}
		}
		if isLegacyCookieAuthentication(request, "42", nil) &&
			(apiHeader != "" || apiQuery != "") {
			t.Fatal("API-key input was elevated to legacy cookie authentication")
		}
		if isLegacyCookieAuthentication(request, "", errors.New("invalid")) {
			t.Fatal("failed or empty identity was elevated to legacy authentication")
		}
	})
}

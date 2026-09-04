package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/authz"
)

func TestWriteAuthzHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		status     int
		body       string
		wantHeader bool
	}{
		{"unauthenticated", authz.UnauthenticatedError{}, http.StatusUnauthorized, "authentication required", true},
		{"capability", authz.DeniedError{Capability: authz.DatabaseSQL}, http.StatusForbidden, "forbidden", false},
		{"ownership", authz.OwnershipError{}, http.StatusForbidden, "forbidden", false},
		{"missing policy", authz.UnregisteredSurfaceError{Kind: authz.SurfaceHTTPRoute, Name: "GET /new"}, http.StatusInternalServerError, "request denied", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if !writeAuthzHTTPError(recorder, test.err) {
				t.Fatal("authorization error was not handled")
			}
			if recorder.Code != test.status || strings.TrimSpace(recorder.Body.String()) != test.body {
				t.Fatalf("response=%d %q want %d %q", recorder.Code, recorder.Body.String(), test.status, test.body)
			}
			if got := recorder.Header().Get("WWW-Authenticate"); (got != "") != test.wantHeader {
				t.Fatalf("WWW-Authenticate=%q wantHeader=%v", got, test.wantHeader)
			}
			if strings.Contains(recorder.Body.String(), "database.sql") || strings.Contains(recorder.Body.String(), "GET /new") {
				t.Fatal("response leaked internal authorization detail")
			}
		})
	}
}

func TestWriteAuthzHTTPErrorRejectsOrdinaryError(t *testing.T) {
	recorder := httptest.NewRecorder()
	if writeAuthzHTTPError(recorder, errors.New("database offline")) {
		t.Fatal("ordinary error was mislabeled as authorization error")
	}
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("writer mutated for ordinary error: %#v", recorder)
	}
}

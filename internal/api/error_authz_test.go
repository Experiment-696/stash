package api

import (
	"context"
	"testing"

	"github.com/stashapp/stash/internal/authz"
)

func TestGQLErrorHandlerPresentsAuthzCodes(t *testing.T) {
	tests := []struct {
		err     error
		message string
		code    string
	}{
		{authz.UnauthenticatedError{}, "authentication required", authz.CodeUnauthenticated},
		{authz.DeniedError{Capability: authz.MetadataWrite}, "forbidden", authz.CodeForbidden},
		{authz.OwnershipError{}, "forbidden", authz.CodeForbidden},
	}
	for _, test := range tests {
		presented := gqlErrorHandler(context.Background(), test.err)
		if presented.Message != test.message {
			t.Errorf("message=%q want %q", presented.Message, test.message)
		}
		if got := presented.Extensions["code"]; got != test.code {
			t.Errorf("code=%v want %s", got, test.code)
		}
	}
}

func TestGQLErrorHandlerDoesNotRewriteOrdinaryErrors(t *testing.T) {
	presented := gqlErrorHandler(context.Background(), context.DeadlineExceeded)
	if presented.Message == "forbidden" || presented.Message == "authentication required" {
		t.Fatalf("ordinary error rewritten as authorization error: %#v", presented)
	}
}

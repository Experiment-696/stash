package api

import (
	"errors"
	"net/http"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/logger"
)

// writeAuthzHTTPError emits the stable public HTTP contract for route guards.
// It returns false for non-authorization errors so callers cannot accidentally
// relabel unrelated failures as authentication or permission failures.
func writeAuthzHTTPError(w http.ResponseWriter, err error) bool {
	var clientError authz.ClientError
	if !errors.As(err, &clientError) {
		return false
	}
	if clientError.HTTPStatus() >= http.StatusInternalServerError {
		logger.Errorf("authorization policy failure: %v", err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if clientError.HTTPStatus() == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "FormBased")
	}
	http.Error(w, clientError.PublicMessage(), clientError.HTTPStatus())
	return true
}

package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

type downloadsRoutes struct{}

func (rs downloadsRoutes) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/{downloadHash}", func(r chi.Router) {
		r.Use(downloadCtx)
		r.Get("/{filename}", rs.file)
	})

	return r
}

func (rs downloadsRoutes) file(w http.ResponseWriter, r *http.Request) {
	hash := r.Context().Value(downloadKey).(string)
	if hash == "" {
		http.NotFound(w, r)
		return
	}

	principal, err := authz.PrincipalFromContext(r.Context())
	mgr := manager.GetInstance()
	if err != nil || !persistedDownloadPrincipal(r.Context(), mgr.Database, principal) {
		http.NotFound(w, r)
		return
	}
	mgr.DownloadStore.Serve(hash, principal, w, r)
}

func persistedDownloadPrincipal(ctx context.Context, database *sqlite.Database, principal authz.Principal) bool {
	id, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || id <= 0 {
		return false
	}
	valid := false
	if err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		user, findErr := database.User.Find(txCtx, id)
		if findErr != nil || user == nil {
			return findErr
		}
		valid = user.Status == authz.StatusActive && user.Role == principal.Role
		return nil
	}); err != nil {
		return false
	}
	return valid
}

func downloadCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadHash := chi.URLParam(r, "downloadHash")

		ctx := context.WithValue(r.Context(), downloadKey, downloadHash)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

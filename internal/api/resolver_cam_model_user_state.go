package api

import (
	"context"
	"errors"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func requirePersistedCamModelPreference(ctx, txCtx context.Context, database *sqlite.Database) (int64, error) {
	principal, err := authz.RequireContext(ctx, authz.PreferenceSelfWrite)
	if err != nil {
		return 0, err
	}
	id, err := persistedPrincipalUserID(principal)
	if err != nil {
		return 0, err
	}
	user, err := database.User.Find(txCtx, id)
	if err != nil || user == nil {
		return 0, authz.UnauthenticatedError{}
	}
	persisted := authz.Principal{UserID: principal.UserID, Role: user.Role, Status: user.Status, TokenScopes: principal.TokenScopes}
	if !persisted.Allows(authz.PreferenceSelfWrite) {
		return 0, authz.DeniedError{Capability: authz.PreferenceSelfWrite}
	}
	return id, nil
}

func (r *mutationResolver) CamModelSetUserState(ctx context.Context, id string, favorite bool, rating100 *int) (*CamModelProfile, error) {
	modelID, err := parseCamID(id)
	if err != nil {
		return nil, err
	}
	database := r.tokenDatabase()
	var profile *sqlite.CamModelProfile
	err = txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		userID, authErr := requirePersistedCamModelPreference(ctx, txCtx, database)
		if authErr != nil {
			return authErr
		}
		existing, findErr := database.CamShow.FindModelProfile(txCtx, modelID)
		if findErr != nil {
			return findErr
		}
		if existing == nil {
			return errors.New("cam model profile not found")
		}
		if setErr := database.CamShow.SetUserState(txCtx, userID, modelID, favorite, rating100); setErr != nil {
			return setErr
		}
		profile, findErr = database.CamShow.FindModelProfileForUser(txCtx, modelID, userID)
		return findErr
	})
	if err != nil {
		return nil, personalDataError("set Cam Model user state", err)
	}
	return camProfileModel(profile, false), nil
}

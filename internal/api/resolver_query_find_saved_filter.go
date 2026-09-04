package api

import (
	"context"
	"strconv"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
)

func (r *queryResolver) FindSavedFilter(ctx context.Context, id string) (ret *models.SavedFilter, err error) {
	principal, err := authz.RequireContext(ctx, authz.AccountSelfRead)
	if err != nil {
		return nil, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return nil, err
	}
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = r.tokenDatabase().SavedFilter.FindForUser(ctx, idInt, userID)
		return err
	}); err != nil {
		return nil, err
	}
	return ret, err
}

func (r *queryResolver) FindSavedFilters(ctx context.Context, mode *models.FilterMode) (ret []*models.SavedFilter, err error) {
	principal, err := authz.RequireContext(ctx, authz.AccountSelfRead)
	if err != nil {
		return nil, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return nil, err
	}
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		if mode != nil {
			ret, err = r.tokenDatabase().SavedFilter.FindByModeForUser(ctx, *mode, userID)
		} else {
			ret, err = r.tokenDatabase().SavedFilter.AllForUser(ctx, userID)
		}
		return err
	}); err != nil {
		return nil, err
	}
	return ret, err
}

func (r *queryResolver) FindDefaultFilter(ctx context.Context, mode models.FilterMode) (ret *models.SavedFilter, err error) {
	principal, err := authz.RequireContext(ctx, authz.AccountSelfRead)
	if err != nil {
		return nil, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return nil, err
	}
	if err := r.withReadTxn(ctx, func(txCtx context.Context) error {
		ret, err = r.tokenDatabase().SavedFilter.FindDefaultForUser(txCtx, userID, mode)
		return err
	}); err != nil {
		return nil, err
	}
	return ret, nil
}

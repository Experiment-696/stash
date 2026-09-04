package api

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
)

func (r *mutationResolver) SaveFilter(ctx context.Context, input SaveFilterInput) (ret *models.SavedFilter, err error) {
	principal, err := authz.RequireContext(ctx, authz.PreferenceSelfWrite)
	if err != nil {
		return nil, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("name must be non-empty")
	}

	var id *int
	if input.ID != nil {
		idv, err := strconv.Atoi(*input.ID)
		if err != nil {
			return nil, fmt.Errorf("converting id: %w", err)
		}
		id = &idv
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		qb := r.repository.SavedFilter

		f := models.SavedFilter{
			UserID:       &userID,
			Mode:         input.Mode,
			Name:         strings.TrimSpace(input.Name),
			FindFilter:   input.FindFilter,
			ObjectFilter: input.ObjectFilter,
			UIOptions:    input.UIOptions,
		}

		if id == nil {
			err = qb.Create(ctx, &f)
			ret = &f
		} else {
			existing, findErr := r.tokenDatabase().SavedFilter.FindForUser(ctx, *id, userID)
			if findErr != nil {
				return findErr
			}
			if existing == nil {
				return authz.OwnershipError{}
			}
			f.ID = *id
			err = r.tokenDatabase().SavedFilter.Update(ctx, &f)
			ret = &f
		}

		return err
	}); err != nil {
		return nil, err
	}
	return ret, err
}

func (r *mutationResolver) DestroySavedFilter(ctx context.Context, input DestroyFilterInput) (bool, error) {
	principal, err := authz.RequireContext(ctx, authz.PreferenceSelfWrite)
	if err != nil {
		return false, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return false, err
	}
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return false, fmt.Errorf("converting id: %w", err)
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		return r.tokenDatabase().SavedFilter.DestroyForUser(ctx, id, userID)
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) SetDefaultFilter(ctx context.Context, input SetDefaultFilterInput) (bool, error) {
	principal, err := authz.RequireContext(ctx, authz.PreferenceSelfWrite)
	if err != nil {
		return false, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return false, err
	}

	if input.FindFilter == nil && input.ObjectFilter == nil && input.UIOptions == nil {
		err = r.withTxn(ctx, func(txCtx context.Context) error {
			return r.tokenDatabase().SavedFilter.SetDefaultForUser(txCtx, userID, input.Mode, nil)
		})
		return err == nil, err
	}
	err = r.withTxn(ctx, func(txCtx context.Context) error {
		filters, findErr := r.tokenDatabase().SavedFilter.FindByModeForUser(txCtx, input.Mode, userID)
		if findErr != nil {
			return findErr
		}
		matches := make([]int, 0, 1)
		for _, filter := range filters {
			if reflect.DeepEqual(filter.FindFilter, input.FindFilter) &&
				reflect.DeepEqual(filter.ObjectFilter, input.ObjectFilter) &&
				reflect.DeepEqual(filter.UIOptions, input.UIOptions) {
				matches = append(matches, filter.ID)
			}
		}
		if len(matches) != 1 {
			return authz.OwnershipError{}
		}
		return r.tokenDatabase().SavedFilter.SetDefaultForUser(txCtx, userID, input.Mode, &matches[0])
	})
	return err == nil, err
}

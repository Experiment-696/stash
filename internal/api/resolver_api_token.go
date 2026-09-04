package api

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func apiTokenModel(token sqlite.APIToken) *APIToken {
	return &APIToken{
		ID: token.ID, Name: token.Name, CreatedAt: token.CreatedAt, ExpiresAt: token.ExpiresAt,
		LastUsedAt: token.LastUsedAt, RevokedAt: token.RevokedAt,
	}
}

func persistedPrincipalUserID(principal authz.Principal) (int64, error) {
	userID, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || userID <= 0 {
		return 0, authz.UnauthenticatedError{}
	}
	return userID, nil
}

func (r *queryResolver) MyAPITokens(ctx context.Context) ([]*APIToken, error) {
	principal, err := authz.RequireContext(ctx, authz.AccountSelfRead)
	if err != nil {
		return nil, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return nil, err
	}
	var records []sqlite.APIToken
	database := r.tokenDatabase()
	err = txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		var listErr error
		records, listErr = database.APIToken.ListForUser(txCtx, userID)
		return listErr
	})
	if err != nil {
		logger.Errorf("listing API token metadata failed: %v", err)
		return nil, errors.New("unable to list API tokens")
	}
	result := make([]*APIToken, len(records))
	for i, record := range records {
		result[i] = apiTokenModel(record)
	}
	return result, nil
}

func (r *mutationResolver) CreateMyAPIToken(ctx context.Context, input APITokenCreateInput) (*APITokenCreateResult, error) {
	principal, err := authz.RequireContext(ctx, authz.AccountSelfWrite)
	if err != nil {
		return nil, err
	}
	scopes := make([]authz.Capability, len(input.Scopes))
	for i, value := range input.Scopes {
		scopes[i] = authz.Capability(value)
		if !authz.IsKnownCapability(scopes[i]) {
			return nil, errors.New("invalid token scope")
		}
	}
	lifetime := time.Duration(0)
	if input.LifetimeDays != nil {
		lifetime = time.Duration(*input.LifetimeDays) * 24 * time.Hour
	}
	var credentials *sqlite.APITokenCredentials
	var record *sqlite.APIToken
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return nil, err
	}
	database := r.tokenDatabase()
	err = txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		var createErr error
		credentials, createErr = database.APIToken.Create(txCtx, principal, input.Name, scopes, lifetime)
		if createErr != nil {
			return createErr
		}
		record, createErr = database.APIToken.GetForUser(txCtx, userID, credentials.ID)
		return createErr
	})
	if err != nil {
		logger.Errorf("creating API token failed: %v", err)
		return nil, errors.New("unable to create API token")
	}
	return &APITokenCreateResult{Token: apiTokenModel(*record), Secret: credentials.ID + "." + credentials.Secret}, nil
}

func (r *mutationResolver) RevokeMyAPIToken(ctx context.Context, id string) (bool, error) {
	principal, err := authz.RequireContext(ctx, authz.AccountSelfWrite)
	if err != nil {
		return false, err
	}
	userID, err := persistedPrincipalUserID(principal)
	if err != nil {
		return false, err
	}
	database := r.tokenDatabase()
	err = txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		return database.APIToken.Revoke(txCtx, userID, id)
	})
	if err != nil {
		logger.Errorf("revoking API token failed: %v", err)
		return false, errors.New("unable to revoke API token")
	}
	return true, nil
}

package api

import (
	"context"
	"errors"
	"strconv"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func userAccountModel(user sqlite.User) *UserAccount {
	principal := authz.Principal{UserID: strconv.FormatInt(user.ID, 10), Role: user.Role, Status: user.Status}
	return &UserAccount{ID: principal.UserID, Username: user.Username, Role: string(user.Role), Status: string(user.Status), Capabilities: principalCapabilities(principal), CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
}

func principalCapabilities(principal authz.Principal) []string {
	ret := make([]string, 0)
	for _, capability := range principal.EffectiveCapabilities() {
		ret = append(ret, string(capability))
	}
	return ret
}

func (r *mutationResolver) BootstrapFirstAdmin(ctx context.Context, input FirstAdminBootstrapInput) (*UserAccount, error) {
	if !session.IsLocalRequest(ctx) && !session.IsBootstrapRequest(ctx) {
		return nil, errors.New("unable to bootstrap account")
	}
	database := r.tokenDatabase()
	var user *sqlite.User
	err := txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		var createErr error
		user, createErr = database.User.BootstrapAdmin(txCtx, input.Username, input.Password)
		return createErr
	})
	if err != nil {
		logger.Errorf("first Admin bootstrap failed: %v", err)
		return nil, errors.New("unable to bootstrap account")
	}
	// A zero-user legacy instance may already have DLNA listening. Stop it
	// before reporting successful activation of multiuser mode.
	manager.RefreshDLNAForAccountTransition()
	session.ConsumeBootstrapToken()
	return userAccountModel(*user), nil
}

func requirePersistedAdmin(ctx, txCtx context.Context, database *sqlite.Database) error {
	principal, err := authz.RequireContext(ctx, authz.AccountManage)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || id <= 0 {
		return authz.UnauthenticatedError{}
	}
	user, err := database.User.Find(txCtx, id)
	if err != nil || user == nil || user.Role != authz.RoleAdmin || user.Status != authz.StatusActive {
		return authz.UnauthenticatedError{}
	}
	return nil
}

func parseUserID(id string) (int64, error) {
	value, err := strconv.ParseInt(id, 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("invalid user id")
	}
	return value, nil
}

func recordAdminUserAudit(ctx, txCtx context.Context, database *sqlite.Database, eventType string, targetUserID int64) error {
	principal, err := authz.RequireContext(ctx, authz.AccountManage)
	if err != nil {
		return err
	}
	actorID, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || actorID <= 0 {
		return authz.UnauthenticatedError{}
	}
	return database.Audit.Record(txCtx, actorID, eventType, "user", strconv.FormatInt(targetUserID, 10), "success")
}

func recordLastAdminDenial(ctx context.Context, database *sqlite.Database, eventType string, targetUserID int64) {
	principal, err := authz.RequireContext(ctx, authz.AccountManage)
	if err != nil {
		return
	}
	actorID, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || actorID <= 0 {
		return
	}
	if err := txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		return database.Audit.Record(txCtx, actorID, eventType, "user", strconv.FormatInt(targetUserID, 10), "denied_last_admin")
	}); err != nil {
		logger.Errorf("recording last-Admin denial failed: %v", err)
	}
}

func adminMutationError(operation string, err error) error {
	var clientError authz.ClientError
	if errors.As(err, &clientError) {
		return clientError
	}
	logger.Errorf("admin user operation %s failed: %v", operation, err)
	return errors.New("unable to update user")
}

func (r *queryResolver) Users(ctx context.Context) ([]*UserAccount, error) {
	database := r.tokenDatabase()
	var users []sqlite.User
	err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		if err := requirePersistedAdmin(ctx, txCtx, database); err != nil {
			return err
		}
		var err error
		users, err = database.User.List(txCtx)
		return err
	})
	if err != nil {
		return nil, err
	}
	result := make([]*UserAccount, len(users))
	for i, user := range users {
		result[i] = userAccountModel(user)
	}
	return result, nil
}

func (r *queryResolver) Me(ctx context.Context) (*UserAccount, error) {
	principal, err := authz.RequireContext(ctx, authz.AccountSelfRead)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || id <= 0 {
		return nil, authz.UnauthenticatedError{}
	}
	database := r.tokenDatabase()
	var user *sqlite.User
	err = txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		var findErr error
		user, findErr = database.User.Find(txCtx, id)
		if findErr == nil && user == nil {
			return authz.UnauthenticatedError{}
		}
		return findErr
	})
	if err != nil {
		return nil, err
	}
	result := userAccountModel(*user)
	result.Capabilities = principalCapabilities(principal)
	return result, nil
}

func (r *mutationResolver) CreateUser(ctx context.Context, input UserCreateInput) (*UserAccount, error) {
	database := r.tokenDatabase()
	var user *sqlite.User
	err := txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		if err := requirePersistedAdmin(ctx, txCtx, database); err != nil {
			return err
		}
		var err error
		user, err = database.User.Create(txCtx, input.Username, input.Password, authz.Role(input.Role))
		if err != nil {
			return err
		}
		return recordAdminUserAudit(ctx, txCtx, database, "user_created", user.ID)
	})
	if err != nil {
		return nil, adminMutationError("create", err)
	}
	manager.RefreshDLNAForAccountTransition()
	return userAccountModel(*user), nil
}

func (r *mutationResolver) UpdateUserAccess(ctx context.Context, input UserAccessInput) (*UserAccount, error) {
	database := r.tokenDatabase()
	id, err := parseUserID(input.ID)
	if err != nil {
		return nil, errors.New("unable to update user")
	}
	var user *sqlite.User
	err = txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		if err := requirePersistedAdmin(ctx, txCtx, database); err != nil {
			return err
		}
		if err := database.User.SetAccess(txCtx, id, authz.Role(input.Role), authz.AccountStatus(input.Status)); err != nil {
			return err
		}
		user, err = database.User.Find(txCtx, id)
		if err != nil {
			return err
		}
		return recordAdminUserAudit(ctx, txCtx, database, "user_access_updated", id)
	})
	if err != nil {
		if errors.Is(err, sqlite.ErrLastActiveAdmin) {
			recordLastAdminDenial(ctx, database, "user_access_update_denied", id)
		}
		return nil, adminMutationError("update access", err)
	}
	return userAccountModel(*user), nil
}

func (r *mutationResolver) ResetUserPassword(ctx context.Context, id string, password string) (bool, error) {
	return r.adminUserAction(ctx, id, "reset password", func(txCtx context.Context, database *sqlite.Database, userID int64) error {
		if err := database.User.ResetPassword(txCtx, userID, password); err != nil {
			return err
		}
		if err := database.Session.RevokeAllForUser(txCtx, userID); err != nil {
			return err
		}
		return database.APIToken.RevokeAllForUser(txCtx, userID)
	})
}

func (r *mutationResolver) RevokeUserSessions(ctx context.Context, id string) (bool, error) {
	return r.adminUserAction(ctx, id, "revoke sessions", func(txCtx context.Context, database *sqlite.Database, userID int64) error {
		return database.Session.RevokeAllForUser(txCtx, userID)
	})
}

func (r *mutationResolver) RevokeUserAPITokens(ctx context.Context, id string) (bool, error) {
	return r.adminUserAction(ctx, id, "revoke API tokens", func(txCtx context.Context, database *sqlite.Database, userID int64) error {
		return database.APIToken.RevokeAllForUser(txCtx, userID)
	})
}

func (r *mutationResolver) adminUserAction(ctx context.Context, id, operation string, action func(context.Context, *sqlite.Database, int64) error) (bool, error) {
	database := r.tokenDatabase()
	userID, err := parseUserID(id)
	if err == nil {
		err = txn.WithTxn(ctx, database, func(txCtx context.Context) error {
			if guardErr := requirePersistedAdmin(ctx, txCtx, database); guardErr != nil {
				return guardErr
			}
			target, findErr := database.User.Find(txCtx, userID)
			if findErr != nil {
				return findErr
			}
			if target == nil {
				return errors.New("user not found")
			}
			if err := action(txCtx, database, userID); err != nil {
				return err
			}
			eventType := map[string]string{
				"reset password":    "user_password_reset",
				"revoke sessions":   "user_sessions_revoked",
				"revoke API tokens": "user_api_tokens_revoked",
			}[operation]
			return recordAdminUserAudit(ctx, txCtx, database, eventType, userID)
		})
	}
	if err != nil {
		if errors.Is(err, sqlite.ErrLastActiveAdmin) {
			recordLastAdminDenial(ctx, database, "user_action_denied", userID)
		}
		return false, adminMutationError(operation, err)
	}
	return true, nil
}

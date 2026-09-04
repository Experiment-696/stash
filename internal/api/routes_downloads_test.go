package api

import (
	"context"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/txn"
)

func TestPersistedDownloadPrincipalFailsClosedAfterAccessChange(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "download-admin", authz.RoleAdmin)
	_ = createResolverUser(t, database, "remaining-download-admin", authz.RoleAdmin)
	if !persistedDownloadPrincipal(context.Background(), database, admin) {
		t.Fatal("active persisted Admin was rejected")
	}

	admin.Role = authz.RoleModerator
	if persistedDownloadPrincipal(context.Background(), database, admin) {
		t.Fatal("principal with role differing from persisted account was accepted")
	}

	admin.Role = authz.RoleAdmin
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		id, err := persistedPrincipalUserID(admin)
		if err != nil {
			return err
		}
		return database.User.SetAccess(ctx, id, authz.RoleAdmin, authz.StatusDisabled)
	}); err != nil {
		t.Fatal(err)
	}
	if persistedDownloadPrincipal(context.Background(), database, admin) {
		t.Fatal("disabled persisted Admin was accepted")
	}

	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		id, err := persistedPrincipalUserID(admin)
		if err != nil {
			return err
		}
		_, _, err = database.ExecSQL(ctx, "DELETE FROM users WHERE id = ?", []interface{}{id})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if persistedDownloadPrincipal(context.Background(), database, admin) {
		t.Fatal("deleted persisted Admin was accepted")
	}
}

package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/txn"
)

func TestPluginCallbackRequiresSignedMarkerAndLocalOrigin(t *testing.T) {
	config.InitializeEmpty()
	store := session.NewStore(config.GetInstance())
	cookie := store.MakePluginCookie(session.SetCurrentUserID(context.Background(), "1"))
	if cookie == nil {
		t.Fatal("MakePluginCookie returned nil")
	}

	remote := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	remote.RemoteAddr = "203.0.113.10:1234"
	remote.AddCookie(cookie)
	remote = session.SetLocalRequest(remote)
	if isLocalPluginCallback(store, remote) {
		t.Fatal("remote marked plugin callback was accepted")
	}

	local := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	local.RemoteAddr = "127.0.0.1:1234"
	local.AddCookie(cookie)
	local = session.SetLocalRequest(local)
	if !isLocalPluginCallback(store, local) {
		t.Fatal("local signed plugin callback was rejected")
	}

	local.Header.Set("ApiKey", "plugin_request=true")
	local.URL.RawQuery = "plugin_request=true"
	unmarked := httptest.NewRequest(http.MethodPost, local.URL.String(), nil)
	unmarked.RemoteAddr = local.RemoteAddr
	unmarked.Header.Set("ApiKey", "plugin_request=true")
	unmarked = session.SetLocalRequest(unmarked)
	if isLocalPluginCallback(store, unmarked) {
		t.Fatal("header/query marker minted plugin callback authority")
	}
}

func TestPluginPrincipalRevalidatesPersistedAccountOnEveryCallback(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "plugin-callback-admin", authz.RoleAdmin)
	_ = createResolverUser(t, database, "remaining-plugin-admin", authz.RoleAdmin)

	resolved, err := resolvePluginPrincipal(context.Background(), database, admin.UserID)
	if err != nil || resolved.Role != authz.RoleAdmin || !resolved.Allows(authz.ExtensionManage) {
		t.Fatalf("active plugin principal = %+v err=%v", resolved, err)
	}
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		id, err := persistedPrincipalUserID(admin)
		if err != nil {
			return err
		}
		return database.User.SetAccess(ctx, id, authz.RoleUser, authz.StatusActive)
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err = resolvePluginPrincipal(context.Background(), database, admin.UserID)
	if err != nil || resolved.Role != authz.RoleUser || resolved.Allows(authz.ExtensionManage) {
		t.Fatalf("demoted plugin principal = %+v err=%v", resolved, err)
	}
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		id, err := persistedPrincipalUserID(admin)
		if err != nil {
			return err
		}
		return database.User.SetAccess(ctx, id, authz.RoleUser, authz.StatusDisabled)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolvePluginPrincipal(context.Background(), database, admin.UserID); !errors.Is(err, session.ErrUnauthorized) {
		t.Fatalf("disabled plugin callback error = %v, want unauthorized", err)
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
	if _, err := resolvePluginPrincipal(context.Background(), database, admin.UserID); !errors.Is(err, session.ErrUnauthorized) {
		t.Fatalf("deleted plugin callback error = %v, want unauthorized", err)
	}
}

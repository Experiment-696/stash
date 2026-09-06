package sqlite

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authn"
	"github.com/stashapp/stash/internal/authz"
)

func TestAPITokenScopesSecrecyAndRevocation(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	user, err := (&UserStore{}).Create(ctx, "admin", "password", authz.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	principal := authz.Principal{UserID: strconv.FormatInt(user.ID, 10), Role: user.Role, Status: user.Status}
	store := &APITokenStore{}
	credentials, err := store.Create(ctx, principal, "reader", []authz.Capability{authz.LibraryRead}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var storedHash string
	if err := dbWrapper.Get(ctx, &storedHash, `SELECT secret_hash FROM user_api_tokens WHERE id = ?`, credentials.ID); err != nil {
		t.Fatal(err)
	}
	if storedHash == credentials.Secret || storedHash != authn.HashOpaqueSecret(credentials.Secret) {
		t.Fatal("API token plaintext was stored or digest was incorrect")
	}
	tokenPrincipal, err := store.Authenticate(ctx, credentials.ID, credentials.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !tokenPrincipal.Allows(authz.LibraryRead) || tokenPrincipal.Allows(authz.DatabaseSQL) {
		t.Fatalf("token scopes not reducing-only: %+v", tokenPrincipal.TokenScopes)
	}
	if _, err := store.Authenticate(ctx, credentials.ID, "wrong", time.Now()); err == nil {
		t.Fatal("wrong API token secret authenticated")
	}
	if err := store.Revoke(ctx, 999, credentials.ID); err == nil {
		t.Fatal("cross-user token revocation succeeded")
	}
	if err := store.Revoke(ctx, user.ID, credentials.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(ctx, user.ID, credentials.ID); err != nil {
		t.Fatalf("owner retry was not idempotent: %v", err)
	}
	if _, err := store.Authenticate(ctx, credentials.ID, credentials.Secret, time.Now()); err == nil {
		t.Fatal("revoked API token authenticated")
	}
}

func TestAPITokenOwnerMetadataAndExpiry(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	users := &UserStore{}
	owner, err := users.Create(ctx, "token-owner", "password", authz.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.Create(ctx, "token-other", "password", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	principal := authz.Principal{UserID: strconv.FormatInt(owner.ID, 10), Role: owner.Role, Status: owner.Status}
	store := &APITokenStore{}
	credentials, err := store.Create(ctx, principal, "one-time", []authz.Capability{authz.LibraryRead}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := store.ListForUser(ctx, owner.ID)
	if err != nil || len(owned) != 1 || owned[0].ID != credentials.ID {
		t.Fatalf("owner metadata list=%+v err=%v", owned, err)
	}
	if strings.Contains(strings.Join([]string{owned[0].ID, owned[0].Name}, " "), credentials.Secret) {
		t.Fatal("one-time secret appeared in token metadata")
	}
	others, err := store.ListForUser(ctx, other.ID)
	if err != nil || len(others) != 0 {
		t.Fatalf("other user saw owner tokens: %+v err=%v", others, err)
	}
	if _, err := store.GetForUser(ctx, other.ID, credentials.ID); err == nil {
		t.Fatal("cross-user metadata lookup succeeded")
	}
	if _, err := dbWrapper.Exec(ctx, `UPDATE user_api_tokens SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Minute), credentials.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, credentials.ID, credentials.Secret, time.Now()); err == nil {
		t.Fatal("expired bearer token authenticated")
	}
	if _, err := store.RequireActive(ctx, owner.ID, credentials.ID, time.Now()); err == nil {
		t.Fatal("expired bearer token remained active")
	}
}

func TestAPITokenRequireActiveDetectsRoleScopeReduction(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	user, err := (&UserStore{}).Create(ctx, "role-reduction", "password", authz.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	principal := authz.Principal{UserID: strconv.FormatInt(user.ID, 10), Role: user.Role, Status: user.Status}
	store := &APITokenStore{}
	credentials, err := store.Create(ctx, principal, "admin-scope", []authz.Capability{authz.DatabaseSQL}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequireActive(ctx, user.ID, credentials.ID, time.Now()); err != nil {
		t.Fatalf("active scoped token rejected: %v", err)
	}
	if _, err := dbWrapper.Exec(ctx, `UPDATE users SET role = ? WHERE id = ?`, authz.RoleUser, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequireActive(ctx, user.ID, credentials.ID, time.Now()); err == nil || strings.Contains(err.Error(), credentials.Secret) {
		t.Fatalf("role-reduced token error was absent or secret-bearing: %v", err)
	}
}

func TestAPITokenBoundsAndScopeEscalation(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	user, err := (&UserStore{}).Create(ctx, "moderator", "password", authz.RoleModerator)
	if err != nil {
		t.Fatal(err)
	}
	principal := authz.Principal{UserID: strconv.FormatInt(user.ID, 10), Role: user.Role, Status: user.Status}
	store := &APITokenStore{}
	if _, err := store.Create(ctx, principal, "too long", nil, MaximumAPITokenLifetime+time.Second); err == nil {
		t.Fatal("token exceeded maximum lifetime")
	}
	if _, err := store.Create(ctx, principal, "escalated", []authz.Capability{authz.DatabaseSQL}, time.Hour); err == nil {
		t.Fatal("Moderator minted Admin-only token scope")
	}
	credentials, err := store.Create(ctx, principal, "empty", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Authenticate(ctx, credentials.ID, credentials.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Allows(authz.LibraryRead) {
		t.Fatal("empty explicit scopes retained role grants")
	}
}

package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/authn"
	"github.com/stashapp/stash/internal/authz"
)

const (
	DefaultAPITokenLifetime = 90 * 24 * time.Hour
	MaximumAPITokenLifetime = 365 * 24 * time.Hour
)

type APITokenCredentials struct {
	ID     string
	Secret string
}

type APIToken struct {
	ID         string     `db:"id"`
	UserID     int64      `db:"user_id"`
	Name       string     `db:"name"`
	CreatedAt  time.Time  `db:"created_at"`
	ExpiresAt  time.Time  `db:"expires_at"`
	LastUsedAt *time.Time `db:"last_used_at"`
	RevokedAt  *time.Time `db:"revoked_at"`
}

type apiTokenPrincipalRow struct {
	ID         string              `db:"id"`
	UserID     int64               `db:"user_id"`
	ScopesJSON *string             `db:"scopes_json"`
	Role       authz.Role          `db:"role"`
	Status     authz.AccountStatus `db:"status"`
}

type APITokenStore struct{}

// ListForUser returns metadata only; token digests are never selected.
func (s *APITokenStore) ListForUser(ctx context.Context, userID int64) ([]APIToken, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	var tokens []APIToken
	err := dbWrapper.Select(ctx, &tokens, `SELECT id, user_id, name, created_at, expires_at, last_used_at, revoked_at
		FROM user_api_tokens WHERE user_id = ? ORDER BY created_at DESC, id`, userID)
	return tokens, err
}

func (s *APITokenStore) GetForUser(ctx context.Context, userID int64, id string) (*APIToken, error) {
	var token APIToken
	err := dbWrapper.Get(ctx, &token, `SELECT id, user_id, name, created_at, expires_at, last_used_at, revoked_at
		FROM user_api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *APITokenStore) Create(ctx context.Context, principal authz.Principal, name string, scopes []authz.Capability, lifetime time.Duration) (*APITokenCredentials, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("token name is required")
	}
	if !principal.IsAuthenticated() {
		return nil, authz.UnauthenticatedError{}
	}
	userID, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || userID <= 0 {
		return nil, errors.New("principal has invalid persisted user id")
	}
	if lifetime == 0 {
		lifetime = DefaultAPITokenLifetime
	}
	if lifetime <= 0 || lifetime > MaximumAPITokenLifetime {
		return nil, errors.New("token lifetime must be positive and no more than one year")
	}
	for _, capability := range scopes {
		if !principal.Allows(capability) {
			return nil, fmt.Errorf("token scope exceeds principal grants: %s", capability)
		}
	}
	encodedScopes, err := json.Marshal(scopes)
	if err != nil {
		return nil, err
	}
	id, err := authn.NewOpaqueSecret(32)
	if err != nil {
		return nil, err
	}
	secret, err := authn.NewOpaqueSecret(32)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, err = dbWrapper.Exec(ctx, `INSERT INTO user_api_tokens
		(id, user_id, name, secret_hash, scopes_json, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, userID, name, authn.HashOpaqueSecret(secret), string(encodedScopes), now, now.Add(lifetime))
	if err != nil {
		return nil, fmt.Errorf("creating API token: %w", err)
	}
	return &APITokenCredentials{ID: id, Secret: secret}, nil
}

func (s *APITokenStore) Authenticate(ctx context.Context, id, secret string, now time.Time) (authz.Principal, error) {
	var row apiTokenPrincipalRow
	err := dbWrapper.Get(ctx, &row, `SELECT t.id, t.user_id, t.scopes_json, u.role, u.status
		FROM user_api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.id = ? AND t.secret_hash = ? AND t.revoked_at IS NULL
		AND t.expires_at > ? AND u.status = 'ACTIVE'`, id, authn.HashOpaqueSecret(secret), now.UTC())
	if err != nil {
		return authz.Principal{}, err
	}
	principal := authz.Principal{UserID: fmt.Sprint(row.UserID), Role: row.Role, Status: row.Status}
	if row.ScopesJSON == nil { // Converted legacy key retains role grants during compatibility window.
		return principal, nil
	}
	var scopes []authz.Capability
	if err := json.Unmarshal([]byte(*row.ScopesJSON), &scopes); err != nil {
		return authz.Principal{}, errors.New("invalid persisted API token scopes")
	}
	for _, capability := range scopes {
		if !principal.Allows(capability) {
			return authz.Principal{}, errors.New("persisted API token scope exceeds current role grants")
		}
	}
	principal.TokenScopes = make(map[authz.Capability]struct{}, len(scopes))
	for _, capability := range scopes {
		principal.TokenScopes[capability] = struct{}{}
	}
	return principal, nil
}

// RequireActive revalidates a connection-bound token without retaining its
// bearer secret. It repeats scope validation against the user's current role.
func (s *APITokenStore) RequireActive(ctx context.Context, userID int64, id string, now time.Time) (authz.Principal, error) {
	var row apiTokenPrincipalRow
	err := dbWrapper.Get(ctx, &row, `SELECT t.id, t.user_id, t.scopes_json, u.role, u.status
		FROM user_api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.id = ? AND t.user_id = ? AND t.revoked_at IS NULL
		AND t.expires_at > ? AND u.status = 'ACTIVE'`, id, userID, now.UTC())
	if err != nil {
		return authz.Principal{}, err
	}
	principal := authz.Principal{UserID: fmt.Sprint(row.UserID), Role: row.Role, Status: row.Status}
	if row.ScopesJSON == nil {
		return principal, nil
	}
	var scopes []authz.Capability
	if err := json.Unmarshal([]byte(*row.ScopesJSON), &scopes); err != nil {
		return authz.Principal{}, errors.New("invalid persisted API token scopes")
	}
	for _, capability := range scopes {
		if !principal.Allows(capability) {
			return authz.Principal{}, errors.New("persisted API token scope exceeds current role grants")
		}
	}
	principal.TokenScopes = make(map[authz.Capability]struct{}, len(scopes))
	for _, capability := range scopes {
		principal.TokenScopes[capability] = struct{}{}
	}
	return principal, nil
}

func (s *APITokenStore) Revoke(ctx context.Context, userID int64, id string) error {
	result, err := dbWrapper.Exec(ctx, `UPDATE user_api_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ? AND user_id = ?`, time.Now().UTC(), id, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("API token not found")
	}
	return nil
}

func (s *APITokenStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return errors.New("invalid user id")
	}
	_, err := dbWrapper.Exec(ctx, `UPDATE user_api_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE user_id = ?`, time.Now().UTC(), userID)
	return err
}

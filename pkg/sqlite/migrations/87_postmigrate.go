package migrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"
)

// post87 converts the legacy configured identity into the first database Admin.
// It is deliberately idempotent so the conversion can be safely exercised and
// retried independently during upgrade rehearsal.
func post87(ctx context.Context, db *sqlx.DB) error {
	cfg := config.GetInstance()
	identity := legacyIdentity{Username: cfg.GetUsername(), PasswordHash: cfg.GetPasswordHash(), APIKey: cfg.GetAPIKey()}
	return convertLegacyIdentityWithDefaults(ctx, db, identity, cfg.GetUIConfiguration(), time.Now().UTC())
}

type legacyIdentity struct {
	Username     string
	PasswordHash string
	APIKey       string
}

func convertLegacyIdentity(ctx context.Context, db *sqlx.DB, identity legacyIdentity, now time.Time) error {
	return convertLegacyIdentityWithDefaults(ctx, db, identity, nil, now)
}

func convertLegacyIdentityWithDefaults(ctx context.Context, db *sqlx.DB, identity legacyIdentity, uiConfig map[string]interface{}, now time.Time) error {
	username := strings.TrimSpace(identity.Username)
	passwordHash := identity.PasswordHash
	legacyAPIKey := identity.APIKey
	if username == "" && passwordHash == "" && legacyAPIKey == "" {
		return nil // Fresh setup creates its first Admin through the bootstrap flow.
	}
	if username == "" {
		username = "admin"
	}
	normalized := strings.ToLower(username)
	now = now.UTC()

	m := migrator{db: db}
	return m.withTxn(ctx, func(tx *sqlx.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO users (username, normalized_username, password_hash, role, status, created_at, updated_at)
			VALUES (?, ?, NULLIF(?, ''), 'ADMIN', 'ACTIVE', ?, ?)
			ON CONFLICT(normalized_username) DO UPDATE SET
				password_hash = CASE WHEN users.password_hash IS NULL THEN excluded.password_hash ELSE users.password_hash END,
				updated_at = excluded.updated_at`, username, normalized, passwordHash, now, now)
		if err != nil {
			return fmt.Errorf("converting legacy Admin identity: %w", err)
		}

		var userID int64
		if err := tx.GetContext(ctx, &userID, `SELECT id FROM users WHERE normalized_username = ?`, normalized); err != nil {
			return fmt.Errorf("reading converted legacy Admin: %w", err)
		}
		if legacyAPIKey != "" {
			digest := sha256.Sum256([]byte(legacyAPIKey))
			hash := hex.EncodeToString(digest[:])
			id := "legacy-converted-api-key"
			expires := now.Add(90 * 24 * time.Hour)
			_, err = tx.ExecContext(ctx, `
				INSERT INTO user_api_tokens (id, user_id, name, secret_hash, scopes_json, created_at, expires_at)
				VALUES (?, ?, 'Converted legacy API key', ?, NULL, ?, ?)
				ON CONFLICT(id) DO NOTHING`, id, userID, hash, now, expires)
			if err != nil {
				return fmt.Errorf("converting legacy API key: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE saved_filters SET user_id = ? WHERE user_id IS NULL`, userID); err != nil {
			return fmt.Errorf("assigning legacy saved filters to converted Admin: %w", err)
		}
		if err := migrateLegacyDefaultFilters(ctx, tx, userID, uiConfig, now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO user_performer_state (user_id, performer_id, favorite, rating, updated_at)
			SELECT ?, id, favorite, rating, ? FROM performers WHERE favorite = 1 OR rating IS NOT NULL
			ON CONFLICT(user_id, performer_id) DO NOTHING`, userID, now); err != nil {
			return fmt.Errorf("assigning legacy performer state to converted Admin: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO user_audit_events (occurred_at, actor_user_id, event_type, target_type, target_id, result)
			SELECT ?, ?, 'legacy_identity_converted', 'user', CAST(? AS TEXT), 'success'
			WHERE NOT EXISTS (SELECT 1 FROM user_audit_events WHERE event_type = 'legacy_identity_converted')`, now, userID, userID)
		return err
	})
}

type legacyDefaultFilter struct {
	FindFilter   interface{} `json:"find_filter"`
	ObjectFilter interface{} `json:"object_filter"`
	UIOptions    interface{} `json:"ui_options"`
}

type legacySavedFilterRow struct {
	ID           int    `db:"id"`
	FindFilter   string `db:"find_filter"`
	ObjectFilter string `db:"object_filter"`
	UIOptions    string `db:"ui_options"`
}

func legacyJSONEqual(stored string, candidate interface{}) bool {
	if stored == "" {
		return candidate == nil
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(stored), &decoded); err != nil {
		return false
	}
	return reflect.DeepEqual(decoded, candidate)
}

func migrateLegacyDefaultFilters(ctx context.Context, tx *sqlx.Tx, userID int64, uiConfig map[string]interface{}, now time.Time) error {
	if uiConfig == nil {
		return nil
	}
	rawDefaults, ok := uiConfig["defaultFilters"].(map[string]interface{})
	if !ok {
		return nil
	}
	for modeKey, raw := range rawDefaults {
		encoded, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var legacy legacyDefaultFilter
		if err := json.Unmarshal(encoded, &legacy); err != nil {
			continue
		}
		mode := strings.ToUpper(modeKey)
		var candidates []legacySavedFilterRow
		if err := tx.SelectContext(ctx, &candidates, `SELECT id, find_filter, object_filter, ui_options FROM saved_filters WHERE user_id = ? AND mode = ?`, userID, mode); err != nil {
			return fmt.Errorf("reading legacy default-filter candidates: %w", err)
		}
		matches := make([]int, 0, 1)
		for _, candidate := range candidates {
			if legacyJSONEqual(candidate.FindFilter, legacy.FindFilter) && legacyJSONEqual(candidate.ObjectFilter, legacy.ObjectFilter) && legacyJSONEqual(candidate.UIOptions, legacy.UIOptions) {
				matches = append(matches, candidate.ID)
			}
		}
		if len(matches) == 1 {
			_, err = tx.ExecContext(ctx, `INSERT INTO user_preferences (user_id, key, value_json, updated_at) VALUES (?, ?, ?, ?)
				ON CONFLICT(user_id, key) DO NOTHING`, userID, "default_filter:"+mode, fmt.Sprintf("%d", matches[0]), now)
			if err != nil {
				return fmt.Errorf("migrating legacy default filter: %w", err)
			}
		}
	}
	return nil
}

func init() {
	sqlite.RegisterPostMigration(87, post87)
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	PreferenceHomepageRoute = "homepage_route"
	PreferenceThemeID       = "theme_id"
)

type PreferenceStore struct{}

func (s *PreferenceStore) Get(ctx context.Context, userID int64, key string) (*string, error) {
	var value string
	err := dbWrapper.Get(ctx, &value, `
		SELECT value_json
		FROM user_preferences
		WHERE user_id = ? AND key = ?
	`, userID, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *PreferenceStore) Set(ctx context.Context, userID int64, key, value string) error {
	_, err := dbWrapper.Exec(ctx, `
		INSERT INTO user_preferences (user_id, key, value_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, key) DO UPDATE SET
			value_json = excluded.value_json,
			updated_at = excluded.updated_at
	`, userID, key, value, time.Now().UTC())
	return err
}

func (s *PreferenceStore) Clear(ctx context.Context, userID int64, key string) error {
	_, err := dbWrapper.Exec(ctx, `
		DELETE FROM user_preferences
		WHERE user_id = ? AND key = ?
	`, userID, key)
	return err
}

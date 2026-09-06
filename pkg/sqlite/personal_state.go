package sqlite

import (
	"context"
	"errors"
	"time"
)

type PersonalStateStore struct{}

type PerformerPersonalState struct {
	Favorite      bool    `db:"favorite"`
	Rating        *int    `db:"rating"`
	RatingSum     int     `db:"rating_sum"`
	RatingCount   int     `db:"rating_count"`
	RatingAverage float64 `db:"rating_average"`
}

type ScenePersonalState struct {
	Rating        *int    `db:"rating"`
	RatingCount   int     `db:"rating_count"`
	RatingAverage float64 `db:"rating_average"`
}

func validatePersonalRating(rating *int) error {
	if rating != nil && (*rating < 1 || *rating > 100) {
		return errors.New("rating must be between 1 and 100")
	}
	return nil
}

func (s *PersonalStateStore) SetPerformerFavorite(ctx context.Context, userID, performerID int64, favorite bool) error {
	_, err := dbWrapper.Exec(ctx, `INSERT INTO user_performer_state (user_id, performer_id, favorite, rating, updated_at)
		VALUES (?, ?, ?, NULL, ?) ON CONFLICT(user_id, performer_id) DO UPDATE SET favorite = excluded.favorite, updated_at = excluded.updated_at`,
		userID, performerID, favorite, time.Now().UTC())
	return err
}

func (s *PersonalStateStore) SetPerformerRating(ctx context.Context, userID, performerID int64, rating *int) error {
	if err := validatePersonalRating(rating); err != nil {
		return err
	}
	_, err := dbWrapper.Exec(ctx, `INSERT INTO user_performer_state (user_id, performer_id, favorite, rating, updated_at)
		VALUES (?, ?, 0, ?, ?) ON CONFLICT(user_id, performer_id) DO UPDATE SET rating = excluded.rating, updated_at = excluded.updated_at`,
		userID, performerID, rating, time.Now().UTC())
	return err
}

func (s *PersonalStateStore) Performer(ctx context.Context, userID, performerID int64) (PerformerPersonalState, error) {
	var state PerformerPersonalState
	err := dbWrapper.Get(ctx, &state, `SELECT
		COALESCE(MAX(CASE WHEN user_id = ? THEN favorite END), 0) AS favorite,
		MAX(CASE WHEN user_id = ? THEN rating END) AS rating,
		COALESCE(SUM(rating), 0) AS rating_sum,
		COUNT(rating) AS rating_count,
		COALESCE(AVG(rating), 0) AS rating_average
		FROM user_performer_state WHERE performer_id = ?`, userID, userID, performerID)
	return state, err
}

func (s *PersonalStateStore) SetSceneRating(ctx context.Context, userID, sceneID int64, rating *int) error {
	if err := validatePersonalRating(rating); err != nil {
		return err
	}
	_, err := dbWrapper.Exec(ctx, `INSERT INTO user_scene_state (user_id, scene_id, rating, updated_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(user_id, scene_id) DO UPDATE SET rating = excluded.rating, updated_at = excluded.updated_at`,
		userID, sceneID, rating, time.Now().UTC())
	return err
}

func (s *PersonalStateStore) Scene(ctx context.Context, userID, sceneID int64) (ScenePersonalState, error) {
	var state ScenePersonalState
	err := dbWrapper.Get(ctx, &state, `SELECT
		MAX(CASE WHEN user_id = ? THEN rating END) AS rating,
		COUNT(rating) AS rating_count,
		COALESCE(AVG(rating), 0) AS rating_average
		FROM user_scene_state WHERE scene_id = ?`, userID, sceneID)
	return state, err
}

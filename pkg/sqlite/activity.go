package sqlite

import (
	"context"
	"time"
)

type ActivityStore struct{}

const (
	SceneHistoryPlay = "PLAY"
	SceneHistoryO    = "O"
)

func (s *ActivityStore) SaveScene(ctx context.Context, userID, sceneID int64, resumeTime, playDuration *float64) error {
	_, err := dbWrapper.Exec(ctx, `INSERT INTO user_scene_activity (user_id, scene_id, resume_time, play_duration, updated_at)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(user_id, scene_id) DO UPDATE SET
		resume_time = COALESCE(excluded.resume_time, resume_time), play_duration = COALESCE(excluded.play_duration, play_duration), updated_at = excluded.updated_at`,
		userID, sceneID, resumeTime, playDuration, time.Now().UTC())
	return err
}

func (s *ActivityStore) ResetScene(ctx context.Context, userID, sceneID int64, resetResume, resetDuration bool) error {
	if resetResume && resetDuration {
		_, err := dbWrapper.Exec(ctx, `DELETE FROM user_scene_activity WHERE user_id = ? AND scene_id = ?`, userID, sceneID)
		return err
	}
	if !resetResume && !resetDuration {
		return nil
	}
	resumeExpr, durationExpr := "resume_time", "play_duration"
	if resetResume {
		resumeExpr = "NULL"
	}
	if resetDuration {
		durationExpr = "NULL"
	}
	_, err := dbWrapper.Exec(ctx, `UPDATE user_scene_activity SET resume_time = `+resumeExpr+`, play_duration = `+durationExpr+`, updated_at = ? WHERE user_id = ? AND scene_id = ?`, time.Now().UTC(), userID, sceneID)
	return err
}

func (s *ActivityStore) AddSceneHistory(ctx context.Context, userID, sceneID int64, kind string, times []time.Time) ([]time.Time, error) {
	if len(times) == 0 {
		times = []time.Time{time.Now()}
	}
	for _, occurred := range times {
		if _, err := dbWrapper.Exec(ctx, `INSERT INTO user_scene_history (user_id, scene_id, kind, occurred_at) VALUES (?, ?, ?, ?)`, userID, sceneID, kind, occurred); err != nil {
			return nil, err
		}
	}
	return s.SceneHistory(ctx, userID, sceneID, kind)
}

func (s *ActivityStore) DeleteSceneHistory(ctx context.Context, userID, sceneID int64, kind string, times []time.Time) ([]time.Time, error) {
	if len(times) == 0 {
		_, err := dbWrapper.Exec(ctx, `DELETE FROM user_scene_history WHERE id = (SELECT id FROM user_scene_history WHERE user_id = ? AND scene_id = ? AND kind = ? ORDER BY occurred_at DESC, id DESC LIMIT 1)`, userID, sceneID, kind)
		if err != nil {
			return nil, err
		}
	} else {
		for _, occurred := range times {
			_, err := dbWrapper.Exec(ctx, `DELETE FROM user_scene_history WHERE id = (SELECT id FROM user_scene_history WHERE user_id = ? AND scene_id = ? AND kind = ? AND occurred_at = ? LIMIT 1)`, userID, sceneID, kind, occurred)
			if err != nil {
				return nil, err
			}
		}
	}
	return s.SceneHistory(ctx, userID, sceneID, kind)
}

func (s *ActivityStore) ResetSceneHistory(ctx context.Context, userID, sceneID int64, kind string) error {
	_, err := dbWrapper.Exec(ctx, `DELETE FROM user_scene_history WHERE user_id = ? AND scene_id = ? AND kind = ?`, userID, sceneID, kind)
	return err
}

func (s *ActivityStore) SceneHistory(ctx context.Context, userID, sceneID int64, kind string) ([]time.Time, error) {
	var values []time.Time
	err := dbWrapper.Select(ctx, &values, `SELECT occurred_at FROM user_scene_history WHERE user_id = ? AND scene_id = ? AND kind = ? ORDER BY occurred_at, id`, userID, sceneID, kind)
	return values, err
}

func (s *ActivityStore) ChangeImageO(ctx context.Context, userID, imageID int64, delta int) (int, error) {
	_, err := dbWrapper.Exec(ctx, `INSERT INTO user_image_activity (user_id, image_id, o_count, updated_at) VALUES (?, ?, MAX(0, ?), ?)
		ON CONFLICT(user_id, image_id) DO UPDATE SET o_count = MAX(0, o_count + ?), updated_at = excluded.updated_at`, userID, imageID, delta, time.Now().UTC(), delta)
	if err != nil {
		return 0, err
	}
	var count int
	err = dbWrapper.Get(ctx, &count, `SELECT o_count FROM user_image_activity WHERE user_id = ? AND image_id = ?`, userID, imageID)
	return count, err
}

func (s *ActivityStore) ImageO(ctx context.Context, userID, imageID int64) (int, error) {
	var count int
	err := dbWrapper.Get(ctx, &count, `SELECT COALESCE((SELECT o_count FROM user_image_activity WHERE user_id = ? AND image_id = ?), 0)`, userID, imageID)
	return count, err
}

func (s *ActivityStore) ResetImageO(ctx context.Context, userID, imageID int64) (int, error) {
	_, err := dbWrapper.Exec(ctx, `DELETE FROM user_image_activity WHERE user_id = ? AND image_id = ?`, userID, imageID)
	return 0, err
}

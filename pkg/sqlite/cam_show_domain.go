package sqlite

import (
	"context"
	"errors"
	"time"
)

type CamShowDomainSite struct {
	ID   int64   `db:"id"`
	Name string  `db:"name"`
	Icon *string `db:"icon"`
}

type CamShowDomainLink struct {
	ID       int64   `db:"id"`
	LinkType string  `db:"link_type"`
	URL      string  `db:"url"`
	Label    *string `db:"label"`
}

type CamShowDomainModel struct {
	ModelID     int64  `db:"model_id"`
	DisplayName string `db:"display_name"`
	Role        string `db:"participation_role"`
}

type CamShowDomainItem struct {
	ID                     int64                `db:"id"`
	SceneID                int64                `db:"scene_id"`
	Title                  string               `db:"title"`
	ShowType               string               `db:"show_type"`
	ShowDate               *time.Time           `db:"show_date"`
	CapturedAt             *time.Time           `db:"captured_at"`
	CapturedTimezone       *string              `db:"captured_timezone"`
	CapturedPrecision      *string              `db:"captured_precision"`
	DurationSeconds        *float64             `db:"duration_seconds"`
	DurationOverridden     bool                 `db:"duration_overridden"`
	DurationOverrideReason *string              `db:"duration_override_reason"`
	Details                *string              `db:"details"`
	HasFavoriteModel       bool                 `db:"has_favorite_model"`
	Tags                   []CamShowLibraryTag  `db:"-"`
	Sites                  []CamShowDomainSite  `db:"-"`
	Links                  []CamShowDomainLink  `db:"-"`
	Models                 []CamShowDomainModel `db:"-"`
}

func (s *CamShowStore) ListShowDomain(ctx context.Context) ([]CamShowDomainItem, error) {
	return s.ListShowDomainForUser(ctx, 0, false)
}

func (s *CamShowStore) ListShowDomainForUser(ctx context.Context, userID int64, favoriteModelsFirst bool) ([]CamShowDomainItem, error) {
	if favoriteModelsFirst && userID <= 0 {
		return nil, errors.New("persisted user is required for Favorite Models ordering")
	}
	if favoriteModelsFirst {
		var persisted bool
		if err := dbWrapper.Get(ctx, &persisted, `SELECT EXISTS(SELECT 1 FROM users WHERE id=?)`, userID); err != nil {
			return nil, err
		}
		if !persisted {
			return nil, errors.New("persisted user is required for Favorite Models ordering")
		}
	}
	var values []CamShowDomainItem
	query := `SELECT cs.id,cs.scene_id,COALESCE(NULLIF(cs.title_override,''),NULLIF(sc.title,''),'Show ' || cs.id) AS title,cs.show_type,cs.show_date,cs.captured_at,cs.captured_timezone,cs.captured_precision,COALESCE(cs.duration_override_seconds,(SELECT vf.duration FROM scenes_files sf JOIN video_files vf ON vf.file_id=sf.file_id WHERE sf.scene_id=cs.scene_id ORDER BY sf."primary" DESC,sf.file_id LIMIT 1)) AS duration_seconds,(cs.duration_override_seconds IS NOT NULL) AS duration_overridden,cs.duration_override_reason,cs.notes AS details,`
	args := []interface{}{}
	if favoriteModelsFirst {
		query += `EXISTS(SELECT 1 FROM cam_show_models csm JOIN cam_model_user_state us ON us.model_id=csm.model_id WHERE csm.show_id=cs.id AND us.user_id=? AND us.favorite=1) AS has_favorite_model `
		args = append(args, userID)
	} else {
		query += `0 AS has_favorite_model `
	}
	query += `FROM cam_shows cs JOIN scenes sc ON sc.id=cs.scene_id `
	if favoriteModelsFirst {
		query += `ORDER BY has_favorite_model DESC,`
	} else {
		query += `ORDER BY `
	}
	query += `COALESCE(cs.show_date,date(cs.captured_at)) DESC,cs.id DESC`
	err := dbWrapper.Select(ctx, &values, query, args...)
	if err != nil {
		return nil, err
	}
	for i := range values {
		if err := dbWrapper.Select(ctx, &values[i].Tags, `SELECT t.id,t.name FROM tags t JOIN scenes_tags st ON st.tag_id=t.id WHERE st.scene_id=? ORDER BY t.name COLLATE NOCASE,t.id`, values[i].SceneID); err != nil {
			return nil, err
		}
		if err := dbWrapper.Select(ctx, &values[i].Sites, `SELECT s.id,s.name,s.icon FROM cam_sites s JOIN cam_show_sites ss ON ss.site_id=s.id WHERE ss.show_id=? ORDER BY s.name COLLATE NOCASE,s.id`, values[i].ID); err != nil {
			return nil, err
		}
		if err := dbWrapper.Select(ctx, &values[i].Links, `SELECT id,link_type,url,label FROM cam_show_links WHERE show_id=? ORDER BY link_type,id`, values[i].ID); err != nil {
			return nil, err
		}
		if err := dbWrapper.Select(ctx, &values[i].Models, `SELECT m.id AS model_id,m.display_name,COALESCE(sm.participation_role,'PARTICIPANT') AS participation_role FROM cam_models m JOIN cam_show_models sm ON sm.model_id=m.id WHERE sm.show_id=? ORDER BY sm.billing_order,m.display_name COLLATE NOCASE,m.id`, values[i].ID); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func validCamShowRole(role string) bool {
	switch role {
	case "SOLO", "PRIMARY", "GUEST", "PARTICIPANT":
		return true
	}
	return false
}

func (s *CamShowStore) LinkModelWithRole(ctx context.Context, showID, modelID int64, billingOrder int, role string) error {
	if showID <= 0 || modelID <= 0 || billingOrder < 0 || !validCamShowRole(role) {
		return errors.New("invalid Cam Show model link")
	}
	_, err := dbWrapper.Exec(ctx, `INSERT INTO cam_show_models(show_id,model_id,billing_order,participation_role) VALUES(?,?,?,?)`, showID, modelID, billingOrder, role)
	return err
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/cammodel/profilescraper"
)

type CamModelProfileScrapeTarget struct {
	AccountID  int64  `db:"account_id"`
	ModelID    int64  `db:"model_id"`
	ProfileURL string `db:"profile_url"`
}

func (s *CamShowStore) FindModelProfileScrapeTarget(ctx context.Context, accountID int64) (*CamModelProfileScrapeTarget, error) {
	var value CamModelProfileScrapeTarget
	err := dbWrapper.Get(ctx, &value, `SELECT id account_id,model_id,profile_url FROM cam_model_accounts WHERE id=? AND valid_to IS NULL AND profile_url IS NOT NULL AND trim(profile_url)<>''`, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func (s *CamShowStore) ApplyScrapedProfileMetadata(ctx context.Context, modelID int64, value profilescraper.Metadata) (*CamModelProfile, error) {
	if modelID <= 0 {
		return nil, errors.New("valid Cam Model is required")
	}
	now := time.Now().UTC()
	_, err := dbWrapper.Exec(ctx, `UPDATE cam_models SET
		image=CASE WHEN image IS NULL OR trim(image)='' THEN ? ELSE image END,
		location=CASE WHEN location IS NULL OR trim(location)='' THEN ? ELSE location END,
		age=COALESCE(age,?),updated_at=? WHERE id=?`, value.ImageURL, value.Location, value.Age, now, modelID)
	if err != nil {
		return nil, err
	}
	for _, social := range value.Socials {
		var count int
		if err := dbWrapper.Get(ctx, &count, `SELECT count(*) FROM cam_model_social_profiles WHERE model_id=? AND lower(url)=lower(?)`, modelID, social.URL); err != nil {
			return nil, err
		}
		if count != 0 {
			continue
		}
		if _, err := s.AddModelSocialProfile(ctx, CamModelSocialProfileInput{ModelID: modelID, Platform: social.Platform, Handle: social.Handle, URL: social.URL, Source: "PROFILE_SCRAPER"}); err != nil {
			return nil, err
		}
	}
	return s.FindModelProfile(ctx, modelID)
}

func cleanScrapedText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

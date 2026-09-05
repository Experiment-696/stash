package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/cammodel"
)

var ErrCamModelDiscoverySite = errors.New("discovery platform is not configured as an enabled cam site")

type CamModelDiscoveryApplyResult struct {
	AccountID    int64
	Disposition  string
	ImageApplied bool
}

// ApplyDiscoveryMetadata attaches an explicitly selected provider result to an
// existing Cam Model. It writes usable identity metadata directly and does not
// create evidence, provenance, or a review queue entry.
func (s *CamShowStore) ApplyDiscoveryMetadata(ctx context.Context, modelID int64, o cammodel.ProfileObservation) (*CamModelDiscoveryApplyResult, error) {
	if modelID <= 0 || strings.TrimSpace(o.Platform) == "" || strings.TrimSpace(o.Username) == "" {
		return nil, errors.New("model, platform, and username are required")
	}
	var site CamSite
	if err := dbWrapper.Get(ctx, &site, "SELECT id,name,base_url,external_key,enabled,created_at,updated_at FROM cam_sites WHERE external_key=? AND enabled=1", o.Platform); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrCamModelDiscoverySite, o.Platform)
		}
		return nil, err
	}
	model, err := s.FindModel(ctx, modelID)
	if err != nil {
		return nil, err
	} else if model == nil {
		return nil, sql.ErrNoRows
	}
	disposition := "CREATED"
	var account struct {
		ID      int64 `db:"id"`
		ModelID int64 `db:"model_id"`
	}
	err = dbWrapper.Get(ctx, &account, "SELECT id,model_id FROM cam_model_accounts WHERE site_id=? AND normalized_handle=? ORDER BY CASE status WHEN 'ACTIVE' THEN 0 ELSE 1 END,id LIMIT 1", site.ID, normalizeCamIdentity(o.Username))
	if err == nil {
		if account.ModelID == modelID {
			disposition = "EXISTING"
		} else {
			return nil, errors.New("that site username is already attached to another Cam Model")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	now := time.Now().UTC()
	if disposition == "CREATED" {
		result, insertErr := dbWrapper.Exec(ctx, `INSERT INTO cam_model_accounts
			(model_id,site_id,handle,normalized_handle,profile_url,status,last_seen_at,source,created_at,updated_at)
			VALUES(?,?,?,?,?,'ACTIVE',?,'CAMGIRLFINDER',?,?)`, modelID, site.ID, strings.TrimSpace(o.Username), normalizeCamIdentity(o.Username), o.SourceURL, o.ObservedAt.UTC(), now, now)
		if insertErr != nil {
			return nil, insertErr
		}
		account.ID, err = result.LastInsertId()
	} else {
		_, err = dbWrapper.Exec(ctx, `UPDATE cam_model_accounts SET profile_url=COALESCE(NULLIF(?,''),profile_url),last_seen_at=?,last_synced_at=?,updated_at=? WHERE id=?`, o.SourceURL, o.ObservedAt.UTC(), now, now, account.ID)
	}
	if err != nil {
		return nil, err
	}
	var aliasCount int
	if err = dbWrapper.Get(ctx, &aliasCount, `SELECT COUNT(*) FROM cam_model_aliases WHERE model_id=? AND account_id=? AND site_id=? AND normalized_alias=? AND is_current=1`, modelID, account.ID, site.ID, normalizeCamIdentity(o.Username)); err != nil {
		return nil, err
	}
	if aliasCount == 0 {
		_, err = dbWrapper.Exec(ctx, `INSERT INTO cam_model_aliases(model_id,account_id,site_id,alias,normalized_alias,is_current,source,created_at,updated_at) VALUES(?,?,?,?,?,1,'CAMGIRLFINDER',?,?)`, modelID, account.ID, site.ID, strings.TrimSpace(o.Username), normalizeCamIdentity(o.Username), now, now)
		if err != nil {
			return nil, err
		}
	}
	imageApplied := false
	if (model.Image == nil || strings.TrimSpace(*model.Image) == "") && o.ImageURL != nil {
		value := strings.TrimSpace(*o.ImageURL)
		parsed, parseErr := url.Parse(value)
		if parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
			result, updateErr := dbWrapper.Exec(ctx, `UPDATE cam_models SET image=?,updated_at=? WHERE id=? AND (image IS NULL OR trim(image)='')`, value, now, modelID)
			if updateErr != nil {
				return nil, updateErr
			}
			changed, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return nil, rowsErr
			}
			imageApplied = changed == 1
		}
	}
	return &CamModelDiscoveryApplyResult{AccountID: account.ID, Disposition: disposition, ImageApplied: imageApplied}, nil
}

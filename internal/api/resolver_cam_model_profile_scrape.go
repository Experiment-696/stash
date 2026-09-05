package api

import (
	"context"

	"github.com/stashapp/stash/pkg/cammodel/profilescraper"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func (r *mutationResolver) CamModelProfileScrape(ctx context.Context, accountID string) (*CamModelProfile, error) {
	id, err := parseCamID(accountID)
	if err != nil {
		return nil, err
	}
	db := r.tokenDatabase()
	var target *sqlite.CamModelProfileScrapeTarget
	err = txn.WithReadTxn(ctx, db, func(txCtx context.Context) error {
		if _, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db); authErr != nil {
			return authErr
		}
		var findErr error
		target, findErr = db.CamShow.FindModelProfileScrapeTarget(txCtx, id)
		return findErr
	})
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrInput
	}
	metadata, err := profilescraper.New(nil).Scrape(ctx, target.ProfileURL)
	if err != nil {
		return nil, err
	}
	var profile *sqlite.CamModelProfile
	err = txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		var applyErr error
		profile, applyErr = db.CamShow.ApplyScrapedProfileMetadata(txCtx, target.ModelID, metadata)
		if applyErr != nil {
			return applyErr
		}
		return recordCamAudit(txCtx, db, actorID, camAuditProfileUpdated, "cam_model", target.ModelID, "profile scraped")
	})
	if err != nil {
		return nil, err
	}
	return camProfileModel(profile, true), nil
}

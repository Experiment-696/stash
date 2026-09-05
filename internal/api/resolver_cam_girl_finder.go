package api

import (
	"context"
	"errors"
	"github.com/stashapp/stash/internal/manager"
	managerconfig "github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
	"github.com/stashapp/stash/pkg/cammodel/camgirlfinder"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
	"strconv"
)

func camGirlFinderConfigModel(v managerconfig.CamGirlFinderConfig) *CamGirlFinderConfig {
	return &CamGirlFinderConfig{Enabled: v.Enabled, RequestIntervalMs: v.RequestIntervalMS, TimeoutSeconds: v.TimeoutSeconds, ResultLimit: v.ResultLimit}
}
func (r *queryResolver) CamGirlFinderConfig(ctx context.Context) (*CamGirlFinderConfig, error) {
	db := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, db, func(txCtx context.Context) error { _, e := requirePersistedCamModelAdmin(ctx, txCtx, db); return e }); err != nil {
		return nil, err
	}
	return camGirlFinderConfigModel(managerconfig.GetInstance().GetCamGirlFinderConfig()), nil
}
func (r *mutationResolver) CamGirlFinderConfigure(ctx context.Context, input CamGirlFinderConfigInput) (*CamGirlFinderConfig, error) {
	db := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, db, func(txCtx context.Context) error { _, e := requirePersistedCamModelAdmin(ctx, txCtx, db); return e }); err != nil {
		return nil, err
	}
	value := managerconfig.CamGirlFinderConfig{Enabled: input.Enabled, RequestIntervalMS: input.RequestIntervalMs, TimeoutSeconds: input.TimeoutSeconds, ResultLimit: input.ResultLimit}
	if err := manager.ValidateCamGirlFinderConfig(value); err != nil {
		return nil, err
	}
	cfg := managerconfig.GetInstance()
	cfg.SetCamGirlFinderConfig(value)
	if err := cfg.Write(); err != nil {
		return nil, err
	}
	return camGirlFinderConfigModel(value), nil
}
func configuredCamGirlFinder() (cammodel.DiscoveryProvider, error) {
	value := managerconfig.GetInstance().GetCamGirlFinderConfig()
	return camgirlfinder.New(manager.AdapterCamGirlFinderConfig(value), nil)
}
func candidateModel(o cammodel.ProfileObservation) *CamGirlFinderCandidate {
	source := ""
	if o.SourceURL != nil {
		source = *o.SourceURL
	}
	return &CamGirlFinderCandidate{EvidenceKey: o.EvidenceKey, Platform: o.Platform, Username: o.Username, SourceURL: source, ImageURL: o.ImageURL, ObservedAt: o.ObservedAt, PayloadJSON: o.PayloadJSON}
}
func (r *mutationResolver) CamGirlFinderSearch(ctx context.Context, query string) ([]*CamGirlFinderCandidate, error) {
	db := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, db, func(txCtx context.Context) error { _, e := requirePersistedCamModelAdmin(ctx, txCtx, db); return e }); err != nil {
		return nil, err
	}
	provider, err := configuredCamGirlFinder()
	if err != nil {
		return nil, err
	}
	items, err := provider.Discover(ctx, query)
	if err != nil {
		return nil, err
	}
	ret := make([]*CamGirlFinderCandidate, len(items))
	for i := range items {
		ret[i] = candidateModel(items[i])
	}
	return ret, nil
}
func (r *mutationResolver) CamGirlFinderIngestPending(ctx context.Context, modelID string, query string, evidenceKeys []string) ([]*CamGirlFinderIngestResult, error) {
	id, err := strconv.ParseInt(modelID, 10, 64)
	if err != nil || id <= 0 || len(evidenceKeys) == 0 {
		return nil, ErrInput
	}
	db := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, db, func(txCtx context.Context) error { _, e := requirePersistedCamModelAdmin(ctx, txCtx, db); return e }); err != nil {
		return nil, err
	}
	provider, err := configuredCamGirlFinder()
	if err != nil {
		return nil, err
	}
	observations, err := provider.Discover(ctx, query)
	if err != nil {
		return nil, err
	}
	selected, err := selectCamGirlFinderObservations(observations, evidenceKeys)
	if err != nil {
		return nil, err
	}
	ret := make([]*CamGirlFinderIngestResult, 0, len(selected))
	err = txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, e := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if e != nil {
			return e
		}
		ret, e = persistCamGirlFinderPending(txCtx, db, actorID, id, selected)
		return e
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func persistCamGirlFinderPending(ctx context.Context, db *sqlite.Database, actorID, modelID int64, selected []cammodel.ProfileObservation) ([]*CamGirlFinderIngestResult, error) {
	if model, err := db.CamShow.FindModel(ctx, modelID); err != nil {
		return nil, err
	} else if model == nil {
		return nil, ErrInput
	}
	ret := make([]*CamGirlFinderIngestResult, 0, len(selected))
	for _, observation := range selected {
		value, err := db.CamShow.ApplyDiscoveryMetadata(ctx, modelID, observation)
		if errors.Is(err, sqlite.ErrCamModelDiscoverySite) {
			reason := err.Error()
			ret = append(ret, &CamGirlFinderIngestResult{EvidenceKey: observation.EvidenceKey, Status: "REJECTED", Reason: &reason})
			continue
		}
		if err != nil {
			return nil, err
		}
		disposition := value.Disposition
		ret = append(ret, &CamGirlFinderIngestResult{EvidenceKey: observation.EvidenceKey, Status: "APPLIED", Disposition: &disposition, ImageApplied: value.ImageApplied})
		if err := recordCamAudit(ctx, db, actorID, camAuditDiscoveryIngested, "cam_model_account", value.AccountID, "APPLIED"); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

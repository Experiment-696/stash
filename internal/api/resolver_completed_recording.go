package api

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/authz"
	managerconfig "github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

type completedImportContextAuthorizer struct {
	database *sqlite.Database
}

func (a completedImportContextAuthorizer) RequireDataAdmin(ctx context.Context) error {
	if a.database == nil {
		return authz.UnauthenticatedError{}
	}
	return txn.WithDatabase(ctx, a.database, func(dbCtx context.Context) error {
		return requirePersistedCamClassificationAdmin(ctx, dbCtx, a.database)
	})
}

type completedImportDatabaseResolver struct {
	database *sqlite.Database
}

func (r completedImportDatabaseResolver) ResolveCompletedRecording(ctx context.Context, root, relativePath string, parsed cammodel.CompletedParsedName) (cammodel.CompletedResolution, error) {
	var ret cammodel.CompletedResolution
	err := txn.WithDatabase(ctx, r.database, func(txCtx context.Context) error {
		var err error
		ret, err = r.database.CamShow.ResolveCompletedRecording(txCtx, root, relativePath, parsed)
		return err
	})
	return ret, err
}

func (r *Resolver) completedRecordingService() *cammodel.CompletedImportService {
	r.completedImportMu.Lock()
	defer r.completedImportMu.Unlock()
	if r.completedImportService == nil {
		database := r.tokenDatabase()
		r.completedImportService = &cammodel.CompletedImportService{
			Authorizer: completedImportContextAuthorizer{database: database},
			Resolver:   completedImportDatabaseResolver{database: database},
			Store:      sqlite.NewCompletedImportRepository(database),
		}
		r.completedImportService.AcquireConfiguredRoot = r.acquireCompletedRecordingRoot
	} else if r.completedImportService.AcquireConfiguredRoot == nil {
		// Test-injected services are completed once under completedImportMu.
		r.completedImportService.AcquireConfiguredRoot = r.acquireCompletedRecordingRoot
	}
	return r.completedImportService
}

func (r *Resolver) acquireCompletedRecordingRoot(expected string) (func(), error) {
	r.completedImportConfigMu.RLock()
	root, err := activeCompletedRecordingRootUnlocked()
	if err != nil || root != expected {
		r.completedImportConfigMu.RUnlock()
		return nil, errors.New("configured root changed; run preview again")
	}
	return r.completedImportConfigMu.RUnlock, nil
}

func completedRecordingConfigModel(value managerconfig.CompletedRecordingImportConfig) *CompletedRecordingImportConfig {
	return &CompletedRecordingImportConfig{Enabled: value.Enabled, Root: value.Root}
}

func normalizeCompletedRecordingConfig(value managerconfig.CompletedRecordingImportConfig, libraryRoots managerconfig.StashConfigs) (managerconfig.CompletedRecordingImportConfig, error) {
	value.Root = strings.TrimSpace(value.Root)
	if value.Root == "" {
		if value.Enabled {
			return managerconfig.CompletedRecordingImportConfig{}, ErrInput
		}
		return managerconfig.CompletedRecordingImportConfig{}, nil
	}
	root, err := cammodel.CanonicalCompletedImportRoot(value.Root)
	if err != nil {
		return managerconfig.CompletedRecordingImportConfig{}, ErrInput
	}
	approved := false
	for _, configured := range libraryRoots.Paths() {
		candidate, candidateErr := cammodel.CanonicalCompletedImportRoot(configured)
		if candidateErr == nil && candidate == root {
			approved = true
			break
		}
	}
	if !approved {
		return managerconfig.CompletedRecordingImportConfig{}, ErrInput
	}
	value.Root = root
	return value, nil
}

func activeCompletedRecordingRootUnlocked() (string, error) {
	cfg := managerconfig.GetInstance()
	value := cfg.GetCompletedRecordingImportConfig()
	if !value.Enabled {
		return "", errors.New("completed-recording import is disabled")
	}
	normalized, err := normalizeCompletedRecordingConfig(value, cfg.GetStashPaths())
	if err != nil {
		return "", errors.New("completed-recording import configuration is invalid")
	}
	return normalized.Root, nil
}

func (r *Resolver) activeCompletedRecordingRoot() (string, error) {
	r.completedImportConfigMu.RLock()
	defer r.completedImportConfigMu.RUnlock()
	return activeCompletedRecordingRootUnlocked()
}

func completedRecordingOptions(input CompletedRecordingPreviewInput, root string) (cammodel.CompletedDiscoveryOptions, cammodel.CompletedFilenameParser, error) {
	if input.MaxFiles < 1 || input.MaxFiles > 10000 || input.MaxDepth < 1 || input.MaxDepth > 64 ||
		input.TimeoutMs < 1 || input.TimeoutMs > int((5*time.Minute)/time.Millisecond) ||
		len(input.Extensions) < 1 || len(input.Extensions) > 32 || len(input.FilenamePattern) > 2048 ||
		len(input.TimestampLayout) > 128 || len(input.Timezone) > 128 ||
		len(input.ParserVersion) > 80 || strings.TrimSpace(input.ParserVersion) == "" {
		return cammodel.CompletedDiscoveryOptions{}, cammodel.CompletedFilenameParser{}, ErrInput
	}
	pattern, err := regexp.Compile(input.FilenamePattern)
	if err != nil {
		return cammodel.CompletedDiscoveryOptions{}, cammodel.CompletedFilenameParser{}, ErrInput
	}
	location, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return cammodel.CompletedDiscoveryOptions{}, cammodel.CompletedFilenameParser{}, ErrInput
	}
	extensions := make(map[string]struct{}, len(input.Extensions))
	for _, value := range input.Extensions {
		value = strings.ToLower(strings.TrimSpace(value))
		if len(value) < 2 || len(value) > 16 || !strings.HasPrefix(value, ".") || strings.ContainsAny(value, "/\\") {
			return cammodel.CompletedDiscoveryOptions{}, cammodel.CompletedFilenameParser{}, ErrInput
		}
		extensions[value] = struct{}{}
	}
	precision := cammodel.CompletedTimePrecision(input.Precision)
	return cammodel.CompletedDiscoveryOptions{
			Root: root, MaxFiles: input.MaxFiles, MaxDepth: input.MaxDepth,
			Timeout: time.Duration(input.TimeoutMs) * time.Millisecond, Extensions: extensions,
		}, cammodel.CompletedFilenameParser{
			Pattern: pattern, Layout: input.TimestampLayout, Location: location,
			Precision: precision, Version: input.ParserVersion,
		}, nil
}

func (r *queryResolver) CompletedRecordingImportConfig(ctx context.Context) (*CompletedRecordingImportConfig, error) {
	database := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		return requirePersistedCamClassificationAdmin(ctx, txCtx, database)
	}); err != nil {
		return nil, err
	}
	r.completedImportConfigMu.RLock()
	defer r.completedImportConfigMu.RUnlock()
	return completedRecordingConfigModel(managerconfig.GetInstance().GetCompletedRecordingImportConfig()), nil
}

func (r *mutationResolver) CompletedRecordingImportConfigure(ctx context.Context, input CompletedRecordingImportConfigInput) (*CompletedRecordingImportConfig, error) {
	database := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		return requirePersistedCamClassificationAdmin(ctx, txCtx, database)
	}); err != nil {
		return nil, err
	}
	cfg := managerconfig.GetInstance()
	r.completedImportConfigMu.Lock()
	defer r.completedImportConfigMu.Unlock()
	value, err := normalizeCompletedRecordingConfig(managerconfig.CompletedRecordingImportConfig{Enabled: input.Enabled, Root: input.Root}, cfg.GetStashPaths())
	if err != nil {
		return nil, err
	}
	cfg.SetCompletedRecordingImportConfig(value)
	if err := cfg.Write(); err != nil {
		return nil, errors.New("unable to save completed-recording import configuration")
	}
	return completedRecordingConfigModel(value), nil
}

func completedRecordingError(err error) error {
	if err == nil {
		return nil
	}
	var unauthenticated authz.UnauthenticatedError
	var denied authz.DeniedError
	if errors.As(err, &unauthenticated) || errors.As(err, &denied) || errors.Is(err, ErrInput) {
		return err
	}
	message := strings.TrimSpace(err.Error())
	if message == "" || len(message) > 256 || strings.ContainsAny(message, "/\\") {
		return errors.New("completed-recording operation failed")
	}
	return errors.New(message)
}

func optionalCompletedID(value int64) *string {
	if value <= 0 {
		return nil
	}
	ret := strconv.FormatInt(value, 10)
	return &ret
}

func completedRecordingPreviewModel(preview cammodel.CompletedImportPreview) *CompletedRecordingPreview {
	ret := &CompletedRecordingPreview{
		PreviewID: preview.ID, CreatedAt: preview.CreatedAt, ExpiresAt: preview.ExpiresAt,
		ScannedCount: preview.ScannedCount, Truncated: preview.Truncated,
		Items: make([]*CompletedRecordingCandidate, len(preview.Items)),
	}
	if preview.BoundReason != cammodel.CompletedBoundNone {
		value := CompletedRecordingBoundReason(preview.BoundReason)
		ret.BoundReason = &value
	}
	for i, item := range preview.Items {
		candidate := &CompletedRecordingCandidate{
			CandidateID: item.CandidateID, RelativePath: item.RelativePath,
			Platform: item.Platform, Username: item.Username, Timezone: item.Timezone,
			SceneID: optionalCompletedID(item.SceneID), SiteID: optionalCompletedID(item.SiteID),
			ModelID: optionalCompletedID(item.ModelID), MatchState: string(item.MatchState),
			Outcome: CompletedRecordingOutcome(item.Outcome),
		}
		if !item.CompletedAt.IsZero() {
			value := item.CompletedAt
			candidate.CompletedAt = &value
		}
		if item.TimePrecision != "" {
			value := CompletedRecordingPrecision(item.TimePrecision)
			candidate.Precision = &value
		}
		if item.ReviewReason != "" {
			value := strings.TrimSpace(item.ReviewReason)
			if len(value) > 256 || strings.ContainsAny(value, "/\\") {
				value = "redacted review detail"
			}
			candidate.ReviewReason = &value
		}
		if item.ReviewCode != cammodel.CompletedReviewNone {
			value := CompletedRecordingReviewReason(item.ReviewCode)
			candidate.ReviewCode = &value
		}
		ret.Items[i] = candidate
	}
	return ret
}

func (r *mutationResolver) CompletedRecordingPreview(ctx context.Context, input CompletedRecordingPreviewInput) (*CompletedRecordingPreview, error) {
	database := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		return requirePersistedCamClassificationAdmin(ctx, txCtx, database)
	}); err != nil {
		return nil, err
	}
	root, err := r.activeCompletedRecordingRoot()
	if err != nil {
		return nil, completedRecordingError(err)
	}
	options, parser, err := completedRecordingOptions(input, root)
	if err != nil {
		return nil, err
	}
	preview, err := r.completedRecordingService().DryRun(ctx, options, parser)
	if err != nil {
		return nil, completedRecordingError(err)
	}
	return completedRecordingPreviewModel(preview), nil
}

func (r *mutationResolver) CompletedRecordingApply(ctx context.Context, input CompletedRecordingApplyInput) ([]*CompletedRecordingApplyResult, error) {
	database := r.tokenDatabase()
	if err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		return requirePersistedCamClassificationAdmin(ctx, txCtx, database)
	}); err != nil {
		return nil, err
	}
	principal, err := authz.RequireContext(ctx, authz.DataAdmin)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.PreviewID) == "" || len(input.SelectedCandidateIDs) > 10000 {
		return nil, ErrInput
	}
	root, err := r.activeCompletedRecordingRoot()
	if err != nil {
		return nil, completedRecordingError(err)
	}
	results, err := r.completedRecordingService().ApplyConfigured(ctx, principal.UserID, input.PreviewID, input.SelectedCandidateIDs, root)
	if err != nil {
		return nil, completedRecordingError(err)
	}
	ret := make([]*CompletedRecordingApplyResult, len(results))
	for i, result := range results {
		value := &CompletedRecordingApplyResult{
			CandidateID: result.CandidateID,
			Outcome:     CompletedRecordingOutcome(result.Outcome),
		}
		if result.Reason != "" {
			reason := strings.TrimSpace(result.Reason)
			if len(reason) > 256 || strings.ContainsAny(reason, "/\\") {
				reason = "redacted result detail"
			}
			value.Reason = &reason
		}
		ret[i] = value
	}
	return ret, nil
}

var _ cammodel.CompletedImportResolver = completedImportDatabaseResolver{}
var _ cammodel.CompletedImportAuthorizer = completedImportContextAuthorizer{}

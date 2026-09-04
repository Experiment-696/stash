package cammodel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

type CompletedImportOutcome string

const (
	CompletedExactReady      CompletedImportOutcome = "EXACT_READY"
	CompletedAlreadyApplied  CompletedImportOutcome = "ALREADY_APPLIED"
	CompletedReviewRequired  CompletedImportOutcome = "REVIEW_REQUIRED"
	CompletedSiteNotFound    CompletedImportOutcome = "SITE_NOT_FOUND"
	CompletedModelNotFound   CompletedImportOutcome = "MODEL_NOT_FOUND"
	CompletedSceneNotFound   CompletedImportOutcome = "SCENE_NOT_FOUND"
	CompletedInvalidName     CompletedImportOutcome = "INVALID_NAME"
	CompletedChanged         CompletedImportOutcome = "CHANGED_SINCE_PREVIEW"
	CompletedOutsideRoot     CompletedImportOutcome = "OUTSIDE_ROOT"
	CompletedRejectedSymlink CompletedImportOutcome = "REJECTED_SYMLINK"
	CompletedApplied         CompletedImportOutcome = "APPLIED"
	CompletedSkipped         CompletedImportOutcome = "SKIPPED"
)

type CompletedAliasMatchState string

const (
	CompletedAliasNone       CompletedAliasMatchState = "NONE"
	CompletedAliasCurrent    CompletedAliasMatchState = "EXACT_CURRENT"
	CompletedAliasHistorical CompletedAliasMatchState = "EXACT_HISTORICAL"
	CompletedAliasAmbiguous  CompletedAliasMatchState = "AMBIGUOUS"
)

type CompletedBoundReason string

const (
	CompletedBoundNone     CompletedBoundReason = ""
	CompletedBoundMaxFiles CompletedBoundReason = "MAX_FILES"
	CompletedBoundMaxDepth CompletedBoundReason = "MAX_DEPTH"
	CompletedBoundTimeout  CompletedBoundReason = "TIMEOUT"
)

type CompletedReviewReasonCode string

const (
	CompletedReviewNone            CompletedReviewReasonCode = ""
	CompletedReviewMultipleScenes  CompletedReviewReasonCode = "MULTIPLE_SCENES"
	CompletedReviewMultipleSites   CompletedReviewReasonCode = "MULTIPLE_SITES"
	CompletedReviewMultipleAliases CompletedReviewReasonCode = "MULTIPLE_ALIASES"
	CompletedReviewAliasReused     CompletedReviewReasonCode = "HISTORICAL_ALIAS_REUSED"
)

type CompletedTimePrecision string

const (
	CompletedTimeDate   CompletedTimePrecision = "DATE"
	CompletedTimeMinute CompletedTimePrecision = "MINUTE"
	CompletedTimeSecond CompletedTimePrecision = "SECOND"
)

type CompletedStatFingerprint struct {
	Size, ModTimeNS int64
	Mode            uint32
	Device, Inode   uint64
}

type CompletedParsedName struct {
	Site, Handle  string
	ObservedAt    time.Time
	Timezone      string
	Precision     CompletedTimePrecision
	ParserVersion string
}

type CompletedResolution struct {
	SceneID, SiteID, ModelID int64
	MatchState               CompletedAliasMatchState
	Outcome                  CompletedImportOutcome
	ReviewReason             string
	ReviewCode               CompletedReviewReasonCode
}

type CompletedFilenameParser struct {
	Pattern   *regexp.Regexp
	Layout    string
	Location  *time.Location
	Precision CompletedTimePrecision
	Version   string
}

func (p CompletedFilenameParser) Parse(name string) (CompletedParsedName, error) {
	if p.Pattern == nil || p.Layout == "" || p.Location == nil || strings.TrimSpace(p.Version) == "" {
		return CompletedParsedName{}, errors.New("invalid completed-recording parser")
	}
	match := p.Pattern.FindStringSubmatch(name)
	if match == nil {
		return CompletedParsedName{}, errors.New("filename does not match")
	}
	values := map[string]string{}
	for i, key := range p.Pattern.SubexpNames() {
		if i > 0 && key != "" {
			values[key] = match[i]
		}
	}
	site := strings.TrimSpace(values["site"])
	handle := strings.TrimSpace(values["model"])
	rawTime := strings.TrimSpace(values["timestamp"])
	if site == "" || handle == "" || rawTime == "" {
		return CompletedParsedName{}, errors.New("site, model, and timestamp captures are required")
	}
	observed, err := time.ParseInLocation(p.Layout, rawTime, p.Location)
	if err != nil {
		return CompletedParsedName{}, fmt.Errorf("parse captured time: %w", err)
	}
	return CompletedParsedName{
		Site: site, Handle: handle, ObservedAt: observed,
		Timezone: p.Location.String(), Precision: p.Precision, ParserVersion: p.Version,
	}, nil
}

type CompletedImportAuthorizer interface {
	RequireDataAdmin(context.Context) error
}

type CompletedImportResolver interface {
	ResolveCompletedRecording(context.Context, string, string, CompletedParsedName) (CompletedResolution, error)
}

type CompletedAliasBinding struct {
	SiteID, ModelID    int64
	NormalizedAlias    string
	ValidFrom, ValidTo *time.Time
	Current            bool
}

type CompletedIdentitySnapshotResolver struct {
	Normalize func(string) string
	Scenes    map[string][]int64
	Sites     map[string][]int64
	Aliases   []CompletedAliasBinding
}

func (r CompletedIdentitySnapshotResolver) ResolveCompletedRecording(_ context.Context, _ string, rel string, parsed CompletedParsedName) (CompletedResolution, error) {
	normalize := r.Normalize
	if normalize == nil {
		normalize = func(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
	}
	sceneIDs := r.Scenes[filepath.Clean(rel)]
	if len(sceneIDs) == 0 {
		return CompletedResolution{Outcome: CompletedSceneNotFound, ReviewReason: "existing Scene path required"}, nil
	}
	if len(sceneIDs) > 1 {
		return CompletedResolution{Outcome: CompletedReviewRequired, ReviewReason: "multiple Scenes have the same path", ReviewCode: CompletedReviewMultipleScenes}, nil
	}
	siteIDs := r.Sites[normalize(parsed.Site)]
	if len(siteIDs) == 0 {
		return CompletedResolution{SceneID: sceneIDs[0], Outcome: CompletedSiteNotFound, ReviewReason: "site not found"}, nil
	}
	if len(siteIDs) != 1 {
		return CompletedResolution{SceneID: sceneIDs[0], Outcome: CompletedReviewRequired, MatchState: CompletedAliasAmbiguous, ReviewReason: "site alias is ambiguous", ReviewCode: CompletedReviewMultipleSites}, nil
	}
	siteID := siteIDs[0]
	handle := normalize(parsed.Handle)
	type match struct {
		modelID int64
		state   CompletedAliasMatchState
	}
	var matches []match
	for _, alias := range r.Aliases {
		if alias.SiteID != siteID || normalize(alias.NormalizedAlias) != handle {
			continue
		}
		inRange := (alias.ValidFrom == nil || !parsed.ObservedAt.Before(*alias.ValidFrom)) &&
			(alias.ValidTo == nil || parsed.ObservedAt.Before(*alias.ValidTo))
		if !inRange {
			continue
		}
		state := CompletedAliasHistorical
		if alias.Current && alias.ValidTo == nil {
			state = CompletedAliasCurrent
		}
		matches = append(matches, match{modelID: alias.ModelID, state: state})
	}
	if len(matches) == 0 {
		return CompletedResolution{SceneID: sceneIDs[0], SiteID: siteID, Outcome: CompletedModelNotFound, ReviewReason: "no exact timestamp-valid alias"}, nil
	}
	if len(matches) != 1 {
		return CompletedResolution{SceneID: sceneIDs[0], SiteID: siteID, Outcome: CompletedReviewRequired, MatchState: CompletedAliasAmbiguous, ReviewReason: "alias is reused or validity intervals overlap", ReviewCode: CompletedReviewMultipleAliases}, nil
	}
	if matches[0].state == CompletedAliasHistorical {
		return CompletedResolution{
			SceneID: sceneIDs[0], SiteID: siteID, ModelID: matches[0].modelID,
			MatchState: CompletedAliasHistorical, Outcome: CompletedReviewRequired,
			ReviewReason: "unique historical alias requires identity review", ReviewCode: CompletedReviewAliasReused,
		}, nil
	}
	return CompletedResolution{
		SceneID: sceneIDs[0], SiteID: siteID, ModelID: matches[0].modelID,
		MatchState: CompletedAliasCurrent, Outcome: CompletedExactReady,
	}, nil
}

type CompletedImportTx interface {
	LinkCamShowMetadata(context.Context, CompletedRecording) (bool, error)
	WriteCompletedImportAudit(context.Context, CompletedImportAudit) error
}

type CompletedImportStore interface {
	WithCompletedImportTransaction(context.Context, func(context.Context, CompletedImportTx) error) error
}

type CompletedImportAudit struct {
	ActorID, PreviewID, CandidateID, RelativePathHash string
	Outcome, Reason                                   string
	ReviewCode                                        CompletedReviewReasonCode
	SceneID, SiteID, ModelID                          int64
	At                                                time.Time
}

type CompletedDiscoveryOptions struct {
	Root               string
	MaxFiles, MaxDepth int
	Timeout            time.Duration
	Extensions         map[string]struct{}
}

type CompletedImportPreview struct {
	ID, Root     string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Items        []CompletedRecording
	ScannedCount int
	Truncated    bool
	BoundReason  CompletedBoundReason
}

type CompletedApplyResult struct {
	CandidateID string
	Outcome     CompletedImportOutcome
	Reason      string
}

type completedPreviewState struct {
	preview  CompletedImportPreview
	parser   CompletedFilenameParser
	inFlight int
}

type CompletedImportService struct {
	Authorizer    CompletedImportAuthorizer
	Resolver      CompletedImportResolver
	Store         CompletedImportStore
	Now           func() time.Time
	PreviewTTL    time.Duration
	MaxPreviews   int
	DiscoveryStep func(context.Context, string)
	// AcquireConfiguredRoot obtains a lease proving configuredRoot remains the
	// active server-side root. The returned release function is held through
	// persistence so configuration cannot change underneath Apply.
	AcquireConfiguredRoot func(configuredRoot string) (release func(), err error)

	mu       sync.Mutex
	previews map[string]*completedPreviewState
}

var errCompletedDiscoveryBound = errors.New("completed-recording discovery bound reached")

type completedBoundTransition struct {
	reached bool
	reason  CompletedBoundReason
}

func (b *completedBoundTransition) reach(reason CompletedBoundReason) error {
	if reason != CompletedBoundTimeout && reason != CompletedBoundMaxFiles && reason != CompletedBoundMaxDepth {
		return fmt.Errorf("invalid completed-recording bound reason %q", reason)
	}
	if b.reached {
		if b.reason != reason {
			return fmt.Errorf("completed-recording bound transition already fixed at %q; cannot overwrite with %q", b.reason, reason)
		}
		return nil
	}
	b.reached, b.reason = true, reason
	return nil
}

func finalizeCompletedBoundEnvelope(preview *CompletedImportPreview, bound completedBoundTransition) error {
	reason := CompletedBoundNone
	if bound.reached {
		if bound.reason != CompletedBoundTimeout && bound.reason != CompletedBoundMaxFiles && bound.reason != CompletedBoundMaxDepth {
			return errors.New("completed-recording bound state is corrupt")
		}
		reason = bound.reason
	} else if bound.reason != CompletedBoundNone {
		return errors.New("completed-recording bound state is corrupt")
	}
	preview.Truncated, preview.BoundReason = bound.reached, reason
	return nil
}

const (
	defaultCompletedPreviewCap = 128
	maximumCompletedPreviewCap = 10000
)

func (s *CompletedImportService) previewCap() (int, error) {
	cap := s.MaxPreviews
	if cap == 0 {
		cap = defaultCompletedPreviewCap
	}
	if cap < 1 || cap > maximumCompletedPreviewCap {
		return 0, errors.New("invalid completed-recording preview capacity")
	}
	return cap, nil
}

func (s *CompletedImportService) storePreview(preview CompletedImportPreview, parser CompletedFilenameParser) error {
	cap, err := s.previewCap()
	if err != nil {
		return err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.previews == nil {
		s.previews = map[string]*completedPreviewState{}
	}
	for id, state := range s.previews {
		if state.inFlight == 0 && !now.Before(state.preview.ExpiresAt) {
			delete(s.previews, id)
		}
	}
	for len(s.previews) >= cap {
		oldestID := ""
		var oldest *completedPreviewState
		for id, state := range s.previews {
			if state.inFlight != 0 {
				continue
			}
			if oldest == nil || state.preview.CreatedAt.Before(oldest.preview.CreatedAt) ||
				(state.preview.CreatedAt.Equal(oldest.preview.CreatedAt) && id < oldestID) {
				oldestID, oldest = id, state
			}
		}
		if oldest == nil {
			return errors.New("completed-recording preview capacity is busy")
		}
		delete(s.previews, oldestID)
	}
	s.previews[preview.ID] = &completedPreviewState{preview: preview, parser: parser}
	return nil
}

func (s *CompletedImportService) acquirePreview(previewID string) (*completedPreviewState, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.previews[previewID]
	if !ok {
		return nil, errors.New("preview is missing or stale")
	}
	if !now.Before(state.preview.ExpiresAt) {
		if state.inFlight == 0 {
			delete(s.previews, previewID)
		}
		return nil, errors.New("preview expired")
	}
	state.inFlight++
	return state, nil
}

func (s *CompletedImportService) releasePreview(previewID string, state *completedPreviewState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.previews[previewID]
	if !ok || current != state {
		return
	}
	if current.inFlight > 0 {
		current.inFlight--
	}
	if current.inFlight == 0 && !s.now().Before(current.preview.ExpiresAt) {
		delete(s.previews, previewID)
	}
}

func (s *CompletedImportService) DryRun(ctx context.Context, opts CompletedDiscoveryOptions, parser CompletedFilenameParser) (CompletedImportPreview, error) {
	if s == nil || s.Authorizer == nil || s.Resolver == nil {
		return CompletedImportPreview{}, errors.New("completed-recording import is not configured")
	}
	if _, err := s.previewCap(); err != nil {
		return CompletedImportPreview{}, err
	}
	if err := s.Authorizer.RequireDataAdmin(ctx); err != nil {
		return CompletedImportPreview{}, err
	}
	root, err := canonicalImportRoot(opts.Root)
	if err != nil {
		return CompletedImportPreview{}, err
	}
	if opts.MaxFiles < 1 || opts.MaxFiles > 10000 || opts.MaxDepth < 1 || opts.MaxDepth > 64 || opts.Timeout <= 0 || opts.Timeout > 5*time.Minute {
		return CompletedImportPreview{}, errors.New("invalid discovery bounds")
	}
	if _, err := parser.Parse("contract-probe"); err != nil && strings.Contains(err.Error(), "invalid completed-recording parser") {
		return CompletedImportPreview{}, err
	}
	previewID, err := randomCompletedID()
	if err != nil {
		return CompletedImportPreview{}, err
	}
	now := s.now()
	ttl := s.PreviewTTL
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	if ttl <= 0 || ttl > time.Hour {
		return CompletedImportPreview{}, errors.New("invalid preview TTL")
	}
	preview := CompletedImportPreview{ID: previewID, Root: root, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	configuredRootID := hashCompletedValue(root)
	var bound completedBoundTransition
	walkCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	rootDev, err := lstatDevice(root)
	if err != nil {
		return CompletedImportPreview{}, err
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, contained := containedRelative(root, path)
		if !contained {
			return errors.New("discovery path escaped configured root")
		}
		if rel == "." {
			return nil
		}
		if s.DiscoveryStep != nil {
			s.DiscoveryStep(walkCtx, rel)
		}
		if err := walkCtx.Err(); err != nil {
			if transitionErr := bound.reach(CompletedBoundTimeout); transitionErr != nil {
				return transitionErr
			}
			return errCompletedDiscoveryBound
		}
		depth := strings.Count(filepath.ToSlash(rel), "/") + 1
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		preview.ScannedCount++
		if ignoredCompletedRecordingPath(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			preview.Items = append(preview.Items, rejectedCompleted(previewID, rel, CompletedRejectedSymlink, "symlinks are never followed", now))
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if depth > opts.MaxDepth {
				if transitionErr := bound.reach(CompletedBoundMaxDepth); transitionErr != nil {
					return transitionErr
				}
				return filepath.SkipDir
			}
			dev, err := deviceFromInfo(info)
			if err != nil || dev != rootDev {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if len(preview.Items) >= opts.MaxFiles {
			if transitionErr := bound.reach(CompletedBoundMaxFiles); transitionErr != nil {
				return transitionErr
			}
			return errCompletedDiscoveryBound
		}
		if len(opts.Extensions) > 0 {
			if _, ok := opts.Extensions[strings.ToLower(filepath.Ext(entry.Name()))]; !ok {
				return nil
			}
		}
		item := CompletedRecording{Path: rel, RelativePath: rel, PreviewID: previewID, ConfiguredRootID: configuredRootID, ObservedAt: now, Fingerprint: fingerprintFromInfo(info)}
		parsed, parseErr := parser.Parse(entry.Name())
		if parseErr != nil {
			item.Outcome, item.ReviewReason = CompletedInvalidName, parseErr.Error()
		} else {
			item.Platform, item.Username = parsed.Site, parsed.Handle
			item.CompletedAt, item.Timezone, item.TimePrecision = parsed.ObservedAt, parsed.Timezone, parsed.Precision
			item.ParserVersion = parsed.ParserVersion
			resolution, resolveErr := s.Resolver.ResolveCompletedRecording(walkCtx, root, rel, parsed)
			if resolveErr != nil {
				return resolveErr
			}
			applyResolution(&item, resolution)
		}
		item.CandidateID = completedCandidateID(previewID, item)
		preview.Items = append(preview.Items, item)
		return nil
	})
	if errors.Is(err, context.DeadlineExceeded) {
		if transitionErr := bound.reach(CompletedBoundTimeout); transitionErr != nil {
			return CompletedImportPreview{}, transitionErr
		}
		err = errCompletedDiscoveryBound
	}
	if err != nil && !errors.Is(err, errCompletedDiscoveryBound) {
		return CompletedImportPreview{}, err
	}
	if err := finalizeCompletedBoundEnvelope(&preview, bound); err != nil {
		return CompletedImportPreview{}, err
	}
	if err := s.storePreview(preview, parser); err != nil {
		return CompletedImportPreview{}, err
	}
	return preview, nil
}

func (s *CompletedImportService) Apply(ctx context.Context, actorID, previewID string, selectedIDs []string) ([]CompletedApplyResult, error) {
	return s.apply(ctx, actorID, previewID, selectedIDs, "")
}

// ApplyConfigured additionally binds Apply to the currently approved root.
// The GraphQL boundary uses this form so disabling or changing configuration
// invalidates every outstanding preview before any database transaction.
func (s *CompletedImportService) ApplyConfigured(ctx context.Context, actorID, previewID string, selectedIDs []string, configuredRoot string) ([]CompletedApplyResult, error) {
	root, err := CanonicalCompletedImportRoot(configuredRoot)
	if err != nil {
		return nil, err
	}
	return s.apply(ctx, actorID, previewID, selectedIDs, root)
}

func (s *CompletedImportService) apply(ctx context.Context, actorID, previewID string, selectedIDs []string, configuredRoot string) ([]CompletedApplyResult, error) {
	if s == nil || s.Authorizer == nil || s.Resolver == nil || s.Store == nil {
		return nil, errors.New("completed-recording import is not configured")
	}
	if err := s.Authorizer.RequireDataAdmin(ctx); err != nil {
		return nil, err
	}
	state, err := s.acquirePreview(previewID)
	if err != nil {
		return nil, err
	}
	defer s.releasePreview(previewID, state)
	if configuredRoot != "" && state.preview.Root != configuredRoot {
		return nil, errors.New("configured root changed; run preview again")
	}
	selected := map[string]struct{}{}
	for _, id := range selectedIDs {
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("selected candidate ID is required")
		}
		if _, duplicate := selected[id]; duplicate {
			return nil, errors.New("duplicate selected candidate")
		}
		selected[id] = struct{}{}
	}
	items := make([]CompletedRecording, 0, len(selected))
	for _, item := range state.preview.Items {
		if _, ok := selected[item.CandidateID]; ok {
			items = append(items, item)
			delete(selected, item.CandidateID)
		}
	}
	if len(selected) != 0 {
		return nil, errors.New("selected candidate is not bound to preview")
	}
	for _, item := range items {
		if _, _, err := lstatCompletedPath(state.preview.Root, item.RelativePath); err != nil {
			return nil, errors.New("selected path is no longer a regular no-follow file")
		}
	}
	if configuredRoot != "" {
		if s.AcquireConfiguredRoot == nil {
			return nil, errors.New("configured root lease is unavailable")
		}
		release, err := s.AcquireConfiguredRoot(configuredRoot)
		if err != nil {
			return nil, err
		}
		if release == nil {
			return nil, errors.New("configured root lease is invalid")
		}
		defer release()
	}
	results := make([]CompletedApplyResult, 0, len(items))
	err = s.Store.WithCompletedImportTransaction(ctx, func(txCtx context.Context, tx CompletedImportTx) error {
		// Revalidate after the authoritative write transaction has begun. This
		// prevents a persisted role/status change from racing the boundary check
		// and the first import/audit write.
		if err := s.Authorizer.RequireDataAdmin(txCtx); err != nil {
			return err
		}
		for _, item := range items {
			fresh, staleReason := s.revalidate(txCtx, *state, item)
			result := CompletedApplyResult{CandidateID: item.CandidateID}
			if staleReason != "" {
				result.Outcome, result.Reason = CompletedChanged, staleReason
			} else if fresh.Outcome != CompletedExactReady {
				result.Outcome, result.Reason = CompletedSkipped, fresh.ReviewReason
			} else {
				applied, err := tx.LinkCamShowMetadata(txCtx, fresh)
				if err != nil {
					return err
				}
				if applied {
					result.Outcome = CompletedApplied
				} else {
					result.Outcome = CompletedAlreadyApplied
				}
			}
			auditOutcome := result.Outcome
			if fresh.Outcome == CompletedReviewRequired {
				auditOutcome = CompletedReviewRequired
			}
			audit := CompletedImportAudit{
				ActorID: actorID, PreviewID: previewID, CandidateID: item.CandidateID,
				RelativePathHash: hashCompletedValue(item.RelativePath),
				Outcome:          string(auditOutcome), Reason: result.Reason, ReviewCode: fresh.ReviewCode,
				SceneID: fresh.SceneID, SiteID: fresh.SiteID, ModelID: fresh.ModelID, At: s.now(),
			}
			if err := tx.WriteCompletedImportAudit(txCtx, audit); err != nil {
				return err
			}
			results = append(results, result)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (s *CompletedImportService) revalidate(ctx context.Context, state completedPreviewState, item CompletedRecording) (CompletedRecording, string) {
	root, err := canonicalImportRoot(state.preview.Root)
	if err != nil || root != state.preview.Root {
		return item, "configured root changed or is no longer canonical"
	}
	path, info, err := lstatCompletedPath(root, item.RelativePath)
	if err != nil {
		return item, "path contains a symlink or file is missing, unreadable, or no longer regular"
	}
	if fingerprintFromInfo(info) != item.Fingerprint {
		return item, "stat fingerprint changed"
	}
	parsed, err := state.parser.Parse(filepath.Base(path))
	if err != nil {
		return item, "filename no longer parses"
	}
	resolution, err := s.Resolver.ResolveCompletedRecording(ctx, root, item.RelativePath, parsed)
	if err != nil {
		return item, "resolution failed"
	}
	fresh := item
	applyResolution(&fresh, resolution)
	if fresh.SceneID != item.SceneID || fresh.SiteID != item.SiteID || fresh.ModelID != item.ModelID || fresh.MatchState != item.MatchState || fresh.Outcome != item.Outcome || fresh.ReviewCode != item.ReviewCode {
		return fresh, "scene or identity resolution changed"
	}
	return fresh, ""
}

func ignoredCompletedRecordingPath(relativePath string) bool {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(relativePath)), "/")
	for _, part := range parts {
		lower := strings.ToLower(part)
		if strings.HasPrefix(part, ".") || strings.HasSuffix(lower, "~") ||
			strings.Contains(lower, ".part.") || strings.Contains(lower, ".partial.") ||
			strings.Contains(lower, ".tmp.") || strings.Contains(lower, ".temp.") ||
			strings.Contains(lower, ".crdownload.") || strings.HasSuffix(lower, ".part") ||
			strings.HasSuffix(lower, ".partial") || strings.HasSuffix(lower, ".tmp") ||
			strings.HasSuffix(lower, ".temp") || strings.HasSuffix(lower, ".crdownload") {
			return true
		}
	}
	return false
}

// lstatCompletedPath checks every relative component with Lstat and rejects
// symlinks present at each check. Pathname components can still be replaced
// between checks; this bounded residual cannot expose media content because
// the import is metadata-only and never opens or modifies the media file.
func lstatCompletedPath(root, relativePath string) (string, os.FileInfo, error) {
	rel := filepath.Clean(relativePath)
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil, errors.New("path escaped configured root")
	}
	current := root
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", nil, errors.New("invalid path component")
		}
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil {
			return "", nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, errors.New("path component is a symlink")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", nil, errors.New("parent component is not a directory")
		}
		if i == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return "", nil, errors.New("path is not a regular file")
			}
			return current, info, nil
		}
	}
	return "", nil, errors.New("invalid path")
}

func canonicalImportRoot(root string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || strings.TrimSpace(root) == "" {
		return "", errors.New("configured root is required")
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) {
		return "", errors.New("filesystem root is not allowed")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil || resolved != abs {
		return "", errors.New("configured root contains a symlink")
	}
	info, err := os.Lstat(abs)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("configured root must be a non-symlink directory")
	}
	return abs, nil
}

// CanonicalCompletedImportRoot exposes the import service's fail-closed root
// validation to configuration boundaries without duplicating path rules.
func CanonicalCompletedImportRoot(root string) (string, error) {
	return canonicalImportRoot(root)
}

func containedRelative(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

func lstatDevice(path string) (uint64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	return deviceFromInfo(info)
}

func deviceFromInfo(info os.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("filesystem stat identity unavailable")
	}
	return uint64(stat.Dev), nil
}

func fingerprintFromInfo(info os.FileInfo) CompletedStatFingerprint {
	ret := CompletedStatFingerprint{Size: info.Size(), ModTimeNS: info.ModTime().UnixNano(), Mode: uint32(info.Mode())}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		ret.Device, ret.Inode = uint64(stat.Dev), uint64(stat.Ino)
	}
	return ret
}

func rejectedCompleted(previewID, rel string, outcome CompletedImportOutcome, reason string, now time.Time) CompletedRecording {
	item := CompletedRecording{Path: rel, RelativePath: rel, PreviewID: previewID, Outcome: outcome, ReviewReason: reason, ObservedAt: now}
	item.CandidateID = completedCandidateID(previewID, item)
	return item
}

func applyResolution(item *CompletedRecording, value CompletedResolution) {
	item.SceneID, item.SiteID, item.ModelID = value.SceneID, value.SiteID, value.ModelID
	item.MatchState, item.Outcome, item.ReviewReason, item.ReviewCode = value.MatchState, value.Outcome, value.ReviewReason, value.ReviewCode
}

func (s *CompletedImportService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func randomCompletedID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func completedCandidateID(previewID string, item CompletedRecording) string {
	return hashCompletedValue(fmt.Sprintf("%s\x00%s\x00%v\x00%s", previewID, item.RelativePath, item.Fingerprint, item.ParserVersion))
}

func hashCompletedValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

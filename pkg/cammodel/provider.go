package cammodel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// ProfileObservation is a metadata candidate returned by a discovery provider.
type ProfileObservation struct {
	Provider, EvidenceKey, Platform, Username string
	ProviderRecordID, SourceURL, ImageURL     *string
	ObservedAt                                time.Time
	PayloadJSON                               string
	Confidence                                *float64
}

// DiscoveryProvider searches external profile metadata. Implementations do not
// persist data; the caller explicitly selects which result to apply.
type DiscoveryProvider interface {
	Key() string
	Discover(context.Context, string) ([]ProfileObservation, error)
}

type PresenceState string

const (
	PresenceUnknown PresenceState = "UNKNOWN"
	PresenceOffline PresenceState = "OFFLINE"
	PresenceOnline  PresenceState = "ONLINE"

	DefaultPresenceMaxBatch = 50
	DefaultPresenceMaxTTL   = 15 * time.Minute
	DefaultPresenceInterval = time.Second
)

var (
	ErrInvalidPresenceTarget      = errors.New("invalid presence target")
	ErrInvalidPresenceObservation = errors.New("invalid presence observation")
	ErrPresenceBatchTooLarge      = errors.New("presence batch exceeds configured limit")
)

// PresenceTarget deliberately contains no local model, account, or identity
// identifier. A provider may look up only an exact Site plus normalized handle.
type PresenceTarget struct {
	Site, NormalizedHandle string
}

func NewPresenceTarget(site, handle string) PresenceTarget {
	return PresenceTarget{
		Site:             strings.ToLower(strings.TrimSpace(site)),
		NormalizedHandle: strings.ToLower(strings.TrimSpace(handle)),
	}
}

func (v PresenceTarget) Validate() error {
	if v.Site == "" || v.NormalizedHandle == "" || v != NewPresenceTarget(v.Site, v.NormalizedHandle) {
		return ErrInvalidPresenceTarget
	}
	return nil
}

func (v PresenceTarget) key() string { return v.Site + "\x00" + v.NormalizedHandle }

// PresenceObservation is a short-lived, read-only provider assertion. EvidenceKey
// is a bounded provider reference/hash, never a local identity or merge key.
type PresenceObservation struct {
	PresenceTarget
	State                 PresenceState
	ObservedAt, ExpiresAt time.Time
	Provider, EvidenceKey string
	SourceURL             *string
}

func (v PresenceObservation) EffectiveState(now time.Time) PresenceState {
	if !now.Before(v.ExpiresAt) {
		return PresenceUnknown
	}
	return v.State
}

func ValidatePresenceObservation(v PresenceObservation, maxTTL time.Duration) error {
	if err := v.PresenceTarget.Validate(); err != nil {
		return errors.Join(ErrInvalidPresenceObservation, err)
	}
	if v.State != PresenceUnknown && v.State != PresenceOffline && v.State != PresenceOnline {
		return ErrInvalidPresenceObservation
	}
	if v.ObservedAt.IsZero() || !v.ExpiresAt.After(v.ObservedAt) || maxTTL <= 0 || v.ExpiresAt.Sub(v.ObservedAt) > maxTTL {
		return ErrInvalidPresenceObservation
	}
	if strings.TrimSpace(v.Provider) == "" || strings.TrimSpace(v.EvidenceKey) == "" || len(v.EvidenceKey) > 256 {
		return ErrInvalidPresenceObservation
	}
	return nil
}

// PresenceProvider isolates authoritative online-now lookup from discovery and
// profile storage. Implementations have no identity or persistence capability.
type PresenceProvider interface {
	Key() string
	PollPresence(context.Context, []PresenceTarget) ([]PresenceObservation, error)
}

// FixtureOnlyPresenceProvider is the production-safe fallback while no Site
// has a qualified public status contract. It performs no I/O and can only
// return short-lived UNKNOWN observations for the exact requested targets.
type FixtureOnlyPresenceProvider struct {
	Now func() time.Time
}

func (FixtureOnlyPresenceProvider) Key() string { return "fixture-only-unknown" }

func (p FixtureOnlyPresenceProvider) PollPresence(ctx context.Context, targets []PresenceTarget) ([]PresenceObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	ret := make([]PresenceObservation, len(targets))
	for i, target := range targets {
		if err := target.Validate(); err != nil {
			return nil, err
		}
		ret[i] = PresenceObservation{
			PresenceTarget: target,
			State:          PresenceUnknown,
			ObservedAt:     now,
			ExpiresAt:      now.Add(time.Minute),
			Provider:       p.Key(),
			EvidenceKey:    "no-qualified-provider",
		}
	}
	return ret, nil
}

// PresenceService bounds batch size, TTL and poll rate. Missing, duplicate, or
// unrequested provider results fail closed instead of being inferred OFFLINE.
type PresenceService struct {
	Provider    PresenceProvider
	MaxBatch    int
	MaxTTL      time.Duration
	MinInterval time.Duration

	rateMu   sync.Mutex
	nextPoll time.Time
	gateOnce sync.Once
	callGate chan struct{}
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
}

func (s *PresenceService) Poll(ctx context.Context, targets []PresenceTarget) ([]PresenceObservation, error) {
	if s == nil || s.Provider == nil || strings.TrimSpace(s.Provider.Key()) == "" {
		return nil, errors.New("online-now provider is not configured")
	}
	maxBatch := s.MaxBatch
	if maxBatch == 0 {
		maxBatch = DefaultPresenceMaxBatch
	}
	maxTTL := s.MaxTTL
	if maxTTL == 0 {
		maxTTL = DefaultPresenceMaxTTL
	}
	interval := s.MinInterval
	if interval == 0 {
		interval = DefaultPresenceInterval
	}
	if maxBatch < 1 || maxBatch > DefaultPresenceMaxBatch || maxTTL <= 0 || maxTTL > DefaultPresenceMaxTTL || interval < 0 {
		return nil, errors.New("invalid presence service limits")
	}
	if len(targets) == 0 {
		return []PresenceObservation{}, nil
	}
	if len(targets) > maxBatch {
		return nil, ErrPresenceBatchTooLarge
	}
	wanted := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if err := target.Validate(); err != nil {
			return nil, err
		}
		if _, exists := wanted[target.key()]; exists {
			return nil, ErrInvalidPresenceTarget
		}
		wanted[target.key()] = struct{}{}
	}
	if err := s.awaitRateLimit(ctx, interval); err != nil {
		return nil, err
	}
	if err := s.acquireProvider(ctx); err != nil {
		return nil, err
	}
	values, err := s.Provider.PollPresence(ctx, targets)
	s.releaseProvider()
	if err != nil {
		return nil, err
	}
	if len(values) != len(targets) {
		return nil, ErrInvalidPresenceObservation
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := ValidatePresenceObservation(value, maxTTL); err != nil || value.Provider != s.Provider.Key() {
			return nil, ErrInvalidPresenceObservation
		}
		key := value.PresenceTarget.key()
		if _, ok := wanted[key]; !ok {
			return nil, ErrInvalidPresenceObservation
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, ErrInvalidPresenceObservation
		}
		seen[key] = struct{}{}
	}
	return values, nil
}

func (s *PresenceService) acquireProvider(ctx context.Context) error {
	s.gateOnce.Do(func() { s.callGate = make(chan struct{}, 1) })
	select {
	case s.callGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *PresenceService) releaseProvider() { <-s.callGate }

func (s *PresenceService) awaitRateLimit(ctx context.Context, interval time.Duration) error {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	wait := waitForPresenceInterval
	if s.wait != nil {
		wait = s.wait
	}
	s.rateMu.Lock()
	current := now()
	delay := s.nextPoll.Sub(current)
	if delay < 0 {
		delay = 0
	}
	s.nextPoll = current.Add(delay).Add(interval)
	s.rateMu.Unlock()
	return wait(ctx, delay)
}

func waitForPresenceInterval(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type CompletedRecording struct {
	Path, RelativePath, Platform, Username string
	CandidateID, PreviewID, ParserVersion  string
	ConfiguredRootID                       string
	CompletedAt, ObservedAt                time.Time
	Timezone                               string
	TimePrecision                          CompletedTimePrecision
	Fingerprint                            CompletedStatFingerprint
	SceneID, SiteID, ModelID               int64
	MatchState                             CompletedAliasMatchState
	Outcome                                CompletedImportOutcome
	ReviewReason                           string
	ReviewCode                             CompletedReviewReasonCode
	SidecarJSON                            *string
}

// CompletedRecordingProvider is the Lite/import boundary. It deliberately has
// no start/stop recording or monitoring-control operation in CS-04.
type CompletedRecordingProvider interface {
	Key() string
	ListCompleted(context.Context) ([]CompletedRecording, error)
}

package cammodel

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type discoveryFixture struct{}

func (discoveryFixture) Key() string { return "discovery" }
func (discoveryFixture) Discover(context.Context, string) ([]ProfileObservation, error) {
	return nil, nil
}

type presenceFixture struct {
	values []PresenceObservation
	err    error
	calls  int
}

func (p *presenceFixture) Key() string { return "presence-fixture" }
func (p *presenceFixture) PollPresence(ctx context.Context, targets []PresenceTarget) ([]PresenceObservation, error) {
	p.calls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.err != nil {
		return nil, p.err
	}
	return append([]PresenceObservation(nil), p.values...), nil
}

type completedFixture struct{}

func (completedFixture) Key() string { return "completed" }
func (completedFixture) ListCompleted(context.Context) ([]CompletedRecording, error) {
	return nil, nil
}

func TestProviderBoundariesRemainSeparateAndHaveNoRecordingControl(t *testing.T) {
	var discovery DiscoveryProvider = discoveryFixture{}
	var presence PresenceProvider = &presenceFixture{}
	var completed CompletedRecordingProvider = completedFixture{}
	if discovery.Key() == presence.Key() || presence.Key() == completed.Key() {
		t.Fatal("provider fixtures are not distinct")
	}
}

func presenceValue(target PresenceTarget, state PresenceState, observed time.Time) PresenceObservation {
	return PresenceObservation{
		PresenceTarget: target,
		State:          state,
		ObservedAt:     observed,
		ExpiresAt:      observed.Add(5 * time.Minute),
		Provider:       "presence-fixture",
		EvidenceKey:    "fixture:" + target.Site + ":" + target.NormalizedHandle,
	}
}

func TestPresenceStateExpiresToUnknown(t *testing.T) {
	observed := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	for _, state := range []PresenceState{PresenceOnline, PresenceOffline, PresenceUnknown} {
		value := presenceValue(NewPresenceTarget("CB", " Alice "), state, observed)
		if got := value.EffectiveState(value.ExpiresAt.Add(-time.Nanosecond)); got != state {
			t.Fatalf("%s before expiry became %s", state, got)
		}
		if got := value.EffectiveState(value.ExpiresAt); got != PresenceUnknown {
			t.Fatalf("%s at expiry became %s, want UNKNOWN", state, got)
		}
	}
}

func TestPresenceServiceReturnsOnlyExactAuthoritativeTargets(t *testing.T) {
	observed := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	targets := []PresenceTarget{
		NewPresenceTarget("cb", "Alice"),
		NewPresenceTarget("mfc", "Bob"),
		NewPresenceTarget("lj", "Carol"),
	}
	provider := &presenceFixture{values: []PresenceObservation{
		presenceValue(targets[0], PresenceOnline, observed),
		presenceValue(targets[1], PresenceOffline, observed),
		presenceValue(targets[2], PresenceUnknown, observed),
	}}
	service := &PresenceService{Provider: provider, MinInterval: time.Nanosecond}
	values, err := service.Poll(context.Background(), targets)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, provider.values) {
		t.Fatalf("got %#v want %#v", values, provider.values)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls=%d", provider.calls)
	}
}

func TestPresenceServiceFailsClosedOnInvalidProviderResults(t *testing.T) {
	observed := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	target := NewPresenceTarget("cb", "alice")
	other := NewPresenceTarget("mfc", "bob")
	tests := map[string][]PresenceObservation{
		"missing":     {},
		"duplicate":   {presenceValue(target, PresenceOnline, observed), presenceValue(target, PresenceOnline, observed)},
		"unrequested": {presenceValue(other, PresenceOnline, observed)},
		"wrong provider": {func() PresenceObservation {
			v := presenceValue(target, PresenceOnline, observed)
			v.Provider = "other"
			return v
		}()},
		"invalid state": {func() PresenceObservation {
			v := presenceValue(target, PresenceOnline, observed)
			v.State = "RECENT"
			return v
		}()},
		"oversized ttl": {func() PresenceObservation {
			v := presenceValue(target, PresenceOnline, observed)
			v.ExpiresAt = observed.Add(DefaultPresenceMaxTTL + time.Second)
			return v
		}()},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			input := []PresenceTarget{target}
			if name == "duplicate" {
				input = []PresenceTarget{target, other}
			}
			service := &PresenceService{Provider: &presenceFixture{values: values}, MinInterval: time.Nanosecond}
			if _, err := service.Poll(context.Background(), input); !errors.Is(err, ErrInvalidPresenceObservation) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestPresenceServiceRejectsBadTargetsAndBoundsBatch(t *testing.T) {
	service := &PresenceService{Provider: &presenceFixture{}, MaxBatch: 1, MinInterval: time.Nanosecond}
	if _, err := service.Poll(context.Background(), []PresenceTarget{{Site: "CB", NormalizedHandle: "Alice"}}); !errors.Is(err, ErrInvalidPresenceTarget) {
		t.Fatalf("non-normalized target: %v", err)
	}
	target := NewPresenceTarget("cb", "alice")
	if _, err := service.Poll(context.Background(), []PresenceTarget{target, target}); !errors.Is(err, ErrPresenceBatchTooLarge) {
		t.Fatalf("oversized batch: %v", err)
	}
	service.MaxBatch = 2
	if _, err := service.Poll(context.Background(), []PresenceTarget{target, target}); !errors.Is(err, ErrInvalidPresenceTarget) {
		t.Fatalf("duplicate target: %v", err)
	}
}

func TestPresencePollingIsRateLimitedAndCancellable(t *testing.T) {
	observed := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	target := NewPresenceTarget("cb", "alice")
	provider := &presenceFixture{values: []PresenceObservation{presenceValue(target, PresenceOnline, observed)}}
	var delays []time.Duration
	service := &PresenceService{
		Provider:    provider,
		MinInterval: 3 * time.Second,
		now:         func() time.Time { return observed },
		wait: func(ctx context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			if delay == 0 {
				return nil
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}
	if _, err := service.Poll(context.Background(), []PresenceTarget{target}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Poll(ctx, []PresenceTarget{target}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(delays, []time.Duration{0, 3 * time.Second}) {
		t.Fatalf("delays=%v", delays)
	}
	if provider.calls != 1 {
		t.Fatalf("provider called after canceled rate wait: %d", provider.calls)
	}
}

type blockingPresenceFixture struct {
	value   PresenceObservation
	entered chan struct{}
	release chan struct{}
}

func (p *blockingPresenceFixture) Key() string { return "presence-fixture" }
func (p *blockingPresenceFixture) PollPresence(ctx context.Context, _ []PresenceTarget) ([]PresenceObservation, error) {
	p.entered <- struct{}{}
	select {
	case <-p.release:
		return []PresenceObservation{p.value}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestPresencePollingBoundsConcurrentProviderCalls(t *testing.T) {
	observed := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	target := NewPresenceTarget("cb", "alice")
	provider := &blockingPresenceFixture{
		value:   presenceValue(target, PresenceOnline, observed),
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	service := &PresenceService{Provider: provider, MinInterval: time.Nanosecond}
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Poll(context.Background(), []PresenceTarget{target})
		firstDone <- err
	}()
	<-provider.entered

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Poll(ctx, []PresenceTarget{target})
		secondDone <- err
	}()
	cancel()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued poll got %v", err)
	}
	select {
	case <-provider.entered:
		t.Fatal("second provider call ran concurrently")
	default:
	}
	close(provider.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestPresenceServiceHasNoConfiguredProviderAndPropagatesProviderError(t *testing.T) {
	if _, err := (*PresenceService)(nil).Poll(context.Background(), nil); err == nil {
		t.Fatal("missing provider accepted")
	}
	want := errors.New("provider unavailable")
	target := NewPresenceTarget("cb", "alice")
	service := &PresenceService{Provider: &presenceFixture{err: want}, MinInterval: time.Nanosecond}
	if _, err := service.Poll(context.Background(), []PresenceTarget{target}); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}

func TestFixtureOnlyPresenceIsBoundedUnknownAndHasNoNetworkOrIdentityAuthority(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	provider := FixtureOnlyPresenceProvider{Now: func() time.Time { return now }}
	service := &PresenceService{Provider: provider, MaxBatch: 2, MinInterval: time.Nanosecond}
	targets := []PresenceTarget{NewPresenceTarget("cb", "alice"), NewPresenceTarget("mfc", "bob")}
	values, err := service.Poll(context.Background(), targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != len(targets) {
		t.Fatalf("values=%#v", values)
	}
	for i, value := range values {
		if value.PresenceTarget != targets[i] || value.State != PresenceUnknown || value.Provider != provider.Key() ||
			value.EvidenceKey != "no-qualified-provider" || value.SourceURL != nil || value.ObservedAt != now || value.ExpiresAt != now.Add(time.Minute) {
			t.Fatalf("unsafe fixture-only observation=%#v", value)
		}
	}
	if _, err := service.Poll(context.Background(), append(targets, NewPresenceTarget("sc", "carol"))); !errors.Is(err, ErrPresenceBatchTooLarge) {
		t.Fatalf("unbounded fixture-only lookup: %v", err)
	}
	providerType := reflect.TypeOf(provider)
	for _, forbidden := range []string{"Client", "Cookie", "Credential", "Repository", "Recorder", "HTTP"} {
		if _, ok := providerType.FieldByName(forbidden); ok {
			t.Fatalf("fixture-only provider gained %s authority", forbidden)
		}
	}
}

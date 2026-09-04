package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	managerconfig "github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/cammodel"
	"github.com/stashapp/stash/pkg/txn"
)

type completedGraphResolver struct {
	err                       error
	requireTransactionContext bool
}

func (r *completedGraphResolver) ResolveCompletedRecording(ctx context.Context, _ string, rel string, parsed cammodel.CompletedParsedName) (cammodel.CompletedResolution, error) {
	if r.requireTransactionContext {
		if marked, _ := ctx.Value(completedGraphTransactionContextKey{}).(bool); !marked {
			return cammodel.CompletedResolution{}, errors.New("identity revalidation used outer context")
		}
	}
	if r.err != nil {
		return cammodel.CompletedResolution{}, r.err
	}
	if strings.Contains(rel, "historical") {
		return cammodel.CompletedResolution{SceneID: 1, SiteID: 2, ModelID: 3, MatchState: cammodel.CompletedAliasHistorical, Outcome: cammodel.CompletedReviewRequired, ReviewReason: "unique historical alias requires identity review", ReviewCode: cammodel.CompletedReviewAliasReused}, nil
	}
	sceneID := int64(1)
	if strings.Contains(rel, "rollback") {
		sceneID = 4
	}
	return cammodel.CompletedResolution{SceneID: sceneID, SiteID: 2, ModelID: 3, MatchState: cammodel.CompletedAliasCurrent, Outcome: cammodel.CompletedExactReady}, nil
}

type completedGraphStore struct {
	links             map[string]struct{}
	audits            []cammodel.CompletedImportAudit
	failAudit         bool
	beforeTransaction func()
}

type completedGraphTransactionContextKey struct{}

type completedGraphAuthorizer struct {
	delegate                  completedImportContextAuthorizer
	requireTransactionContext bool
	outerAllowed              bool
}

func (a *completedGraphAuthorizer) RequireDataAdmin(ctx context.Context) error {
	if a.requireTransactionContext {
		marked, _ := ctx.Value(completedGraphTransactionContextKey{}).(bool)
		if !marked {
			if !a.outerAllowed {
				return errors.New("persisted authorization used outer context")
			}
			a.outerAllowed = false
		}
	}
	return a.delegate.RequireDataAdmin(ctx)
}

type completedGraphTx struct {
	links     map[string]struct{}
	audits    []cammodel.CompletedImportAudit
	failAudit bool
}

func (s *completedGraphStore) WithCompletedImportTransaction(ctx context.Context, fn func(context.Context, cammodel.CompletedImportTx) error) error {
	if s.beforeTransaction != nil {
		s.beforeTransaction()
	}
	links := map[string]struct{}{}
	for key := range s.links {
		links[key] = struct{}{}
	}
	tx := &completedGraphTx{links: links, audits: append([]cammodel.CompletedImportAudit(nil), s.audits...), failAudit: s.failAudit}
	txCtx := context.WithValue(ctx, completedGraphTransactionContextKey{}, true)
	if err := fn(txCtx, tx); err != nil {
		return err
	}
	s.links, s.audits = tx.links, tx.audits
	return nil
}

func (t *completedGraphTx) LinkCamShowMetadata(ctx context.Context, item cammodel.CompletedRecording) (bool, error) {
	if marked, _ := ctx.Value(completedGraphTransactionContextKey{}).(bool); !marked {
		return false, errors.New("metadata link used outer context")
	}
	key := item.RelativePath
	if _, ok := t.links[key]; ok {
		return false, nil
	}
	t.links[key] = struct{}{}
	return true, nil
}

func (t *completedGraphTx) WriteCompletedImportAudit(ctx context.Context, audit cammodel.CompletedImportAudit) error {
	if marked, _ := ctx.Value(completedGraphTransactionContextKey{}).(bool); !marked {
		return errors.New("audit write used outer context")
	}
	if t.failAudit {
		return errors.New("injected audit failure")
	}
	t.audits = append(t.audits, audit)
	return nil
}

func completedGraphInput(root string) CompletedRecordingPreviewInput {
	_ = root
	return CompletedRecordingPreviewInput{
		MaxFiles: 20, MaxDepth: 3, TimeoutMs: 1000, Extensions: []string{".mp4"},
		FilenamePattern: `^(?P<site>[A-Za-z]+)-(?P<model>[A-Za-z]+)-(?P<timestamp>[0-9]{8}-[0-9]{6})[.]mp4$`,
		TimestampLayout: "20060102-150405", Timezone: "UTC",
		Precision: CompletedRecordingPrecisionSecond, ParserVersion: "graphql-test-v1",
	}
}

func configureCompletedRecordingTest(t *testing.T, root string, enabled bool) *managerconfig.Config {
	t.Helper()
	cfg := managerconfig.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.SetInterface(managerconfig.Stash, managerconfig.StashConfigs{&managerconfig.StashConfig{Path: root}})
	cfg.SetCompletedRecordingImportConfig(managerconfig.CompletedRecordingImportConfig{Enabled: enabled, Root: root})
	return cfg
}

func TestCompletedRecordingResolversAuthorizationStructuredReviewIdempotencyAndRollback(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "completed-admin", authz.RoleAdmin)
	user := createResolverUser(t, database, "completed-user", authz.RoleUser)
	reduced := admin
	reduced.TokenScopes = map[authz.Capability]struct{}{authz.LibraryRead: {}}
	root := t.TempDir()
	configureCompletedRecordingTest(t, root, true)
	for _, name := range []string{
		"CB-current-20260721-120000.mp4",
		"CB-historical-20260720-120000.mp4",
		"CB-rollback-20260719-120000.mp4",
	} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := &completedGraphStore{links: map[string]struct{}{}}
	resolverImpl := &completedGraphResolver{}
	authorizer := &completedGraphAuthorizer{delegate: completedImportContextAuthorizer{database: database}}
	service := &cammodel.CompletedImportService{
		Authorizer: authorizer,
		Resolver:   resolverImpl,
		Store:      store,
		Now:        func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
	resolver := &Resolver{database: database, completedImportService: service}
	mutation := &mutationResolver{Resolver: resolver}

	for name, ctx := range map[string]context.Context{
		"anonymous": context.Background(),
		"user":      authz.WithPrincipal(context.Background(), user),
		"reduced":   authz.WithPrincipal(context.Background(), reduced),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mutation.CompletedRecordingPreview(ctx, completedGraphInput(root)); err == nil {
				t.Fatal("preview allowed")
			}
			if _, err := mutation.CompletedRecordingApply(ctx, CompletedRecordingApplyInput{PreviewID: strings.Repeat("a", 32)}); err == nil {
				t.Fatal("apply allowed")
			}
		})
	}

	adminCtx := authz.WithPrincipal(context.Background(), admin)
	preview, err := mutation.CompletedRecordingPreview(adminCtx, completedGraphInput(root))
	if err != nil {
		t.Fatal(err)
	}
	if preview.PreviewID == "" || preview.Truncated || preview.ScannedCount != 3 || len(preview.Items) != 3 {
		t.Fatalf("preview=%#v", preview)
	}
	resolverImpl.requireTransactionContext = true
	var current, historical, rollback *CompletedRecordingCandidate
	for _, item := range preview.Items {
		switch {
		case strings.Contains(item.RelativePath, "current"):
			current = item
		case strings.Contains(item.RelativePath, "historical"):
			historical = item
		case strings.Contains(item.RelativePath, "rollback"):
			rollback = item
		}
	}
	if historical == nil || historical.Outcome != CompletedRecordingOutcomeReviewRequired ||
		historical.ReviewCode == nil || *historical.ReviewCode != CompletedRecordingReviewReasonHistoricalAliasReused {
		t.Fatalf("historical=%#v", historical)
	}
	reviewResult, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{PreviewID: preview.PreviewID, SelectedCandidateIDs: []string{historical.CandidateID}})
	if err != nil || len(reviewResult) != 1 || reviewResult[0].Outcome != CompletedRecordingOutcomeSkipped {
		t.Fatalf("review apply=%#v err=%v", reviewResult, err)
	}
	if len(store.links) != 0 || len(store.audits) != 1 || store.audits[0].Outcome != string(cammodel.CompletedReviewRequired) ||
		store.audits[0].ReviewCode != cammodel.CompletedReviewAliasReused {
		t.Fatalf("review persistence links=%d audits=%#v", len(store.links), store.audits)
	}

	first, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{PreviewID: preview.PreviewID, SelectedCandidateIDs: []string{current.CandidateID}})
	if err != nil || first[0].Outcome != CompletedRecordingOutcomeApplied {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	replay, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{PreviewID: preview.PreviewID, SelectedCandidateIDs: []string{current.CandidateID}})
	if err != nil || replay[0].Outcome != CompletedRecordingOutcomeAlreadyApplied || len(store.links) != 1 {
		t.Fatalf("replay=%#v err=%v links=%d", replay, err, len(store.links))
	}

	beforeAudits := len(store.audits)
	store.failAudit = true
	if _, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{PreviewID: preview.PreviewID, SelectedCandidateIDs: []string{rollback.CandidateID}}); err == nil {
		t.Fatal("audit failure accepted")
	}
	if len(store.links) != 1 || len(store.audits) != beforeAudits {
		t.Fatalf("resolver rollback links=%d audits=%d", len(store.links), len(store.audits))
	}
	store.failAudit = false
	recovered, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{PreviewID: preview.PreviewID, SelectedCandidateIDs: []string{rollback.CandidateID}})
	if err != nil || recovered[0].Outcome != CompletedRecordingOutcomeApplied || len(store.links) != 2 {
		t.Fatalf("recovered=%#v err=%v links=%d", recovered, err, len(store.links))
	}
}

func TestCompletedRecordingApplyRevalidatesPersistedAdminInsideWriteTransaction(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "completed-race-admin", authz.RoleAdmin)
	_ = createResolverUser(t, database, "completed-race-retained-admin", authz.RoleAdmin)
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	root := t.TempDir()
	configureCompletedRecordingTest(t, root, true)
	if err := os.WriteFile(filepath.Join(root, "CB-current-20260721-120000.mp4"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &completedGraphStore{links: map[string]struct{}{}}
	resolverImpl := &completedGraphResolver{}
	authorizer := &completedGraphAuthorizer{delegate: completedImportContextAuthorizer{database: database}}
	service := &cammodel.CompletedImportService{
		Authorizer: authorizer,
		Resolver:   resolverImpl,
		Store:      store,
	}
	resolver := &Resolver{database: database, completedImportService: service}
	mutation := &mutationResolver{Resolver: resolver}
	preview, err := mutation.CompletedRecordingPreview(adminCtx, completedGraphInput(root))
	if err != nil || len(preview.Items) != 1 {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	resolverImpl.requireTransactionContext = true
	authorizer.requireTransactionContext = true
	authorizer.outerAllowed = true
	adminID, err := strconv.ParseInt(admin.UserID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}

	store.beforeTransaction = func() {
		store.beforeTransaction = nil
		if err := txn.WithTxn(context.Background(), database, func(txCtx context.Context) error {
			return database.User.SetAccess(txCtx, adminID, authz.RoleUser, authz.StatusActive)
		}); err != nil {
			t.Fatalf("downgrade persisted Admin: %v", err)
		}
	}
	if _, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{
		PreviewID: preview.PreviewID, SelectedCandidateIDs: []string{preview.Items[0].CandidateID},
	}); err == nil {
		t.Fatal("persisted role downgrade raced past Apply authorization")
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatalf("unauthorized Apply persisted links=%d audits=%d", len(store.links), len(store.audits))
	}
}

func TestCompletedRecordingResolverBoundsValidationAndRedactedErrors(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "completed-errors-admin", authz.RoleAdmin)
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	root := t.TempDir()
	configureCompletedRecordingTest(t, root, true)
	if err := os.WriteFile(filepath.Join(root, "CB-current-20260721-120000.mp4"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	resolverImpl := &completedGraphResolver{err: errors.New("lookup failed for /private/identity/alice")}
	service := &cammodel.CompletedImportService{Authorizer: completedImportContextAuthorizer{database: database}, Resolver: resolverImpl, Store: &completedGraphStore{links: map[string]struct{}{}}}
	resolver := &Resolver{database: database, completedImportService: service}
	mutation := &mutationResolver{Resolver: resolver}
	input := completedGraphInput(root)
	if _, err := mutation.CompletedRecordingPreview(adminCtx, input); err == nil || strings.Contains(err.Error(), "private") || err.Error() != "completed-recording operation failed" {
		t.Fatalf("unredacted error=%v", err)
	}
	input = completedGraphInput(root)
	input.MaxFiles = 10001
	if _, err := mutation.CompletedRecordingPreview(adminCtx, input); !errors.Is(err, ErrInput) {
		t.Fatalf("invalid bound=%v", err)
	}
	input = completedGraphInput(root)
	input.FilenamePattern = "["
	if _, err := mutation.CompletedRecordingPreview(adminCtx, input); !errors.Is(err, ErrInput) {
		t.Fatalf("invalid regex=%v", err)
	}

	timeoutService := &cammodel.CompletedImportService{
		Authorizer: completedImportContextAuthorizer{database: database}, Resolver: &completedGraphResolver{},
		Store:         &completedGraphStore{links: map[string]struct{}{}},
		DiscoveryStep: func(ctx context.Context, _ string) { <-ctx.Done() },
	}
	timeoutResolver := &mutationResolver{Resolver: &Resolver{database: database, completedImportService: timeoutService}}
	input = completedGraphInput(root)
	input.TimeoutMs = 1
	bounded, err := timeoutResolver.CompletedRecordingPreview(adminCtx, input)
	if err != nil || bounded.PreviewID == "" || !bounded.Truncated || bounded.BoundReason == nil ||
		*bounded.BoundReason != CompletedRecordingBoundReasonTimeout || bounded.ScannedCount != 0 || len(bounded.Items) != 0 {
		t.Fatalf("timeout envelope=%#v err=%v", bounded, err)
	}
}

func TestCompletedRecordingConfigurationIsDisabledByDefaultAndBindsPreviewApplyToOneLibraryRoot(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "completed-config-admin", authz.RoleAdmin)
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	root := t.TempDir()
	otherRoot := t.TempDir()
	outsideRoot := t.TempDir()
	for _, value := range []string{root, otherRoot} {
		if err := os.WriteFile(filepath.Join(value, "CB-current-20260721-120000.mp4"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := managerconfig.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.SetInterface(managerconfig.Stash, managerconfig.StashConfigs{
		&managerconfig.StashConfig{Path: root},
		&managerconfig.StashConfig{Path: otherRoot},
	})
	store := &completedGraphStore{links: map[string]struct{}{}}
	service := &cammodel.CompletedImportService{
		Authorizer: completedImportContextAuthorizer{database: database}, Resolver: &completedGraphResolver{}, Store: store,
	}
	resolver := &Resolver{database: database, completedImportService: service}
	query := &queryResolver{Resolver: resolver}
	mutation := &mutationResolver{Resolver: resolver}

	got, err := query.CompletedRecordingImportConfig(adminCtx)
	if err != nil || got.Enabled || got.Root != "" {
		t.Fatalf("unsafe default config=%#v err=%v", got, err)
	}
	if _, err := query.CompletedRecordingImportConfig(context.Background()); err == nil {
		t.Fatal("anonymous config read allowed")
	}
	if _, err := mutation.CompletedRecordingPreview(adminCtx, completedGraphInput(root)); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled preview error=%v", err)
	}
	if _, err := mutation.CompletedRecordingImportConfigure(adminCtx, CompletedRecordingImportConfigInput{Enabled: true, Root: outsideRoot}); !errors.Is(err, ErrInput) {
		t.Fatalf("outside configured Library roots accepted: %v", err)
	}
	configured, err := mutation.CompletedRecordingImportConfigure(adminCtx, CompletedRecordingImportConfigInput{Enabled: true, Root: root})
	if err != nil || !configured.Enabled || configured.Root != root {
		t.Fatalf("configured=%#v err=%v", configured, err)
	}
	preview, err := mutation.CompletedRecordingPreview(adminCtx, completedGraphInput(root))
	if err != nil || len(preview.Items) != 1 {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}

	cfg.SetCompletedRecordingImportConfig(managerconfig.CompletedRecordingImportConfig{Enabled: true, Root: otherRoot})
	if _, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{
		PreviewID: preview.PreviewID, SelectedCandidateIDs: []string{preview.Items[0].CandidateID},
	}); err == nil || !strings.Contains(err.Error(), "configured root changed") {
		t.Fatalf("apply survived configured-root change: %v", err)
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatalf("root change mutated persistence: links=%d audits=%d", len(store.links), len(store.audits))
	}
	cfg.SetCompletedRecordingImportConfig(managerconfig.CompletedRecordingImportConfig{Enabled: false, Root: otherRoot})
	if _, err := mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{PreviewID: preview.PreviewID}); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("apply survived disable: %v", err)
	}
}

func TestCompletedRecordingApplyLeasesAuthoritativeConfigurationBeforePersistence(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "completed-config-race-admin", authz.RoleAdmin)
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	root := t.TempDir()
	otherRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CB-current-20260721-120000.mp4"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := managerconfig.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.SetInterface(managerconfig.Stash, managerconfig.StashConfigs{&managerconfig.StashConfig{Path: root}, &managerconfig.StashConfig{Path: otherRoot}})
	cfg.SetCompletedRecordingImportConfig(managerconfig.CompletedRecordingImportConfig{Enabled: true, Root: root})
	store := &completedGraphStore{links: map[string]struct{}{}}
	service := &cammodel.CompletedImportService{Authorizer: completedImportContextAuthorizer{database: database}, Resolver: &completedGraphResolver{}, Store: store}
	resolver := &Resolver{database: database, completedImportService: service}
	mutation := &mutationResolver{Resolver: resolver}
	preview, err := mutation.CompletedRecordingPreview(adminCtx, completedGraphInput(root))
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	service.AcquireConfiguredRoot = func(expected string) (func(), error) {
		once.Do(func() { close(entered) })
		<-resume
		return resolver.acquireCompletedRecordingRoot(expected)
	}
	// completedRecordingService installs the production lease, so replace it
	// after Preview and call the service directly to discriminate the boundary.
	done := make(chan error, 1)
	go func() {
		_, applyErr := service.ApplyConfigured(adminCtx, admin.UserID, preview.PreviewID, []string{preview.Items[0].CandidateID}, root)
		done <- applyErr
	}()
	<-entered
	cfg.SetCompletedRecordingImportConfig(managerconfig.CompletedRecordingImportConfig{Enabled: false, Root: otherRoot})
	close(resume)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "configured root changed") {
		t.Fatalf("apply survived concurrent disable/root change: %v", err)
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatalf("configuration race persisted: links=%d audits=%d", len(store.links), len(store.audits))
	}
}

func TestCompletedRecordingConcurrentConfigurePreviewAndApplyAreRaceFree(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "completed-concurrency-admin", authz.RoleAdmin)
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CB-current-20260721-120000.mp4"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := managerconfig.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.SetInterface(managerconfig.Stash, managerconfig.StashConfigs{&managerconfig.StashConfig{Path: root}})
	cfg.SetCompletedRecordingImportConfig(managerconfig.CompletedRecordingImportConfig{Enabled: true, Root: root})
	service := &cammodel.CompletedImportService{
		Authorizer: completedImportContextAuthorizer{database: database}, Resolver: &completedGraphResolver{},
		Store: &completedGraphStore{links: map[string]struct{}{}},
	}
	resolver := &Resolver{database: database, completedImportService: service}
	mutation := &mutationResolver{Resolver: resolver}
	preview, err := mutation.CompletedRecordingPreview(adminCtx, completedGraphInput(root))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, _ = mutation.CompletedRecordingImportConfigure(adminCtx, CompletedRecordingImportConfigInput{Enabled: true, Root: root})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, _ = mutation.CompletedRecordingPreview(adminCtx, completedGraphInput(root))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, _ = mutation.CompletedRecordingApply(adminCtx, CompletedRecordingApplyInput{PreviewID: preview.PreviewID})
		}
	}()
	wg.Wait()
}

package cammodel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type completedAuthFixture struct{ allow bool }

func (a completedAuthFixture) RequireDataAdmin(context.Context) error {
	if !a.allow {
		return errors.New("data.admin required")
	}
	return nil
}

type completedResolverFixture struct{}

func (completedResolverFixture) ResolveCompletedRecording(_ context.Context, _ string, rel string, parsed CompletedParsedName) (CompletedResolution, error) {
	if strings.Contains(rel, "noscene") {
		return CompletedResolution{Outcome: CompletedSceneNotFound, ReviewReason: "existing Scene path required"}, nil
	}
	switch strings.ToLower(parsed.Site) {
	case "missing":
		return CompletedResolution{Outcome: CompletedSiteNotFound, ReviewReason: "site not found"}, nil
	case "cb":
	default:
		return CompletedResolution{Outcome: CompletedSiteNotFound, ReviewReason: "site not found"}, nil
	}
	switch strings.ToLower(parsed.Handle) {
	case "alice":
		return CompletedResolution{SceneID: sceneForRel(rel), SiteID: 10, ModelID: 20, MatchState: CompletedAliasCurrent, Outcome: CompletedExactReady}, nil
	case "oldalice":
		return CompletedResolution{SceneID: sceneForRel(rel), SiteID: 10, ModelID: 20, MatchState: CompletedAliasHistorical, Outcome: CompletedReviewRequired, ReviewReason: "unique historical alias requires identity review", ReviewCode: CompletedReviewAliasReused}, nil
	case "reused":
		return CompletedResolution{SceneID: sceneForRel(rel), SiteID: 10, MatchState: CompletedAliasAmbiguous, Outcome: CompletedReviewRequired, ReviewReason: "historical alias reused", ReviewCode: CompletedReviewAliasReused}, nil
	default:
		return CompletedResolution{SceneID: sceneForRel(rel), SiteID: 10, Outcome: CompletedModelNotFound, ReviewReason: "model not found"}, nil
	}
}

func sceneForRel(rel string) int64 {
	if strings.Contains(rel, "second") {
		return 2
	}
	return 1
}

type completedStoreFixture struct {
	mu        sync.Mutex
	links     map[string]CompletedRecording
	audits    []CompletedImportAudit
	failScene int64
}

type completedTxFixture struct {
	links     map[string]CompletedRecording
	audits    []CompletedImportAudit
	failScene int64
}

func (s *completedStoreFixture) WithCompletedImportTransaction(ctx context.Context, fn func(context.Context, CompletedImportTx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := map[string]CompletedRecording{}
	for key, value := range s.links {
		clone[key] = value
	}
	tx := &completedTxFixture{links: clone, audits: append([]CompletedImportAudit(nil), s.audits...), failScene: s.failScene}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	s.links, s.audits = tx.links, tx.audits
	return nil
}

func (t *completedTxFixture) LinkCamShowMetadata(_ context.Context, item CompletedRecording) (bool, error) {
	if item.SceneID == t.failScene {
		return false, errors.New("injected persistence failure")
	}
	key := completedImportKey(item)
	if _, exists := t.links[key]; exists {
		return false, nil
	}
	t.links[key] = item
	return true, nil
}

func (t *completedTxFixture) WriteCompletedImportAudit(_ context.Context, audit CompletedImportAudit) error {
	t.audits = append(t.audits, audit)
	return nil
}

func completedImportKey(item CompletedRecording) string {
	return item.RelativePath + "|" + item.ParserVersion + "|" + string(rune(item.SceneID))
}

func completedParser() CompletedFilenameParser {
	return CompletedFilenameParser{
		Pattern: regexp.MustCompile(`^(?P<site>[A-Za-z]+)-(?P<model>[A-Za-z]+)-(?P<timestamp>[0-9]{8}-[0-9]{6})(?:-(?:second|noscene))?[.]mp4$`),
		Layout:  "20060102-150405", Location: time.FixedZone("fixture", -7*60*60),
		Precision: CompletedTimeSecond, Version: "fixture-v1",
	}
}

func completedOptions(root string) CompletedDiscoveryOptions {
	return CompletedDiscoveryOptions{
		Root: root, MaxFiles: 100, MaxDepth: 5, Timeout: time.Second,
		Extensions: map[string]struct{}{".mp4": {}},
	}
}

func writeSynthetic(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newCompletedService(allow bool, store *completedStoreFixture) *CompletedImportService {
	return &CompletedImportService{
		Authorizer: completedAuthFixture{allow: allow}, Resolver: completedResolverFixture{}, Store: store,
		Now: func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
	}
}

func findCompleted(t *testing.T, items []CompletedRecording, suffix string) CompletedRecording {
	t.Helper()
	for _, item := range items {
		if strings.HasSuffix(item.RelativePath, suffix) {
			return item
		}
	}
	t.Fatalf("missing %s in %#v", suffix, items)
	return CompletedRecording{}
}

func TestCompletedImportDryRunTypedOutcomesAndProvenance(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	writeSynthetic(t, root, "CB-oldalice-20260720-110000.mp4")
	writeSynthetic(t, root, "CB-reused-20260719-100000.mp4")
	writeSynthetic(t, root, "missing-alice-20260721-120000.mp4")
	writeSynthetic(t, root, "CB-unknown-20260721-120000.mp4")
	writeSynthetic(t, root, "CB-alice-20260721-120000-noscene.mp4")
	writeSynthetic(t, root, "not-a-recording.mp4")
	writeSynthetic(t, root, "ignored.txt")
	writeSynthetic(t, root, ".hidden.mp4")
	writeSynthetic(t, root, "CB-alice-20260721-120000.partial.mp4")
	if err := os.Mkdir(filepath.Join(root, ".scratch"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeSynthetic(t, root, ".scratch/CB-alice-20260721-120000.mp4")

	service := newCompletedService(true, &completedStoreFixture{links: map[string]CompletedRecording{}})
	preview, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 7 {
		t.Fatalf("items=%d %#v", len(preview.Items), preview.Items)
	}
	for _, item := range preview.Items {
		if strings.Contains(item.RelativePath, ".hidden") || strings.Contains(item.RelativePath, ".partial.") || strings.Contains(item.RelativePath, ".scratch") {
			t.Fatalf("hidden or partial scratch file admitted: %#v", item)
		}
	}
	exact := findCompleted(t, preview.Items, "CB-alice-20260721-120000.mp4")
	if exact.Outcome != CompletedExactReady || exact.SceneID != 1 || exact.SiteID != 10 || exact.ModelID != 20 || exact.MatchState != CompletedAliasCurrent {
		t.Fatalf("exact=%#v", exact)
	}
	if exact.TimePrecision != CompletedTimeSecond || exact.Timezone != "fixture" || exact.CompletedAt.IsZero() || exact.ObservedAt.IsZero() || exact.Fingerprint.Inode == 0 {
		t.Fatalf("missing provenance: %#v", exact)
	}
	if got := findCompleted(t, preview.Items, "CB-oldalice-20260720-110000.mp4"); got.MatchState != CompletedAliasHistorical || got.Outcome != CompletedReviewRequired || got.ReviewCode != CompletedReviewAliasReused {
		t.Fatalf("historical=%#v", got)
	}
	if got := findCompleted(t, preview.Items, "CB-reused-20260719-100000.mp4"); got.Outcome != CompletedReviewRequired || got.ReviewCode != CompletedReviewAliasReused || got.ReviewReason == "" {
		t.Fatalf("ambiguous=%#v", got)
	}
	if got := findCompleted(t, preview.Items, "missing-alice-20260721-120000.mp4"); got.Outcome != CompletedSiteNotFound {
		t.Fatalf("site=%#v", got)
	}
	if got := findCompleted(t, preview.Items, "CB-unknown-20260721-120000.mp4"); got.Outcome != CompletedModelNotFound {
		t.Fatalf("model=%#v", got)
	}
	if got := findCompleted(t, preview.Items, "CB-alice-20260721-120000-noscene.mp4"); got.Outcome != CompletedSceneNotFound {
		t.Fatalf("scene=%#v", got)
	}
	if got := findCompleted(t, preview.Items, "not-a-recording.mp4"); got.Outcome != CompletedInvalidName {
		t.Fatalf("invalid=%#v", got)
	}
}

func TestCompletedImportRejectsFileAndDirectorySymlinksWithLstat(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := writeSynthetic(t, outside, "CB-alice-20260721-120000.mp4")
	if err := os.Symlink(target, filepath.Join(root, "linked.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	service := newCompletedService(true, &completedStoreFixture{links: map[string]CompletedRecording{}})
	preview, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if findCompleted(t, preview.Items, "linked.mp4").Outcome != CompletedRejectedSymlink {
		t.Fatal("file symlink was not rejected")
	}
	if findCompleted(t, preview.Items, "linked-dir").Outcome != CompletedRejectedSymlink {
		t.Fatal("directory symlink was not rejected")
	}
	for _, item := range preview.Items {
		if strings.Contains(item.RelativePath, "CB-alice") {
			t.Fatalf("followed symlink: %#v", item)
		}
	}
	symlinkRoot := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(root, symlinkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DryRun(context.Background(), completedOptions(symlinkRoot), completedParser()); err == nil {
		t.Fatal("symlink root accepted")
	}
}

func TestCompletedImportAuthorizationSelectionApplyIdempotencyAndAuditRedaction(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	store := &completedStoreFixture{links: map[string]CompletedRecording{}}
	if _, err := newCompletedService(false, store).DryRun(context.Background(), completedOptions(root), completedParser()); err == nil {
		t.Fatal("non-admin dry run accepted")
	}
	service := newCompletedService(true, store)
	preview, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	item := preview.Items[0]
	if _, err := service.Apply(context.Background(), "admin-1", preview.ID, []string{item.CandidateID, item.CandidateID}); err == nil {
		t.Fatal("duplicate selection accepted")
	}
	if _, err := service.Apply(context.Background(), "admin-1", preview.ID, []string{"not-bound"}); err == nil {
		t.Fatal("unbound selection accepted")
	}
	results, err := service.Apply(context.Background(), "admin-1", preview.ID, []string{item.CandidateID})
	if err != nil || len(results) != 1 || results[0].Outcome != CompletedApplied {
		t.Fatalf("apply=%#v err=%v", results, err)
	}
	results, err = service.Apply(context.Background(), "admin-1", preview.ID, []string{item.CandidateID})
	if err != nil || results[0].Outcome != CompletedAlreadyApplied {
		t.Fatalf("replay=%#v err=%v", results, err)
	}
	if len(store.links) != 1 || len(store.audits) != 2 {
		t.Fatalf("links=%d audits=%d", len(store.links), len(store.audits))
	}
	for _, audit := range store.audits {
		if audit.RelativePathHash == "" || strings.Contains(audit.RelativePathHash, "alice") {
			t.Fatalf("audit path not redacted: %#v", audit)
		}
	}
	if _, err := newCompletedService(false, store).Apply(context.Background(), "user", preview.ID, []string{item.CandidateID}); err == nil {
		t.Fatal("non-admin apply accepted")
	}
}

func TestCompletedImportApplyRechecksLstatContainmentAndFingerprint(t *testing.T) {
	root := t.TempDir()
	path := writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	store := &completedStoreFixture{links: map[string]CompletedRecording{}}
	service := newCompletedService(true, store)
	preview, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	item := preview.Items[0]
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := writeSynthetic(t, t.TempDir(), "target.mp4")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	results, err := service.Apply(context.Background(), "admin", preview.ID, []string{item.CandidateID})
	if err == nil || results != nil || !strings.Contains(err.Error(), "no-follow") {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatalf("symlink swap wrote persistence: links=%d audits=%d", len(store.links), len(store.audits))
	}

	root2 := t.TempDir()
	path2 := writeSynthetic(t, root2, "CB-alice-20260721-120000.mp4")
	service2 := newCompletedService(true, &completedStoreFixture{links: map[string]CompletedRecording{}})
	preview2, err := service2.DryRun(context.Background(), completedOptions(root2), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path2, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	results, err = service2.Apply(context.Background(), "admin", preview2.ID, []string{preview2.Items[0].CandidateID})
	if err != nil || results[0].Outcome != CompletedChanged || !strings.Contains(results[0].Reason, "fingerprint") {
		t.Fatalf("results=%#v err=%v", results, err)
	}
}

func TestCompletedImportApplyRejectsParentDirectorySymlinkSwapWithoutPersistence(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "nested")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSynthetic(t, directory, "CB-alice-20260721-120000.mp4")
	store := &completedStoreFixture{links: map[string]CompletedRecording{}}
	service := newCompletedService(true, store)
	preview, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeSynthetic(t, outside, "CB-alice-20260721-120000.mp4")
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, directory); err != nil {
		t.Fatal(err)
	}
	results, err := service.Apply(context.Background(), "admin", preview.ID, []string{preview.Items[0].CandidateID})
	if err == nil || results != nil || !strings.Contains(err.Error(), "no-follow") {
		t.Fatalf("parent symlink swap results=%#v err=%v", results, err)
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatalf("parent symlink swap persisted: links=%d audits=%d", len(store.links), len(store.audits))
	}
}

func TestCompletedImportTransactionRollsBackPersistenceFailure(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	writeSynthetic(t, root, "CB-alice-20260721-120000-second.mp4")
	store := &completedStoreFixture{links: map[string]CompletedRecording{}, failScene: 2}
	service := newCompletedService(true, store)
	preview, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	selected := []string{preview.Items[0].CandidateID, preview.Items[1].CandidateID}
	if _, err := service.Apply(context.Background(), "admin", preview.ID, selected); err == nil {
		t.Fatal("persistence failure accepted")
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatalf("transaction not rolled back: links=%d audits=%d", len(store.links), len(store.audits))
	}
}

func TestCompletedImportDiscoveryBoundsAndNoMutation(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	writeSynthetic(t, root, "CB-oldalice-20260720-110000.mp4")
	service := newCompletedService(true, &completedStoreFixture{links: map[string]CompletedRecording{}})
	opts := completedOptions(root)
	opts.MaxFiles = 1
	preview, err := service.DryRun(context.Background(), opts, completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Truncated || preview.BoundReason != CompletedBoundMaxFiles || preview.ScannedCount != 2 || len(preview.Items) != 1 {
		t.Fatalf("preview=%#v", preview)
	}
	if info, err := os.Stat(filepath.Join(root, "CB-alice-20260721-120000.mp4")); err != nil || info.Size() != 0 {
		t.Fatalf("fixture mutated: %v %#v", err, info)
	}
}

func TestCompletedIdentitySnapshotResolverHistoricalReviewActiveAndIsolation(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	resolver := CompletedIdentitySnapshotResolver{
		Scenes: map[string][]int64{"capture.mp4": {7}},
		Sites:  map[string][]int64{"cb": {8}, "other": {11}},
		Aliases: []CompletedAliasBinding{
			{SiteID: 8, ModelID: 9, NormalizedAlias: "oldalice", ValidFrom: &from, ValidTo: &to},
			{SiteID: 8, ModelID: 9, NormalizedAlias: "alice", ValidFrom: &to, Current: true},
			{SiteID: 11, ModelID: 12, NormalizedAlias: "alice", Current: true},
		},
	}
	before := CompletedIdentitySnapshotResolver{
		Scenes: map[string][]int64{"capture.mp4": append([]int64(nil), resolver.Scenes["capture.mp4"]...)},
		Sites: map[string][]int64{
			"cb":    append([]int64(nil), resolver.Sites["cb"]...),
			"other": append([]int64(nil), resolver.Sites["other"]...),
		},
		Aliases: append([]CompletedAliasBinding(nil), resolver.Aliases...),
	}

	historical, err := resolver.ResolveCompletedRecording(context.Background(), "", "capture.mp4", CompletedParsedName{
		Site: "CB", Handle: "OldAlice", ObservedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || historical.Outcome != CompletedReviewRequired || historical.ReviewCode != CompletedReviewAliasReused ||
		historical.MatchState != CompletedAliasHistorical || historical.ModelID != 9 {
		t.Fatalf("historical=%#v err=%v", historical, err)
	}
	current, err := resolver.ResolveCompletedRecording(context.Background(), "", "capture.mp4", CompletedParsedName{
		Site: "cB", Handle: "ALICE", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || current.Outcome != CompletedExactReady || current.MatchState != CompletedAliasCurrent || current.ModelID != 9 {
		t.Fatalf("current=%#v err=%v", current, err)
	}
	otherSite, err := resolver.ResolveCompletedRecording(context.Background(), "", "capture.mp4", CompletedParsedName{
		Site: "OTHER", Handle: "Alice", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || otherSite.Outcome != CompletedExactReady || otherSite.ModelID != 12 || otherSite.SiteID != 11 {
		t.Fatalf("site isolation=%#v err=%v", otherSite, err)
	}

	collision := resolver
	collision.Aliases = append([]CompletedAliasBinding(nil), resolver.Aliases...)
	collision.Aliases = append(collision.Aliases, CompletedAliasBinding{
		SiteID: 8, ModelID: 13, NormalizedAlias: "alice", ValidFrom: &from, ValidTo: nil,
	})
	ambiguous, err := collision.ResolveCompletedRecording(context.Background(), "", "capture.mp4", CompletedParsedName{
		Site: "cb", Handle: "alice", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || ambiguous.Outcome != CompletedReviewRequired || ambiguous.ReviewCode != CompletedReviewMultipleAliases ||
		ambiguous.MatchState != CompletedAliasAmbiguous {
		t.Fatalf("current/historical collision=%#v err=%v", ambiguous, err)
	}
	if !reflect.DeepEqual(resolver, before) {
		t.Fatalf("resolver mutated identity snapshot: before=%#v after=%#v", before, resolver)
	}
}

func TestCompletedBoundEnvelopeFinalizationIsSingleValidatedTransition(t *testing.T) {
	tests := []struct {
		name       string
		transition completedBoundTransition
		truncated  bool
		reason     CompletedBoundReason
	}{
		{"complete", completedBoundTransition{}, false, CompletedBoundNone},
		{"timeout", completedBoundTransition{reached: true, reason: CompletedBoundTimeout}, true, CompletedBoundTimeout},
		{"max-files", completedBoundTransition{reached: true, reason: CompletedBoundMaxFiles}, true, CompletedBoundMaxFiles},
		{"max-depth", completedBoundTransition{reached: true, reason: CompletedBoundMaxDepth}, true, CompletedBoundMaxDepth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview := CompletedImportPreview{Truncated: !tt.truncated, BoundReason: "CORRUPT"}
			if err := finalizeCompletedBoundEnvelope(&preview, tt.transition); err != nil {
				t.Fatal(err)
			}
			if preview.Truncated != tt.truncated || preview.BoundReason != tt.reason {
				t.Fatalf("preview=%#v", preview)
			}
		})
	}
	for _, corrupt := range []completedBoundTransition{
		{reached: true, reason: "CORRUPT"},
		{reached: false, reason: CompletedBoundTimeout},
	} {
		preview := CompletedImportPreview{Truncated: false, BoundReason: CompletedBoundNone}
		if err := finalizeCompletedBoundEnvelope(&preview, corrupt); err == nil {
			t.Fatalf("corrupt transition accepted: %#v", corrupt)
		}
		if preview.Truncated || preview.BoundReason != CompletedBoundNone {
			t.Fatalf("corrupt transition partially repaired envelope: %#v", preview)
		}
	}

	source, err := os.ReadFile("completed_import.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(source), "preview.Truncated, preview.BoundReason ="); got != 1 {
		t.Fatalf("bound envelope assignments=%d; want exactly one authoritative assignment", got)
	}
}

func TestCompletedBoundTransitionFirstValidReasonWinsAndOverwriteFails(t *testing.T) {
	tests := []struct {
		name          string
		first, second CompletedBoundReason
	}{
		{"timeout-then-max-files", CompletedBoundTimeout, CompletedBoundMaxFiles},
		{"max-files-then-timeout", CompletedBoundMaxFiles, CompletedBoundTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var transition completedBoundTransition
			if err := transition.reach(tt.first); err != nil {
				t.Fatalf("first transition: %v", err)
			}
			if err := transition.reach(tt.second); err == nil {
				t.Fatalf("competing transition %s overwrote %s", tt.second, tt.first)
			}
			if !transition.reached || transition.reason != tt.first {
				t.Fatalf("first transition was not preserved: %#v", transition)
			}
			preview := CompletedImportPreview{}
			if err := finalizeCompletedBoundEnvelope(&preview, transition); err != nil {
				t.Fatal(err)
			}
			if !preview.Truncated || preview.BoundReason != tt.first {
				t.Fatalf("final envelope lost first reason: %#v", preview)
			}
		})
	}
}

func TestCompletedImportTimeoutReturnsRetryableExpiringEmptyPreview(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	store := &completedStoreFixture{links: map[string]CompletedRecording{}}
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	service := newCompletedService(true, store)
	service.Now = func() time.Time { return now }
	service.PreviewTTL = 2 * time.Minute
	service.DiscoveryStep = func(ctx context.Context, _ string) { <-ctx.Done() }
	opts := completedOptions(root)
	opts.Timeout = time.Nanosecond

	preview, err := service.DryRun(context.Background(), opts, completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if preview.ID == "" || !preview.Truncated || preview.BoundReason != CompletedBoundTimeout || preview.ScannedCount != 0 || len(preview.Items) != 0 {
		t.Fatalf("preview=%#v", preview)
	}
	results, err := service.Apply(context.Background(), "admin", preview.ID, nil)
	if err != nil || len(results) != 0 {
		t.Fatalf("empty selection=%#v err=%v", results, err)
	}
	service.DiscoveryStep = nil
	retry, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil || retry.ID == "" || retry.ID == preview.ID || len(retry.Items) != 1 {
		t.Fatalf("retry=%#v err=%v", retry, err)
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatalf("timeout/retry mutated store: links=%d audits=%d", len(store.links), len(store.audits))
	}
	now = preview.ExpiresAt
	if _, err := service.Apply(context.Background(), "admin", preview.ID, nil); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired preview got %v", err)
	}
	if _, err := service.Apply(context.Background(), "admin", preview.ID, nil); err == nil || !strings.Contains(err.Error(), "missing or stale") {
		t.Fatalf("expired preview was not evicted: %v", err)
	}
}

func TestCompletedImportTimeoutPreservesSafePartialProgress(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	writeSynthetic(t, root, "CB-oldalice-20260720-110000.mp4")
	store := &completedStoreFixture{links: map[string]CompletedRecording{}}
	service := newCompletedService(true, store)
	service.DiscoveryStep = func(ctx context.Context, rel string) {
		if strings.Contains(rel, "oldalice") {
			<-ctx.Done()
		}
	}
	opts := completedOptions(root)
	opts.Timeout = 20 * time.Millisecond
	preview, err := service.DryRun(context.Background(), opts, completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if preview.ID == "" || preview.BoundReason != CompletedBoundTimeout || !preview.Truncated || preview.ScannedCount != 1 || len(preview.Items) != 1 {
		t.Fatalf("partial preview=%#v", preview)
	}
	if preview.Items[0].Outcome != CompletedExactReady {
		t.Fatalf("safe item=%#v", preview.Items[0])
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatal("partial timeout dry run mutated store")
	}
	selected := []string{preview.Items[0].CandidateID}
	results, err := service.Apply(context.Background(), "admin", preview.ID, selected)
	if err != nil || len(results) != 1 || results[0].Outcome != CompletedApplied {
		t.Fatalf("partial apply=%#v err=%v", results, err)
	}
	results, err = service.Apply(context.Background(), "admin", preview.ID, selected)
	if err != nil || len(results) != 1 || results[0].Outcome != CompletedAlreadyApplied {
		t.Fatalf("partial replay=%#v err=%v", results, err)
	}
	if len(store.links) != 1 || len(store.audits) != 2 {
		t.Fatalf("partial replay links=%d audits=%d", len(store.links), len(store.audits))
	}
}

func TestCompletedImportDepthBoundReturnsPreviewWithoutMutation(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "level-one", "level-two")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSynthetic(t, deep, "CB-alice-20260721-120000.mp4")
	store := &completedStoreFixture{links: map[string]CompletedRecording{}}
	service := newCompletedService(true, store)
	opts := completedOptions(root)
	opts.MaxDepth = 1
	preview, err := service.DryRun(context.Background(), opts, completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if preview.ID == "" || !preview.Truncated || preview.BoundReason != CompletedBoundMaxDepth || preview.ScannedCount != 2 || len(preview.Items) != 0 {
		t.Fatalf("depth preview=%#v", preview)
	}
	if len(store.links) != 0 || len(store.audits) != 0 {
		t.Fatal("depth-bound dry run mutated store")
	}
}

func TestCompletedIdentityCardinalityConflictsShareReviewOutcomeWithTypedReasons(t *testing.T) {
	observed := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	parsed := CompletedParsedName{Site: "cb", Handle: "alice", ObservedAt: observed}
	base := CompletedIdentitySnapshotResolver{
		Scenes:  map[string][]int64{"capture.mp4": {1}},
		Sites:   map[string][]int64{"cb": {2}},
		Aliases: []CompletedAliasBinding{{SiteID: 2, ModelID: 3, NormalizedAlias: "alice", Current: true}},
	}
	tests := []struct {
		name string
		edit func(*CompletedIdentitySnapshotResolver)
		code CompletedReviewReasonCode
	}{
		{"scenes", func(r *CompletedIdentitySnapshotResolver) { r.Scenes["capture.mp4"] = []int64{1, 4} }, CompletedReviewMultipleScenes},
		{"sites", func(r *CompletedIdentitySnapshotResolver) { r.Sites["cb"] = []int64{2, 5} }, CompletedReviewMultipleSites},
		{"aliases", func(r *CompletedIdentitySnapshotResolver) {
			r.Aliases = append(r.Aliases, CompletedAliasBinding{SiteID: 2, ModelID: 6, NormalizedAlias: "alice", Current: true})
		}, CompletedReviewMultipleAliases},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := base
			resolver.Scenes = map[string][]int64{"capture.mp4": append([]int64(nil), base.Scenes["capture.mp4"]...)}
			resolver.Sites = map[string][]int64{"cb": append([]int64(nil), base.Sites["cb"]...)}
			resolver.Aliases = append([]CompletedAliasBinding(nil), base.Aliases...)
			tt.edit(&resolver)
			got, err := resolver.ResolveCompletedRecording(context.Background(), "", "capture.mp4", parsed)
			if err != nil || got.Outcome != CompletedReviewRequired || got.ReviewCode != tt.code || got.ReviewReason == "" {
				t.Fatalf("got=%#v err=%v", got, err)
			}
		})
	}
}

func TestCompletedPreviewCapacityPrunesExpiredAndDeterministicallyEvictsOldest(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	service := newCompletedService(true, &completedStoreFixture{links: map[string]CompletedRecording{}})
	service.Now = func() time.Time { return now }
	service.PreviewTTL = time.Minute
	service.MaxPreviews = 2

	first, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	second, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if len(service.previews) != 2 {
		t.Fatalf("cap boundary size=%d", len(service.previews))
	}
	now = now.Add(time.Second)
	third, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if len(service.previews) != 2 || service.previews[first.ID] != nil || service.previews[second.ID] == nil || service.previews[third.ID] == nil {
		t.Fatalf("oldest eviction state=%#v first=%s second=%s third=%s", service.previews, first.ID, second.ID, third.ID)
	}
	if _, err := service.Apply(context.Background(), "admin", first.ID, nil); err == nil || !strings.Contains(err.Error(), "missing or stale") {
		t.Fatalf("evicted preview apply=%v", err)
	}

	now = now.Add(2 * time.Minute)
	fourth, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if len(service.previews) != 1 || service.previews[fourth.ID] == nil {
		t.Fatalf("expired previews not pruned: %#v", service.previews)
	}
	for _, expired := range []string{second.ID, third.ID} {
		if _, err := service.Apply(context.Background(), "admin", expired, nil); err == nil || !strings.Contains(err.Error(), "missing or stale") {
			t.Fatalf("pruned preview %s apply=%v", expired, err)
		}
	}

	service.MaxPreviews = -1
	if _, err := service.DryRun(context.Background(), completedOptions(root), completedParser()); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("invalid capacity=%v", err)
	}
}

func TestCompletedPreviewCapacityUsesIDToBreakCreatedAtTies(t *testing.T) {
	service := &CompletedImportService{
		Now:         func() time.Time { return time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC) },
		MaxPreviews: 2,
	}
	created := service.now()
	for _, id := range []string{"b", "a", "c"} {
		preview := CompletedImportPreview{ID: id, CreatedAt: created, ExpiresAt: created.Add(time.Minute)}
		if err := service.storePreview(preview, CompletedFilenameParser{}); err != nil {
			t.Fatal(err)
		}
	}
	if service.previews["a"] != nil || service.previews["b"] == nil || service.previews["c"] == nil {
		t.Fatalf("tie eviction is not deterministic: %#v", service.previews)
	}
}

type completedBlockingStore struct {
	delegate *completedStoreFixture
	started  chan struct{}
	release  chan struct{}
}

func (s *completedBlockingStore) WithCompletedImportTransaction(ctx context.Context, fn func(context.Context, CompletedImportTx) error) error {
	close(s.started)
	<-s.release
	return s.delegate.WithCompletedImportTransaction(ctx, fn)
}

func TestCompletedPreviewCapacityNeverEvictsInFlightApply(t *testing.T) {
	root := t.TempDir()
	writeSynthetic(t, root, "CB-alice-20260721-120000.mp4")
	delegate := &completedStoreFixture{links: map[string]CompletedRecording{}}
	blocking := &completedBlockingStore{delegate: delegate, started: make(chan struct{}), release: make(chan struct{})}
	service := newCompletedService(true, delegate)
	service.Store = blocking
	service.MaxPreviews = 1
	preview, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, applyErr := service.Apply(context.Background(), "admin", preview.ID, []string{preview.Items[0].CandidateID})
		done <- applyErr
	}()
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("apply did not acquire preview")
	}
	if state := service.previews[preview.ID]; state == nil || state.inFlight != 1 {
		t.Fatalf("preview not leased: %#v", state)
	}
	if _, err := service.DryRun(context.Background(), completedOptions(root), completedParser()); err == nil || !strings.Contains(err.Error(), "capacity is busy") {
		t.Fatalf("in-flight preview was evicted or cap did not fail closed: %v", err)
	}
	if service.previews[preview.ID] == nil {
		t.Fatal("in-flight preview evicted")
	}
	close(blocking.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("apply did not finish")
	}
	replacement, err := service.DryRun(context.Background(), completedOptions(root), completedParser())
	if err != nil {
		t.Fatal(err)
	}
	if service.previews[preview.ID] != nil || service.previews[replacement.ID] == nil {
		t.Fatalf("completed lease was not deterministically evictable: %#v", service.previews)
	}
}

func TestCompletedImportSurfaceHasNoRecordingProcessOrMediaWriteAuthority(t *testing.T) {
	typ := reflect.TypeOf(&CompletedImportService{})
	for _, forbidden := range []string{"Start", "Stop", "Record", "Download", "Scan", "WriteFile", "OpenMedia", "Exec", "FFmpeg", "ZeroMQ"} {
		for i := 0; i < typ.NumMethod(); i++ {
			if strings.Contains(strings.ToLower(typ.Method(i).Name), strings.ToLower(forbidden)) {
				t.Fatalf("forbidden method %s", typ.Method(i).Name)
			}
		}
	}
	providerType := reflect.TypeOf((*CompletedRecordingProvider)(nil)).Elem()
	if providerType.NumMethod() != 2 {
		t.Fatalf("CompletedRecordingProvider grew authority: %d methods", providerType.NumMethod())
	}
	for _, want := range []string{"Key", "ListCompleted"} {
		if _, ok := providerType.MethodByName(want); !ok {
			t.Fatalf("missing %s", want)
		}
	}
}

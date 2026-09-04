package api

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/cammodel"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func TestCamGirlFinderResolversRequirePersistedDataAdmin(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "cam-girl-finder-admin", authz.RoleAdmin)
	user := createResolverUser(t, database, "cam-girl-finder-user", authz.RoleUser)
	disabledAdmin := createResolverUser(t, database, "cam-girl-finder-disabled-admin", authz.RoleAdmin)
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		id, err := strconv.ParseInt(disabledAdmin.UserID, 10, 64)
		if err != nil {
			return err
		}
		return database.User.SetAccess(ctx, id, authz.RoleAdmin, authz.StatusDisabled)
	}); err != nil {
		t.Fatal(err)
	}
	nonPersistedAdmin := admin
	nonPersistedAdmin.UserID = "999999999"
	reduced := admin
	reduced.TokenScopes = map[authz.Capability]struct{}{authz.LibraryRead: {}}
	resolver := &Resolver{database: database}
	q := &queryResolver{Resolver: resolver}
	m := &mutationResolver{Resolver: resolver}
	for name, ctx := range map[string]context.Context{
		"missing": context.Background(), "user": authz.WithPrincipal(context.Background(), user),
		"reduced-admin": authz.WithPrincipal(context.Background(), reduced),
		"disabled-persisted-admin-with-stale-active-context": authz.WithPrincipal(context.Background(), disabledAdmin),
		"non-persisted-admin":                                authz.WithPrincipal(context.Background(), nonPersistedAdmin),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := q.CamGirlFinderConfig(ctx); err == nil {
				t.Fatal("read config allowed")
			}
			if _, err := m.CamGirlFinderConfigure(ctx, CamGirlFinderConfigInput{Enabled: false, RequestIntervalMs: 1000, TimeoutSeconds: 15, ResultLimit: 25}); err == nil {
				t.Fatal("configure allowed")
			}
			if _, err := m.CamGirlFinderSearch(ctx, "fixture"); err == nil {
				t.Fatal("search allowed")
			}
			if _, err := m.CamGirlFinderIngestPending(ctx, "1", "fixture", []string{"selected"}); err == nil {
				t.Fatal("ingest allowed")
			}
		})
	}
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	got, err := q.CamGirlFinderConfig(adminCtx)
	if err != nil || got.Enabled || got.RequestIntervalMs != 1000 || got.TimeoutSeconds != 15 || got.ResultLimit != 25 {
		t.Fatalf("admin config=%+v err=%v", got, err)
	}
	if _, err := m.CamGirlFinderSearch(adminCtx, "fixture"); err == nil {
		t.Fatal("disabled provider search unexpectedly succeeded")
	}
}

func TestCamGirlFinderPendingAuditIsRedactedAndAtomic(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "cam-girl-finder-audit-admin", authz.RoleAdmin)
	actorID, err := strconv.ParseInt(admin.UserID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	var modelID int64
	if err := txn.WithTxn(context.Background(), database, func(txCtx context.Context) error {
		if _, err := database.CamShow.CreateSite(txCtx, "Chaturbate", nil, camProfileStringPointer("CB")); err != nil {
			return err
		}
		model, err := database.CamShow.CreateModel(txCtx, "Audit Model", nil)
		if err == nil {
			modelID = model.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	observation := func(key, username string) cammodel.ProfileObservation {
		source := "https://provider.invalid/" + username
		providerID := "secret-provider-id-" + username
		return cammodel.ProfileObservation{
			Provider: "camgirlfinder", EvidenceKey: key, Platform: "CB", Username: username,
			ProviderRecordID: &providerID, SourceURL: &source, ObservedAt: time.Now().UTC(),
			PayloadJSON: `{"private":"sentinel"}`,
		}
	}
	var baselineResult *CamGirlFinderIngestResult
	if err := txn.WithTxn(context.Background(), database, func(txCtx context.Context) error {
		results, err := persistCamGirlFinderPending(txCtx, database, actorID, modelID, []cammodel.ProfileObservation{observation("audit-one", "alice")})
		if err != nil {
			return err
		}
		if len(results) != 1 || results[0].ChangeID == nil || results[0].EvidenceID == nil {
			t.Fatalf("results=%+v", results)
		}
		baselineResult = results[0]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var audits []sqlite.AuditEvent
	if err := txn.WithReadTxn(context.Background(), database, func(txCtx context.Context) error {
		var err error
		audits, err = database.Audit.List(txCtx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || audits[0].EventType != camAuditDiscoveryIngested || audits[0].ActorUserID == nil || *audits[0].ActorUserID != actorID || audits[0].DetailsJSON != nil || audits[0].TargetType == nil || *audits[0].TargetType != "cam_sync_change" || audits[0].TargetID == nil || *audits[0].TargetID != *baselineResult.ChangeID || audits[0].Result != baselineResult.Status {
		t.Fatalf("audit=%+v", audits)
	}
	baselineCounts := camGirlFinderAuditCounts(t, database)

	if err := txn.WithTxn(context.Background(), database, func(txCtx context.Context) error {
		_, _, err := database.ExecSQL(txCtx, `CREATE TRIGGER cam_girl_finder_second_audit_fail
			BEFORE INSERT ON user_audit_events
			WHEN NEW.event_type='cam_girl_finder_evidence_ingested'
			 AND (SELECT count(*) FROM user_audit_events WHERE event_type='cam_girl_finder_evidence_ingested') >= 2
			BEGIN SELECT RAISE(ABORT,'injected second discovery audit failure'); END`, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = txn.WithTxn(context.Background(), database, func(txCtx context.Context) error {
		_, err := persistCamGirlFinderPending(txCtx, database, actorID, modelID, []cammodel.ProfileObservation{
			observation("audit-two", "bob"), observation("audit-three", "carol"),
		})
		return err
	})
	if err == nil {
		t.Fatal("CamGirlFinder identity evidence committed despite audit failure")
	}
	if after := camGirlFinderAuditCounts(t, database); after != baselineCounts {
		t.Fatalf("late audit failure left batch residue: before=%v after=%v", baselineCounts, after)
	}
}

func camGirlFinderAuditCounts(t *testing.T, database *sqlite.Database) [3]int64 {
	t.Helper()
	var counts [3]int64
	if err := txn.WithReadTxn(context.Background(), database, func(txCtx context.Context) error {
		_, rows, err := database.QuerySQL(txCtx, `SELECT
			(SELECT count(*) FROM cam_model_profile_provenance),
			(SELECT count(*) FROM cam_sync_changes),
			(SELECT count(*) FROM user_audit_events WHERE event_type='cam_girl_finder_evidence_ingested')`, nil)
		if err != nil {
			return err
		}
		if len(rows) != 1 || len(rows[0]) != len(counts) {
			t.Fatalf("unexpected count result=%+v", rows)
		}
		for i, value := range rows[0] {
			count, ok := value.(int64)
			if !ok {
				t.Fatalf("count %d has type %T", i, value)
			}
			counts[i] = count
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return counts
}

package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func TestCamClassificationResolversRequirePersistedAdmin(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "cam-classification-admin", authz.RoleAdmin)
	user := createResolverUser(t, database, "cam-classification-user", authz.RoleUser)
	staleAdmin := createResolverUser(t, database, "cam-classification-stale-admin", authz.RoleAdmin)
	staleAdminContext := staleAdmin
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		id, err := strconv.ParseInt(staleAdmin.UserID, 10, 64)
		if err != nil {
			return err
		}
		return database.User.SetAccess(ctx, id, authz.RoleUser, authz.StatusActive)
	}); err != nil {
		t.Fatal(err)
	}
	nonPersistedAdmin := admin
	nonPersistedAdmin.UserID = "999999999"
	reducedAdmin := admin
	reducedAdmin.TokenScopes = map[authz.Capability]struct{}{authz.LibraryRead: {}}
	resolver := &Resolver{database: database}
	query := &queryResolver{Resolver: resolver}
	mutation := &mutationResolver{Resolver: resolver}
	input := CamClassificationRuleCreateInput{Name: "Timestamp", Pattern: `^\d{4}-.+\.mp4$`, Target: "BASENAME", Category: "RECORDED", Enabled: true}
	updateInput := CamClassificationRuleUpdateInput{ID: "1", Name: "Updated Timestamp", Pattern: `^updated-.+\.mp4$`, Target: "RELATIVE_PATH", Category: "LIVE", Enabled: true}

	for name, ctx := range map[string]context.Context{
		"missing": context.Background(), "user": authz.WithPrincipal(context.Background(), user),
		"reduced-admin": authz.WithPrincipal(context.Background(), reducedAdmin),
		"downgraded-persisted-admin-with-stale-context": authz.WithPrincipal(context.Background(), staleAdminContext),
		"non-persisted-admin":                           authz.WithPrincipal(context.Background(), nonPersistedAdmin),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := query.CamClassificationRules(ctx); err == nil {
				t.Fatal("listed classification rules")
			}
			if _, err := query.CamClassificationPreview(ctx); err == nil {
				t.Fatal("previewed classification")
			}
			if _, err := mutation.CamClassificationApply(ctx); err == nil {
				t.Fatal("applied classification")
			}
			if _, err := mutation.CamClassificationRuleCreate(ctx, input); err == nil {
				t.Fatal("created classification rule")
			}
			if _, err := mutation.CamClassificationRuleSetEnabled(ctx, "1", false); err == nil {
				t.Fatal("changed classification rule")
			}
			if _, err := mutation.CamClassificationRuleUpdate(ctx, updateInput); err == nil {
				t.Fatal("updated classification rule")
			}
			if _, err := mutation.CamShowUpdate(ctx, CamShowCoreUpdateInput{ID: "1", Title: "denied", ShowType: "LIVE"}); err == nil {
				t.Fatal("updated Cam Show")
			}
		})
	}
	if _, err := query.CamShows(context.Background(), CamShowSortModeDefault); err == nil {
		t.Fatal("anonymous listed Shows")
	}
	for name, principal := range map[string]authz.Principal{"user": user, "reduced-admin": reducedAdmin, "admin": admin} {
		t.Run("shows-"+name, func(t *testing.T) {
			if shows, err := query.CamShows(authz.WithPrincipal(context.Background(), principal), CamShowSortModeDefault); err != nil || len(shows) != 0 {
				t.Fatalf("shows=%+v err=%v", shows, err)
			}
		})
	}
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	created, err := mutation.CamClassificationRuleCreate(adminCtx, input)
	if err != nil || created.Name != input.Name || !created.Enabled {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	updateInput.ID = created.ID
	updated, err := mutation.CamClassificationRuleUpdate(adminCtx, updateInput)
	if err != nil || updated.Name != updateInput.Name || updated.Pattern != updateInput.Pattern || updated.Target != updateInput.Target || updated.Category != updateInput.Category || !updated.Enabled {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	rules, err := query.CamClassificationRules(adminCtx)
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules=%+v err=%v", rules, err)
	}
	if ok, err := mutation.CamClassificationRuleSetEnabled(adminCtx, created.ID, false); err != nil || !ok {
		t.Fatalf("disable ok=%v err=%v", ok, err)
	}
	rules, err = query.CamClassificationRules(adminCtx)
	if err != nil || len(rules) != 1 || rules[0].Enabled {
		t.Fatalf("disabled rules=%+v err=%v", rules, err)
	}
	var audits []sqlite.AuditEvent
	if err := txn.WithReadTxn(context.Background(), database, func(ctx context.Context) error {
		var err error
		audits, err = database.Audit.List(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := []struct{ event, result string }{{camAuditRuleCreated, "success"}, {camAuditRuleUpdated, "success"}, {camAuditRuleEnabledChanged, "disabled"}}
	if len(audits) != len(want) {
		t.Fatalf("audits=%+v", audits)
	}
	for i, expected := range want {
		if audits[i].EventType != expected.event || audits[i].Result != expected.result || audits[i].TargetType == nil || *audits[i].TargetType != "cam_classification_rule" || audits[i].TargetID == nil || *audits[i].TargetID != created.ID || audits[i].DetailsJSON != nil {
			t.Fatalf("audit[%d]=%+v", i, audits[i])
		}
	}
}

func TestCamShowUpdateAuditIsRedactedAndAtomic(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "cam-show-audit-admin", authz.RoleAdmin)
	actorID, _ := strconv.ParseInt(admin.UserID, 10, 64)
	var sceneID, showID int64
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		now := time.Now().UTC()
		scene := &models.Scene{Title: "Original", CreatedAt: now, UpdatedAt: now}
		if err := database.Scene.Create(ctx, scene, nil); err != nil {
			return err
		}
		show, err := database.CamShow.CreateShow(ctx, int64(scene.ID), "LIVE", nil)
		if err == nil {
			sceneID, showID = int64(scene.ID), show.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m := &mutationResolver{Resolver: &Resolver{database: database}}
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	if _, err := m.CamShowUpdate(adminCtx, CamShowCoreUpdateInput{ID: strconv.FormatInt(showID, 10), Title: "Audited", ShowType: "LIVE_PUBLIC"}); err != nil {
		t.Fatal(err)
	}
	var baselineAudits []sqlite.AuditEvent
	if err := txn.WithReadTxn(context.Background(), database, func(ctx context.Context) error {
		var err error
		baselineAudits, err = database.Audit.List(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(baselineAudits) != 1 || baselineAudits[0].EventType != camAuditShowUpdated || baselineAudits[0].ActorUserID == nil || *baselineAudits[0].ActorUserID != actorID || baselineAudits[0].TargetID == nil || *baselineAudits[0].TargetID != strconv.FormatInt(showID, 10) || baselineAudits[0].Result != "success" || baselineAudits[0].DetailsJSON != nil {
		t.Fatalf("audit=%+v", baselineAudits)
	}
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		_, _, err := database.ExecSQL(ctx, `CREATE TRIGGER cam_show_audit_fail BEFORE INSERT ON user_audit_events WHEN NEW.event_type='cam_show_updated' BEGIN SELECT RAISE(ABORT,'injected Cam Show audit failure'); END`, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CamShowUpdate(adminCtx, CamShowCoreUpdateInput{ID: strconv.FormatInt(showID, 10), Title: "Must Roll Back", ShowType: "LIVE_PRIVATE"}); err == nil {
		t.Fatal("Cam Show update committed despite audit failure")
	}
	if err := txn.WithReadTxn(context.Background(), database, func(ctx context.Context) error {
		shows, err := database.CamShow.ListShowDomain(ctx)
		if err != nil {
			return err
		}
		if len(shows) != 1 || shows[0].SceneID != sceneID || shows[0].Title != "Audited" || shows[0].ShowType != "LIVE_PUBLIC" {
			t.Fatalf("shows after rollback=%+v", shows)
		}
		audits, err := database.Audit.List(ctx)
		if err == nil && len(audits) != len(baselineAudits) {
			t.Fatalf("audit count=%d want=%d", len(audits), len(baselineAudits))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCamClassificationApplyLateAuditFailureRollsBackEntireBatch(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "cam-classification-atomic-admin", authz.RoleAdmin)
	actorID, _ := strconv.ParseInt(admin.UserID, 10, 64)
	var sceneIDs [2]int64
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		now := time.Now().UTC()
		for i := range sceneIDs {
			scene := &models.Scene{Title: fmt.Sprintf("Atomic %d", i), CreatedAt: now, UpdatedAt: now}
			if err := database.Scene.Create(ctx, scene, nil); err != nil {
				return err
			}
			sceneIDs[i] = int64(scene.ID)
		}
		_, err := database.CamShow.CreateClassificationRule(ctx, "atomic", `^atomic-.+\.mp4$`, sqlite.CamClassificationTargetBasename, "RECORDED", true, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`CREATE TRIGGER cam_classification_late_audit_fail
		BEFORE INSERT ON user_audit_events
		WHEN NEW.event_type='cam_classification_applied'
		 AND (SELECT count(*) FROM cam_shows WHERE scene_id IN (%d,%d))=2
		BEGIN SELECT RAISE(ABORT,'injected late classification audit failure'); END`, sceneIDs[0], sceneIDs[1])
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		_, _, err := database.ExecSQL(ctx, trigger, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	candidates := []sqlite.CamClassificationCandidate{{SceneID: sceneIDs[0], Basename: "atomic-one.mp4"}, {SceneID: sceneIDs[1], Basename: "atomic-two.mp4"}}
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		result, err := applyCamClassificationWithAudit(ctx, database, actorID, candidates)
		if err == nil && (result == nil || result.Applied != 2) {
			t.Fatalf("pre-audit result=%+v", result)
		}
		return err
	}); err == nil {
		t.Fatal("late audit failure did not abort classification batch")
	}
	if err := txn.WithReadTxn(context.Background(), database, func(ctx context.Context) error {
		shows, err := database.CamShow.ListShows(ctx)
		if err != nil {
			return err
		}
		if len(shows) != 0 {
			t.Fatalf("classification residue=%+v", shows)
		}
		audits, err := database.Audit.List(ctx)
		if err == nil && len(audits) != 0 {
			t.Fatalf("audit residue=%+v", audits)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCamClassificationApplyAuditIsRedactedAndDeterministic(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "cam-classification-redacted-admin", authz.RoleAdmin)
	actorID, _ := strconv.ParseInt(admin.UserID, 10, 64)
	var sceneID int64
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		now := time.Now().UTC()
		scene := &models.Scene{Title: "Redacted", CreatedAt: now, UpdatedAt: now}
		if err := database.Scene.Create(ctx, scene, nil); err != nil {
			return err
		}
		sceneID = int64(scene.ID)
		_, err := database.CamShow.CreateClassificationRule(ctx, "redacted", `^private-sentinel\.mp4$`, sqlite.CamClassificationTargetBasename, "RECORDED", true, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		_, err := applyCamClassificationWithAudit(ctx, database, actorID, []sqlite.CamClassificationCandidate{{SceneID: sceneID, Basename: "private-sentinel.mp4", RelativePath: "secret/path/private-sentinel.mp4"}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := txn.WithReadTxn(context.Background(), database, func(ctx context.Context) error {
		audits, err := database.Audit.List(ctx)
		if err != nil {
			return err
		}
		if len(audits) != 1 || audits[0].EventType != camAuditClassificationApplied || audits[0].ActorUserID == nil || *audits[0].ActorUserID != actorID || audits[0].TargetType == nil || *audits[0].TargetType != "cam_classification" || audits[0].TargetID == nil || *audits[0].TargetID != "enabled-rules" || audits[0].Result != "success" || audits[0].DetailsJSON != nil {
			t.Fatalf("audit=%+v", audits)
		}
		for _, secret := range []string{"private-sentinel", "secret/path", "RECORDED"} {
			if strings.Contains(fmt.Sprint(audits[0]), secret) {
				t.Fatalf("audit contains sentinel %q: %+v", secret, audits[0])
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

package api

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func TestCamModelProfileResolversAdminAuthorizationLifecycleIdempotencyAndRollback(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "cam-profile-admin", authz.RoleAdmin)
	user := createResolverUser(t, database, "cam-profile-user", authz.RoleUser)
	disabledAdmin := createResolverUser(t, database, "cam-profile-disabled-admin", authz.RoleAdmin)
	disabledAdminContext := disabledAdmin
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
	var firstSite, secondSite *sqlite.CamSite
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		var e error
		firstSite, e = database.CamShow.CreateSite(ctx, "Profile First", nil, camProfileStringPointer("profile-first"))
		if e != nil {
			return e
		}
		secondSite, e = database.CamShow.CreateSite(ctx, "Profile Second", nil, camProfileStringPointer("profile-second"))
		return e
	}); err != nil {
		t.Fatal(err)
	}
	resolver := &Resolver{database: database}
	q := &queryResolver{Resolver: resolver}
	m := &mutationResolver{Resolver: resolver}
	create := CamModelProfileCreateInput{DisplayName: "Profile Model", Status: "ACTIVE", Accounts: []*CamModelAccountCreateInput{{SiteID: itoa64(firstSite.ID), Handle: "old_name", ExternalAccountID: camProfileStringPointer("sensitive-account-id")}}}
	missingCtx := context.Background()
	if _, e := q.CamModelProfiles(missingCtx, false); e == nil {
		t.Fatal("anonymous listed profiles")
	}
	if _, e := q.CamModelProfile(missingCtx, "1"); e == nil {
		t.Fatal("anonymous read profile")
	}
	if _, e := q.CamModelSites(missingCtx); e == nil {
		t.Fatal("anonymous listed sites")
	}

	for name, ctx := range map[string]context.Context{"user": authz.WithPrincipal(context.Background(), user), "reduced-admin": authz.WithPrincipal(context.Background(), reduced)} {
		t.Run(name+"-read", func(t *testing.T) {
			if profiles, e := q.CamModelProfiles(ctx, false); e != nil || len(profiles) != 0 {
				t.Fatalf("profiles=%+v err=%v", profiles, e)
			}
			if profile, e := q.CamModelProfile(ctx, "1"); e != nil || profile != nil {
				t.Fatalf("profile=%+v err=%v", profile, e)
			}
			if sites, e := q.CamModelSites(ctx); e != nil || len(sites) != 2 {
				t.Fatalf("sites=%+v err=%v", sites, e)
			}
		})
	}
	for name, ctx := range map[string]context.Context{
		"missing": missingCtx, "user": authz.WithPrincipal(context.Background(), user),
		"reduced-admin": authz.WithPrincipal(context.Background(), reduced),
		"disabled-persisted-admin-with-stale-active-context": authz.WithPrincipal(context.Background(), disabledAdminContext),
		"non-persisted-admin":                                authz.WithPrincipal(context.Background(), nonPersistedAdmin),
	} {
		t.Run(name+"-mutation-denial", func(t *testing.T) {
			if _, e := m.CamModelProfileCreate(ctx, create); e == nil {
				t.Fatal("created profile")
			}
			if _, e := m.CamModelProfileUpdate(ctx, CamModelProfileUpdateInput{ID: "1", DisplayName: "x", Status: "ACTIVE"}); e == nil {
				t.Fatal("updated profile")
			}
			if _, e := m.CamModelAccountAdd(ctx, CamModelAccountAddInput{ModelID: "1", Account: &CamModelAccountCreateInput{SiteID: itoa64(firstSite.ID), Handle: "x"}}); e == nil {
				t.Fatal("added account")
			}
			if _, e := m.CamModelAccountRetire(ctx, "1", time.Now()); e == nil {
				t.Fatal("retired account")
			}
			if _, e := m.CamModelEvidenceCreate(ctx, CamModelEvidenceCreateInput{ModelID: "1", Provider: "fixture", EvidenceKey: "x", ObservedAt: time.Now(), PayloadJSON: "{}"}); e == nil {
				t.Fatal("created evidence")
			}
			if _, e := m.CamModelEvidenceReview(ctx, "1", "APPROVED"); e == nil {
				t.Fatal("reviewed evidence")
			}
		})
	}
	adminCtx := authz.WithPrincipal(context.Background(), admin)
	bad := create
	bad.DisplayName = "Rollback Model"
	bad.Accounts = []*CamModelAccountCreateInput{{SiteID: itoa64(firstSite.ID), Handle: "valid"}, {SiteID: "999999", Handle: "invalid"}}
	if _, e := m.CamModelProfileCreate(adminCtx, bad); e == nil {
		t.Fatal("invalid multi-account create succeeded")
	}
	profiles, e := q.CamModelProfiles(adminCtx, false)
	if e != nil || len(profiles) != 0 {
		t.Fatalf("rollback profiles=%+v err=%v", profiles, e)
	}
	created, e := m.CamModelProfileCreate(adminCtx, create)
	if e != nil {
		t.Fatal(e)
	}
	if len(created.Accounts) != 1 || created.Accounts[0].Source != "MANUAL" {
		t.Fatalf("created=%+v", created)
	}
	updated, e := m.CamModelProfileUpdate(adminCtx, CamModelProfileUpdateInput{ID: created.ID, DisplayName: "Updated Profile", Notes: camProfileStringPointer("notes"), Status: "INACTIVE"})
	if e != nil || updated.DisplayName != "Updated Profile" || updated.Status != "INACTIVE" {
		t.Fatalf("updated=%+v err=%v", updated, e)
	}
	withSecond, e := m.CamModelAccountAdd(adminCtx, CamModelAccountAddInput{ModelID: created.ID, Account: &CamModelAccountCreateInput{SiteID: itoa64(secondSite.ID), Handle: "other_site"}})
	if e != nil || len(withSecond.Accounts) != 2 {
		t.Fatalf("second=%+v err=%v", withSecond, e)
	}
	retired, e := m.CamModelAccountRetire(adminCtx, withSecond.Accounts[0].ID, time.Now().UTC())
	if e != nil || retired.Accounts[0].ValidTo == nil {
		t.Fatalf("retired=%+v err=%v", retired, e)
	}
	observed := time.Now().UTC().Truncate(time.Second)
	evidenceInput := CamModelEvidenceCreateInput{ModelID: created.ID, Provider: "fixture-provider", EvidenceKey: "stable-1", ObservedAt: observed, PayloadJSON: `{"username":"suggested"}`, ProviderRecordID: camProfileStringPointer("sensitive-provider-id")}
	inserted, e := m.CamModelEvidenceCreate(adminCtx, evidenceInput)
	if e != nil || inserted.Status != "INSERTED" || inserted.Evidence.ReviewState != "PENDING" {
		t.Fatalf("insert=%+v err=%v", inserted, e)
	}
	repeat, e := m.CamModelEvidenceCreate(adminCtx, evidenceInput)
	if e != nil || repeat.Status != "UNCHANGED" || repeat.Evidence.ID != inserted.Evidence.ID {
		t.Fatalf("repeat=%+v err=%v", repeat, e)
	}
	userCtx := authz.WithPrincipal(context.Background(), user)
	browserProfile, e := q.CamModelProfile(userCtx, created.ID)
	if e != nil || browserProfile == nil || browserProfile.Accounts[0].ExternalAccountID != nil || browserProfile.Evidence[0].ProviderRecordID != nil || browserProfile.Evidence[0].PayloadJSON != "" {
		t.Fatalf("read-only redaction profile=%+v err=%v", browserProfile, e)
	}
	beforeReview, e := q.CamModelProfile(adminCtx, created.ID)
	if e != nil || len(beforeReview.Accounts) != 2 || len(beforeReview.Evidence) != 1 {
		t.Fatalf("before review=%+v err=%v", beforeReview, e)
	}
	reviewed, e := m.CamModelEvidenceReview(adminCtx, inserted.Evidence.ID, "APPROVED")
	if e != nil || reviewed.ReviewState != "APPROVED" {
		t.Fatalf("review=%+v err=%v", reviewed, e)
	}
	if _, e := m.CamModelEvidenceReview(adminCtx, inserted.Evidence.ID, "REJECTED"); e == nil {
		t.Fatal("review transition repeated")
	}
	withSocial, e := m.CamModelSocialProfileCreate(adminCtx, CamModelSocialProfileCreateInput{
		ModelID: created.ID, Platform: "X", Handle: camProfileStringPointer("profile_model"),
		ProfileURL: "https://social.example/profile_model", Source: "MANUAL",
	})
	if e != nil || len(withSocial.SocialProfiles) != 1 {
		t.Fatalf("social create=%+v err=%v", withSocial, e)
	}
	withRetiredSocial, e := m.CamModelSocialProfileRetire(adminCtx, withSocial.SocialProfiles[0].ID, time.Now().UTC())
	if e != nil || len(withRetiredSocial.SocialProfiles) != 1 || withRetiredSocial.SocialProfiles[0].ValidTo == nil {
		t.Fatalf("social retire=%+v err=%v", withRetiredSocial, e)
	}
	listed, e := q.CamModelProfiles(adminCtx, false)
	if e != nil || len(listed) != 1 || len(listed[0].Accounts) != 2 || len(listed[0].Evidence) != 1 {
		t.Fatalf("listed=%+v err=%v", listed, e)
	}

	var audits []sqlite.AuditEvent
	if e := txn.WithReadTxn(context.Background(), database, func(txCtx context.Context) error {
		var auditErr error
		audits, auditErr = database.Audit.List(txCtx)
		return auditErr
	}); e != nil {
		t.Fatal(e)
	}
	wantEvents := map[string]int{
		camAuditProfileCreated: 1, camAuditProfileUpdated: 1,
		camAuditAccountAdded: 1, camAuditAccountRetired: 1,
		camAuditEvidenceRecorded: 2, camAuditEvidenceReviewed: 1,
		camAuditSocialProfileAdded: 1, camAuditSocialProfileRetired: 1,
	}
	for _, audit := range audits {
		if audit.ActorUserID == nil || strconv.FormatInt(*audit.ActorUserID, 10) != admin.UserID || audit.DetailsJSON != nil {
			t.Fatalf("unscoped or detailed Cam audit=%+v", audit)
		}
		wantEvents[audit.EventType]--
	}
	for event, remaining := range wantEvents {
		if remaining != 0 {
			t.Fatalf("audit event %s remaining=%d audits=%+v", event, remaining, audits)
		}
	}

	if e := txn.WithTxn(context.Background(), database, func(txCtx context.Context) error {
		_, _, triggerErr := database.ExecSQL(txCtx, "CREATE TRIGGER cam_identity_audit_fail BEFORE INSERT ON user_audit_events BEGIN SELECT RAISE(ABORT,'injected cam audit failure'); END", nil)
		return triggerErr
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := m.CamModelProfileUpdate(adminCtx, CamModelProfileUpdateInput{ID: created.ID, DisplayName: "Must Roll Back", Status: "ACTIVE"}); e == nil {
		t.Fatal("identity update committed despite audit failure")
	}
	afterAuditFailure, e := q.CamModelProfile(adminCtx, created.ID)
	if e != nil || afterAuditFailure.DisplayName != "Updated Profile" || afterAuditFailure.Status != "INACTIVE" {
		t.Fatalf("audit failure did not roll back profile=%+v err=%v", afterAuditFailure, e)
	}
}

func camProfileStringPointer(value string) *string { return &value }
func itoa64(v int64) string                        { return strconv.FormatInt(v, 10) }

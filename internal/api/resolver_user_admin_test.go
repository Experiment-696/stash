package api

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authservice"
	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func TestUserAdminResolversAuthorizationAndLastAdmin(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "admin-resolver", authz.RoleAdmin)
	moderator := createResolverUser(t, database, "moderator-resolver", authz.RoleModerator)
	user := createResolverUser(t, database, "user-resolver", authz.RoleUser)
	resolver := &Resolver{database: database}
	query := &queryResolver{Resolver: resolver}
	mutation := &mutationResolver{Resolver: resolver}

	for name, ctx := range map[string]context.Context{
		"missing":   context.Background(),
		"moderator": authz.WithPrincipal(context.Background(), moderator),
		"user":      authz.WithPrincipal(context.Background(), user),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := query.Users(ctx); err == nil {
				t.Fatal("listed users")
			}
			if _, err := mutation.CreateUser(ctx, UserCreateInput{Username: "denied-" + name, Password: "password", Role: string(authz.RoleUser)}); err == nil {
				t.Fatal("created user")
			}
			if _, err := mutation.UpdateUserAccess(ctx, UserAccessInput{ID: admin.UserID, Role: string(authz.RoleAdmin), Status: string(authz.StatusActive)}); err == nil {
				t.Fatal("updated access")
			}
			if _, err := mutation.ResetUserPassword(ctx, user.UserID, "new-password"); err == nil {
				t.Fatal("reset password")
			}
			if _, err := mutation.RevokeUserSessions(ctx, user.UserID); err == nil {
				t.Fatal("revoked sessions")
			}
			if _, err := mutation.RevokeUserAPITokens(ctx, user.UserID); err == nil {
				t.Fatal("revoked tokens")
			}
		})
	}

	adminCtx := authz.WithPrincipal(context.Background(), admin)
	if _, err := mutation.UpdateUserAccess(adminCtx, UserAccessInput{ID: admin.UserID, Role: string(authz.RoleUser), Status: string(authz.StatusActive)}); err == nil {
		t.Fatal("last active Admin was demoted")
	}
	if _, err := mutation.UpdateUserAccess(adminCtx, UserAccessInput{ID: admin.UserID, Role: string(authz.RoleAdmin), Status: string(authz.StatusDisabled)}); err == nil {
		t.Fatal("last active Admin was disabled")
	}
	if _, err := mutation.ResetUserPassword(adminCtx, admin.UserID, "replacement-password"); err == nil {
		t.Fatal("last active Admin was forced out by password reset")
	}
	var denialCount int
	if err := txn.WithReadTxn(context.Background(), database, func(txCtx context.Context) error {
		events, listErr := database.Audit.List(txCtx)
		if listErr != nil {
			return listErr
		}
		for _, event := range events {
			if event.Result == "denied_last_admin" {
				denialCount++
				if event.DetailsJSON != nil {
					t.Fatalf("last-Admin denial included details: %+v", event)
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if denialCount != 3 {
		t.Fatalf("last-Admin denial audit count=%d want=3", denialCount)
	}
	created, err := mutation.CreateUser(adminCtx, UserCreateInput{Username: " New Person ", Password: "password", Role: string(authz.RoleUser)})
	if err != nil || created.Username != "New Person" {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	if _, err := mutation.CreateUser(adminCtx, UserCreateInput{Username: "new person", Password: "password", Role: string(authz.RoleUser)}); err == nil {
		t.Fatal("normalized username collision succeeded")
	}
	users, err := query.Users(adminCtx)
	if err != nil || len(users) != 4 {
		t.Fatalf("users=%+v err=%v", users, err)
	}
}

func TestResetPasswordRevokesSessionsAndTokensAtomically(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "reset-admin", authz.RoleAdmin)
	target := createResolverUser(t, database, "reset-target", authz.RoleUser)
	targetID, _ := persistedPrincipalUserID(target)
	var sessionCredentials *sqlite.SessionCredentials
	var tokenCredentials *sqlite.APITokenCredentials
	err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		var err error
		sessionCredentials, err = database.Session.Create(ctx, targetID, authservice.DefaultSessionIdle, authservice.DefaultSessionAbsolute)
		if err != nil {
			return err
		}
		tokenCredentials, err = database.APIToken.Create(ctx, target, "before-reset", []authz.Capability{authz.LibraryRead}, time.Hour)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	mutation := &mutationResolver{Resolver: &Resolver{database: database}}
	if ok, err := mutation.ResetUserPassword(authz.WithPrincipal(context.Background(), admin), target.UserID, "replacement-password"); err != nil || !ok {
		t.Fatalf("reset ok=%v err=%v", ok, err)
	}
	err = txn.WithReadTxn(context.Background(), database, func(ctx context.Context) error {
		user, err := database.User.Find(ctx, targetID)
		if err != nil {
			return err
		}
		if user.Status != authz.StatusPasswordChangeRequired {
			t.Fatalf("status=%s", user.Status)
		}
		if _, err := database.Session.Authenticate(ctx, sessionCredentials.ID, sessionCredentials.Secret, time.Now()); err == nil {
			t.Fatal("session survived reset")
		}
		if _, err := database.APIToken.Authenticate(ctx, tokenCredentials.ID, tokenCredentials.Secret, time.Now()); err == nil {
			t.Fatal("token survived reset")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAdminUserActionsWriteRedactedAuditEvents(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "audit-admin", authz.RoleAdmin)
	mutation := &mutationResolver{Resolver: &Resolver{database: database}}
	ctx := authz.WithPrincipal(context.Background(), admin)
	created, err := mutation.CreateUser(ctx, UserCreateInput{Username: "audit-target", Password: "never-log-this-password", Role: string(authz.RoleUser)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.UpdateUserAccess(ctx, UserAccessInput{ID: created.ID, Role: string(authz.RoleModerator), Status: string(authz.StatusActive)}); err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.ResetUserPassword(ctx, created.ID, "another-secret-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.RevokeUserSessions(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.RevokeUserAPITokens(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	var events []sqlite.AuditEvent
	err = txn.WithReadTxn(context.Background(), database, func(txCtx context.Context) error {
		var listErr error
		events, listErr = database.Audit.List(txCtx)
		return listErr
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"user_created", "user_access_updated", "user_password_reset", "user_sessions_revoked", "user_api_tokens_revoked"}
	got := make([]string, 0, len(want))
	for _, event := range events {
		for _, eventType := range want {
			if event.EventType == eventType {
				got = append(got, event.EventType)
				if event.ActorUserID == nil || *event.ActorUserID == 0 || event.TargetID == nil || *event.TargetID != created.ID || event.Result != "success" || event.DetailsJSON != nil {
					t.Fatalf("unsafe or incomplete audit event: %+v", event)
				}
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("audit events=%v want=%v", got, want)
	}
}

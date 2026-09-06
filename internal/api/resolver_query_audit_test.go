package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/txn"
)

func TestAuditEventsAreAdminOnlyAndBounded(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "audit-reader-admin", authz.RoleAdmin)
	user := createResolverUser(t, database, "audit-reader-user", authz.RoleUser)
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		return database.Audit.Record(ctx, 1, "test_event", "user", "2", "success")
	}); err != nil {
		t.Fatal(err)
	}
	resolver := &queryResolver{Resolver: &Resolver{database: database}}
	if _, err := resolver.AuditEvents(authz.WithPrincipal(context.Background(), user), 25, 0); err == nil {
		t.Fatal("User read audit events")
	}
	for _, page := range []struct{ limit, offset int }{{0, 0}, {101, 0}, {25, -1}} {
		if _, err := resolver.AuditEvents(authz.WithPrincipal(context.Background(), admin), page.limit, page.offset); err == nil {
			t.Fatalf("invalid page accepted: %+v", page)
		}
	}
	events, err := resolver.AuditEvents(authz.WithPrincipal(context.Background(), admin), 1, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("Admin audit page=%+v err=%v", events, err)
	}
}

func TestAuditEventsSanitizeDatabaseErrors(t *testing.T) {
	database := tokenResolverTestDatabase(t)
	admin := createResolverUser(t, database, "audit-error-admin", authz.RoleAdmin)
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		_, _, err := database.ExecSQL(ctx, "DROP TABLE user_audit_events", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	resolver := &queryResolver{Resolver: &Resolver{database: database}}
	_, err := resolver.AuditEvents(authz.WithPrincipal(context.Background(), admin), 25, 0)
	if !errors.Is(err, errPersonalDataUnavailable) {
		t.Fatalf("AuditEvents error = %v, want sanitized unavailable error", err)
	}
	for _, leaked := range []string{"user_audit_events", "no such table", "SELECT"} {
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(leaked)) {
			t.Fatalf("AuditEvents exposed storage detail %q in %q", leaked, err)
		}
	}
}

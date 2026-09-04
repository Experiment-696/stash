package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stashapp/stash/internal/authn"
	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
)

// Independent store-level verification of P1A-F021's BootstrapAdmin, run
// separately from the resolver-level remote/replay/concurrency test in
// internal/api/resolver_bootstrap_admin_test.go. This exercises the real
// atomic guard directly against a file-backed WAL database (not :memory:,
// so genuine cross-connection contention is possible), and covers
// properties not exercised there: audit-row atomicity, username
// normalization, and password hashing (no plaintext persisted anywhere).

func TestBootstrapAdminUsernameNormalization(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	store := &UserStore{}

	user, err := store.BootstrapAdmin(ctx, "  Ｆirst-Admin  ", "bootstrap-password-1")
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "Ｆirst-Admin" {
		t.Fatalf("username should be trimmed but not case/width-folded: got %q", user.Username)
	}
	if user.NormalizedUsername != "first-admin" {
		t.Fatalf("normalized username: got %q, want %q", user.NormalizedUsername, "first-admin")
	}
	if user.Role != authz.RoleAdmin {
		t.Fatalf("bootstrapped user role=%v, want Admin", user.Role)
	}
	if user.Status != authz.StatusActive {
		t.Fatalf("bootstrapped user status=%v, want Active", user.Status)
	}

	// The normal Create-path collision rule must still apply: no second
	// account, bootstrapped or not, can reuse the same normalized identity.
	if _, err := store.Create(ctx, "first-admin", "different-password", authz.RoleUser); err == nil {
		t.Fatal("normalized-equivalent username was accepted for a second account")
	}
}

func TestBootstrapAdminPasswordHashedNeverPlaintext(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	store := &UserStore{}

	const plaintext = "correct-horse-battery-staple-9427"
	user, err := store.BootstrapAdmin(ctx, "hash-check-admin", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash == nil {
		t.Fatal("bootstrapped user has no password hash")
	}
	stored := *user.PasswordHash
	if stored == plaintext {
		t.Fatal("plaintext password was stored verbatim")
	}
	if strings.Contains(stored, plaintext) {
		t.Fatal("stored hash embeds the plaintext password as a substring")
	}
	if !strings.HasPrefix(stored, "$argon2id$") {
		t.Fatalf("stored hash is not Argon2id: %s", stored)
	}
	ok, err := authn.VerifyPassword(stored, plaintext)
	if err != nil || !ok {
		t.Fatalf("stored hash does not verify the original password: ok=%v err=%v", ok, err)
	}

	// Read the raw column directly too, bypassing the Go struct, in case a
	// future change stores the hash somewhere the struct mapping wouldn't
	// catch (e.g. a duplicate plaintext column).
	var rawHash string
	if err := dbWrapper.Get(ctx, &rawHash, `SELECT password_hash FROM users WHERE id = ?`, user.ID); err != nil {
		t.Fatal(err)
	}
	if rawHash != stored || strings.Contains(rawHash, plaintext) {
		t.Fatalf("raw stored password_hash column is inconsistent or leaks plaintext: %q", rawHash)
	}
}

func TestBootstrapAdminAuditRowAtomicWithCreation(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	store := &UserStore{}

	user, err := store.BootstrapAdmin(ctx, "audit-check-admin", "bootstrap-password-2")
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := dbWrapper.Get(ctx, &count, `SELECT count(*) FROM user_audit_events WHERE event_type = 'first_admin_bootstrapped' AND target_id = ? AND actor_user_id = ? AND result = 'success'`, user.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one first_admin_bootstrapped audit row for user %d, got %d", user.ID, count)
	}
}

func TestBootstrapAdminClosesPermanentlyAfterFirstSuccess(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	store := &UserStore{}

	if _, err := store.BootstrapAdmin(ctx, "first-admin", "bootstrap-password-3"); err != nil {
		t.Fatal(err)
	}
	// A second bootstrap attempt, even with an entirely different username,
	// must be refused once any user exists — this is not a same-username
	// replay check, it's "the bootstrap door is closed at all" once count>0.
	if _, err := store.BootstrapAdmin(ctx, "second-admin-attempt", "bootstrap-password-4"); !errors.Is(err, ErrBootstrapClosed) {
		t.Fatalf("second bootstrap with a different username: err=%v, want ErrBootstrapClosed", err)
	}
	var count int
	if err := dbWrapper.Get(ctx, &count, `SELECT count(*) FROM users`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("user count after a rejected second bootstrap attempt: got %d, want 1", count)
	}
}

func TestBootstrapAdminValidationFailureLeavesRetryOpen(t *testing.T) {
	ctx, _ := userStoreTestContext(t)
	store := &UserStore{}

	if _, err := store.BootstrapAdmin(ctx, "", "bootstrap-password"); err == nil {
		t.Fatal("invalid bootstrap unexpectedly succeeded")
	}
	var count int
	if err := dbWrapper.Get(ctx, &count, `SELECT count(*) FROM users`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid bootstrap created %d users, want 0", count)
	}
	if _, err := store.BootstrapAdmin(ctx, "retry-admin", "bootstrap-password"); err != nil {
		t.Fatalf("valid retry after validation failure: %v", err)
	}
}

func TestBootstrapAdminConcurrentExactlyOneWinnerRealWALDatabase(t *testing.T) {
	// Independent re-verification of the resolver-level concurrency test,
	// this time against a real file-backed database opened through the
	// normal production path (multiple physical connections, genuine
	// SQLite-level write contention via the single-writer-connection
	// discipline already established for P1A-F012), rather than the
	// resolver test's synthetic in-memory harness.
	config.InitializeEmpty()
	path := filepath.Join(t.TempDir(), "bootstrap-concurrent.sqlite")
	db := NewDatabase()
	if err := db.Open(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const attempts = 8
	start := make(chan struct{})
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ctx, err := db.Begin(context.Background(), true)
			if err != nil {
				return
			}
			_, createErr := (&UserStore{}).BootstrapAdmin(ctx, "concurrent-admin", "bootstrap-password-5")
			if createErr == nil {
				if commitErr := db.Commit(ctx); commitErr == nil {
					successes.Add(1)
					return
				}
			}
			_ = db.Rollback(ctx)
		}(i)
	}
	close(start)
	wg.Wait()

	if successes.Load() != 1 {
		t.Fatalf("concurrent bootstrap successes=%d, want exactly 1", successes.Load())
	}
	var count int
	readCtx, err := db.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(readCtx)
	if err := dbWrapper.Get(readCtx, &count, `SELECT count(*) FROM users`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("final user count=%d, want exactly 1", count)
	}
}

package api

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	gqlTransport "github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stashapp/stash/internal/authz"
)

// fakeTxnManager satisfies txn.Manager without touching a real database,
// since monitorWebsocketSession's revalidation work only needs a
// transaction boundary, not real rows.
type fakeTxnManager struct{}

func (fakeTxnManager) Begin(ctx context.Context, _ bool) (context.Context, error) { return ctx, nil }
func (fakeTxnManager) Commit(ctx context.Context) error                           { return nil }
func (fakeTxnManager) Rollback(ctx context.Context) error                         { return nil }
func (fakeTxnManager) IsLocked(err error) bool                                    { return false }

type fakeValidator struct {
	mu       sync.Mutex
	calls    int
	failAt   int // 0 means never fail
	lastUser int64
	lastID   string
}

func (f *fakeValidator) RequireActive(_ context.Context, userID int64, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastUser, f.lastID = userID, id
	if f.failAt != 0 && f.calls >= f.failAt {
		return errors.New("session no longer active")
	}
	return nil
}

func (f *fakeValidator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeTokenValidator struct {
	mu       sync.Mutex
	calls    int
	fail     bool // return an error (revoked/expired/disabled) from the given call onward
	failAt   int
	returned authz.Principal // principal to return once active (or on calls before failAt)
	lastUser int64
	lastID   string
}

func (f *fakeTokenValidator) RequireActive(_ context.Context, userID int64, id string, _ time.Time) (authz.Principal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastUser, f.lastID = userID, id
	if f.fail && (f.failAt == 0 || f.calls >= f.failAt) {
		return authz.Principal{}, errors.New("token no longer active")
	}
	return f.returned, nil
}

func (f *fakeTokenValidator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestMonitorWebsocketSessionActiveStaysOpen(t *testing.T) {
	validator := &fakeValidator{}
	ctx, cancel := context.WithCancel(context.Background())
	binding := dbSessionBinding{ID: "sess-1", UserID: 42}

	done := make(chan struct{})
	go func() {
		monitorWebsocketSession(ctx, cancel, fakeTxnManager{}, validator, binding, 5*time.Millisecond)
		close(done)
	}()

	// Let several revalidation ticks pass while the session stays active.
	time.Sleep(40 * time.Millisecond)
	select {
	case <-ctx.Done():
		t.Fatal("connection context was canceled while the session remained active")
	default:
	}
	if validator.callCount() < 2 {
		t.Fatalf("expected multiple RequireActive calls, got %d", validator.callCount())
	}

	// Clean shutdown: external cancellation must stop the goroutine promptly.
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitorWebsocketSession did not exit after external context cancellation — goroutine leak")
	}
}

func TestMonitorWebsocketSessionRevokedCancelsConnection(t *testing.T) {
	validator := &fakeValidator{failAt: 2} // active on tick 1, revoked from tick 2 on
	ctx, cancel := context.WithCancel(context.Background())
	binding := dbSessionBinding{ID: "sess-2", UserID: 7}

	done := make(chan struct{})
	go func() {
		monitorWebsocketSession(ctx, cancel, fakeTxnManager{}, validator, binding, 5*time.Millisecond)
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("connection context was not canceled after RequireActive started failing")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitorWebsocketSession goroutine did not exit after cancellation — goroutine leak")
	}
	if validator.lastUser != 7 || validator.lastID != "sess-2" {
		t.Fatalf("RequireActive called with wrong binding: user=%d id=%s", validator.lastUser, validator.lastID)
	}
}

func TestWebsocketSessionInitNoDBBindingSpawnsNoMonitor(t *testing.T) {
	validator := &fakeValidator{}
	initFn := websocketSessionInitWithInterval(fakeTxnManager{}, validator, &fakeTokenValidator{}, 5*time.Millisecond)

	// No dbSessionBinding in context: anonymous/legacy-authenticated connection.
	inCtx := context.Background()
	outCtx, payload, err := initFn(inCtx, gqlTransport.InitPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		t.Fatalf("expected nil ack payload, got %+v", payload)
	}
	if outCtx != inCtx {
		t.Fatal("expected the same context to be returned unmodified when no DB session binding is present (no monitor should be spawned)")
	}

	time.Sleep(30 * time.Millisecond)
	if validator.callCount() != 0 {
		t.Fatalf("RequireActive was called %d times for a connection with no DB session binding", validator.callCount())
	}
}

func TestWebsocketSessionInitWithDBBindingSpawnsMonitorAndCancelsOnRevocation(t *testing.T) {
	validator := &fakeValidator{failAt: 1} // fail immediately
	initFn := websocketSessionInitWithInterval(fakeTxnManager{}, validator, &fakeTokenValidator{}, 5*time.Millisecond)

	inCtx := context.WithValue(context.Background(), dbSessionBindingContextKey{}, dbSessionBinding{ID: "sess-3", UserID: 99})
	outCtx, _, err := initFn(inCtx, gqlTransport.InitPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if outCtx == inCtx {
		t.Fatal("expected a derived, cancelable context to be returned when a DB session binding is present")
	}

	select {
	case <-outCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("monitor did not cancel the connection context after RequireActive failed")
	}
}

func samplePrincipal(scopes ...authz.Capability) authz.Principal {
	p := authz.Principal{UserID: "99", Role: authz.RoleUser, Status: authz.StatusActive}
	if len(scopes) > 0 {
		p.TokenScopes = make(map[authz.Capability]struct{}, len(scopes))
		for _, s := range scopes {
			p.TokenScopes[s] = struct{}{}
		}
	}
	return p
}

func TestSameTokenPrincipal(t *testing.T) {
	base := samplePrincipal(authz.LibraryRead, authz.MediaStream)
	cases := []struct {
		name  string
		other authz.Principal
		want  bool
	}{
		{"identical", samplePrincipal(authz.LibraryRead, authz.MediaStream), true},
		{"different user id", authz.Principal{UserID: "1", Role: base.Role, Status: base.Status, TokenScopes: base.TokenScopes}, false},
		{"different role", authz.Principal{UserID: base.UserID, Role: authz.RoleModerator, Status: base.Status, TokenScopes: base.TokenScopes}, false},
		{"different status", authz.Principal{UserID: base.UserID, Role: base.Role, Status: authz.StatusDisabled, TokenScopes: base.TokenScopes}, false},
		{"fewer scopes (reduced)", samplePrincipal(authz.LibraryRead), false},
		{"more scopes", samplePrincipal(authz.LibraryRead, authz.MediaStream, authz.MetadataWrite), false},
		{"same count, different members", samplePrincipal(authz.LibraryRead, authz.MetadataWrite), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sameTokenPrincipal(base, c.other); got != c.want {
				t.Fatalf("sameTokenPrincipal=%v want=%v", got, c.want)
			}
		})
	}
}

func TestMonitorWebsocketTokenActiveStaysOpen(t *testing.T) {
	principal := samplePrincipal(authz.LibraryRead)
	validator := &fakeTokenValidator{returned: principal}
	ctx, cancel := context.WithCancel(context.Background())
	binding := dbTokenBinding{ID: "tok-1", UserID: 99, Principal: principal}

	done := make(chan struct{})
	go func() {
		monitorWebsocketToken(ctx, cancel, fakeTxnManager{}, validator, binding, 5*time.Millisecond)
		close(done)
	}()

	time.Sleep(40 * time.Millisecond)
	select {
	case <-ctx.Done():
		t.Fatal("connection context was canceled while the token remained active and unchanged")
	default:
	}
	if validator.callCount() < 2 {
		t.Fatalf("expected multiple RequireActive calls, got %d", validator.callCount())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitorWebsocketToken did not exit after external context cancellation — goroutine leak")
	}
}

func TestMonitorWebsocketTokenRevokedOrExpiredCancelsConnection(t *testing.T) {
	principal := samplePrincipal(authz.LibraryRead)
	validator := &fakeTokenValidator{returned: principal, fail: true, failAt: 2}
	ctx, cancel := context.WithCancel(context.Background())
	binding := dbTokenBinding{ID: "tok-2", UserID: 7, Principal: principal}

	done := make(chan struct{})
	go func() {
		monitorWebsocketToken(ctx, cancel, fakeTxnManager{}, validator, binding, 5*time.Millisecond)
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("connection context was not canceled after RequireActive started failing (revoked/expired/disabled)")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitorWebsocketToken goroutine did not exit after cancellation — goroutine leak")
	}
	if validator.lastUser != 7 || validator.lastID != "tok-2" {
		t.Fatalf("RequireActive called with wrong binding: user=%d id=%s", validator.lastUser, validator.lastID)
	}
}

func TestMonitorWebsocketTokenRoleOrScopeChangeCancelsConnection(t *testing.T) {
	original := samplePrincipal(authz.LibraryRead, authz.MediaStream)
	reduced := samplePrincipal(authz.LibraryRead) // e.g. role demotion revalidated fewer scopes
	validator := &fakeTokenValidator{returned: reduced}
	ctx, cancel := context.WithCancel(context.Background())
	binding := dbTokenBinding{ID: "tok-3", UserID: 99, Principal: original}

	done := make(chan struct{})
	go func() {
		monitorWebsocketToken(ctx, cancel, fakeTxnManager{}, validator, binding, 5*time.Millisecond)
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("connection context was not canceled after the token's revalidated principal diverged from its handshake-time principal (role/scope reduction)")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("monitorWebsocketToken goroutine did not exit after cancellation — goroutine leak")
	}
}

func TestWebsocketSessionInitNoBindingSpawnsNoMonitorAtAll(t *testing.T) {
	sessionValidator := &fakeValidator{}
	tokenValidator := &fakeTokenValidator{}
	initFn := websocketSessionInitWithInterval(fakeTxnManager{}, sessionValidator, tokenValidator, 5*time.Millisecond)

	inCtx := context.Background()
	outCtx, _, err := initFn(inCtx, gqlTransport.InitPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if outCtx != inCtx {
		t.Fatal("expected the same context back when neither a session nor token binding is present")
	}
	time.Sleep(30 * time.Millisecond)
	if sessionValidator.callCount() != 0 || tokenValidator.callCount() != 0 {
		t.Fatalf("a monitor ran with no binding present: session calls=%d token calls=%d", sessionValidator.callCount(), tokenValidator.callCount())
	}
}

func TestWebsocketSessionInitTokenBindingTakesPriorityAndSpawnsTokenMonitor(t *testing.T) {
	principal := samplePrincipal(authz.LibraryRead)
	sessionValidator := &fakeValidator{}
	tokenValidator := &fakeTokenValidator{returned: principal, fail: true, failAt: 1}
	initFn := websocketSessionInitWithInterval(fakeTxnManager{}, sessionValidator, tokenValidator, 5*time.Millisecond)

	// Simulate both bindings somehow present; token must take priority per
	// the real websocketSessionInitWithInterval control flow (bearer auth is
	// checked before the v2 cookie in authenticateHandler too).
	ctx := context.WithValue(context.Background(), dbTokenBindingContextKey{}, dbTokenBinding{ID: "tok-4", UserID: 1, Principal: principal})
	ctx = context.WithValue(ctx, dbSessionBindingContextKey{}, dbSessionBinding{ID: "sess-4", UserID: 1})

	outCtx, _, err := initFn(ctx, gqlTransport.InitPayload{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-outCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("token monitor did not cancel the connection")
	}
	if tokenValidator.callCount() == 0 {
		t.Fatal("expected the token monitor to run")
	}
	if sessionValidator.callCount() != 0 {
		t.Fatal("session monitor ran even though a token binding was present — token binding should take exclusive priority")
	}
}

package session

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func readMigrationToken(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 65 || b[64] != '\n' {
		t.Fatalf("unexpected handoff token format")
	}
	return string(b[:64])
}

func TestMigrationTokenHandoffExchangeReplayAndExpiry(t *testing.T) {
	ConsumeMigrationToken()
	t.Cleanup(ConsumeMigrationToken)
	dir := t.TempDir()
	now := time.Now()
	path, expiry, err := EnsureMigrationToken(now, dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0o600 || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("handoff must be a regular mode-0600 file: %v %v", info, err)
	}
	token := readMigrationToken(t, path)

	r := httptest.NewRequest("GET", "http://stash/?"+MigrationTokenQuery+"="+token, nil)
	r.RemoteAddr = "172.18.0.1:42000"
	r.Header.Set("Forwarded", "for=127.0.0.1")
	r.Header.Set("X-Forwarded-For", "127.0.0.1")
	r, sessionToken, csrf, exchanged := SetMigrationRequest(r, now)
	if !exchanged || !IsMigrationRequest(r.Context()) || len(sessionToken) != 64 || len(csrf) != 64 {
		t.Fatal("valid migration token did not atomically exchange")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("handoff file survived exchange")
	}
	if expiry != MigrationTokenExpiry() {
		t.Fatal("exchange changed the bounded expiry")
	}
	replay := httptest.NewRequest("GET", "http://stash/?"+MigrationTokenQuery+"="+token, nil)
	if _, _, _, ok := SetMigrationRequest(replay, now); ok {
		t.Fatal("exchange bearer was replayable")
	}

	cookieReq := httptest.NewRequest("POST", "http://stash/graphql", nil)
	cookieReq.AddCookie(&http.Cookie{Name: MigrationCookie, Value: sessionToken})
	cookieReq, _, _, _ = SetMigrationRequest(cookieReq, now)
	if !IsMigrationRequest(cookieReq.Context()) || !VerifyMigrationCSRF(csrf) {
		t.Fatal("migration session or CSRF was not recognized")
	}
	if VerifyMigrationCSRF(csrf + "0") {
		t.Fatal("forged CSRF was accepted")
	}
	expired := httptest.NewRequest("POST", "http://stash/graphql", nil)
	expired.AddCookie(&http.Cookie{Name: MigrationCookie, Value: sessionToken})
	expired, _, _, _ = SetMigrationRequest(expired, now.Add(migrationTokenTTL))
	if IsMigrationRequest(expired.Context()) {
		t.Fatal("expired migration session was accepted")
	}
}

func TestEnsureMigrationTokenPreservesLiveExchangedSession(t *testing.T) {
	ConsumeMigrationToken()
	t.Cleanup(ConsumeMigrationToken)
	dir := t.TempDir()
	now := time.Now()
	path, expiry, err := EnsureMigrationToken(now, dir)
	if err != nil {
		t.Fatal(err)
	}
	bearer := readMigrationToken(t, path)
	exchange := httptest.NewRequest(http.MethodGet, "http://stash/?"+MigrationTokenQuery+"="+bearer, nil)
	_, sessionToken, csrf, exchanged := SetMigrationRequest(exchange, now)
	if !exchanged {
		t.Fatal("initial bearer exchange failed")
	}

	const ensureWorkers = 32
	type ensureResult struct {
		path   string
		expiry time.Time
		err    error
	}
	start := make(chan struct{})
	results := make(chan ensureResult, ensureWorkers)
	var wg sync.WaitGroup
	for i := 0; i < ensureWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ensuredPath, ensuredExpiry, ensureErr := EnsureMigrationToken(now.Add(time.Second), dir)
			results <- ensureResult{path: ensuredPath, expiry: ensuredExpiry, err: ensureErr}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent migration-window ensure invalidated live session: %v", result.err)
		}
		if result.path != "" || result.expiry != expiry {
			t.Fatalf("concurrent ensure recreated handoff or changed expiry: path=%q expiry=%v", result.path, result.expiry)
		}
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("concurrent ensure recreated the consumed bearer handoff")
	}

	authenticated := httptest.NewRequest(http.MethodPost, "http://stash/graphql", nil)
	authenticated.AddCookie(&http.Cookie{Name: MigrationCookie, Value: sessionToken})
	authenticated, _, _, _ = SetMigrationRequest(authenticated, now.Add(time.Second))
	if !IsMigrationRequest(authenticated.Context()) || !VerifyMigrationCSRF(csrf) {
		t.Fatal("concurrent ensure destroyed the live migration session or CSRF nonce")
	}
	if AcquireMigrationLease() == false {
		t.Fatal("concurrent ensure destroyed migration execution authority")
	}
}

func TestMigrationTokenRejectsSymlinkAndExistingHandoff(t *testing.T) {
	ConsumeMigrationToken()
	t.Cleanup(ConsumeMigrationToken)
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "config")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureMigrationToken(time.Now(), linkDir); err == nil {
		t.Fatal("symlinked config directory was accepted")
	}

	ConsumeMigrationToken()
	handoff := filepath.Join(realDir, MigrationTokenFile)
	if err := os.WriteFile(handoff, []byte("do-not-overwrite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureMigrationToken(time.Now(), realDir); err == nil {
		t.Fatal("pre-existing handoff was overwritten")
	}
	b, _ := os.ReadFile(handoff)
	if string(b) != "do-not-overwrite" {
		t.Fatal("pre-existing handoff contents changed")
	}
}

func TestMigrationTokenConcurrentExchangeAndLease(t *testing.T) {
	ConsumeMigrationToken()
	t.Cleanup(ConsumeMigrationToken)
	now := time.Now()
	path, _, err := EnsureMigrationToken(now, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	token := readMigrationToken(t, path)

	const workers = 24
	start := make(chan struct{})
	results := make(chan bool, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := httptest.NewRequest("GET", "http://stash/?"+MigrationTokenQuery+"="+token, nil)
			_, _, _, exchanged := SetMigrationRequest(r, now)
			results <- exchanged
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for won := range results {
		if won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected one exchange winner, got %d", winners)
	}
	if !AcquireMigrationLease() || AcquireMigrationLease() {
		t.Fatal("migration execution lease was not exclusive")
	}
	ReleaseMigrationLease()
	if !AcquireMigrationLease() {
		t.Fatal("released migration lease was not retryable")
	}
}

func TestMigrationTokenFailsClosedWhenHandoffCannotBeDeleted(t *testing.T) {
	ConsumeMigrationToken()
	t.Cleanup(ConsumeMigrationToken)
	now := time.Now()
	path, _, err := EnsureMigrationToken(now, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	token := readMigrationToken(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://stash/?"+MigrationTokenQuery+"="+token, nil)
	request, sessionToken, csrf, exchanged := SetMigrationRequest(request, now)
	if exchanged || sessionToken != "" || csrf != "" || IsMigrationRequest(request.Context()) {
		t.Fatal("exchange succeeded while plaintext handoff remained")
	}
}

func TestMigrationTokenRestartInvalidatesPriorBearer(t *testing.T) {
	ConsumeMigrationToken()
	t.Cleanup(ConsumeMigrationToken)
	now := time.Now()
	dir := t.TempDir()
	path, _, err := EnsureMigrationToken(now, dir)
	if err != nil {
		t.Fatal(err)
	}
	old := readMigrationToken(t, path)
	ConsumeMigrationToken()
	path, _, err = EnsureMigrationToken(now.Add(time.Second), dir)
	if err != nil {
		t.Fatal(err)
	}
	if replacement := readMigrationToken(t, path); replacement == old {
		t.Fatal("restart simulation reused the prior bearer")
	}
	request := httptest.NewRequest(http.MethodGet, "http://stash/?"+MigrationTokenQuery+"="+old, nil)
	if _, _, _, exchanged := SetMigrationRequest(request, now.Add(time.Second)); exchanged {
		t.Fatal("bearer from a prior process state was accepted")
	}
}

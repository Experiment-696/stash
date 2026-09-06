package session

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/logger"
)

type bootstrapLogCapture struct {
	logger.BasicLogger
	mu      sync.Mutex
	entries []string
}

func (l *bootstrapLogCapture) Infof(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *bootstrapLogCapture) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func TestBootstrapTokenIsExplicitExpiringAndSingleUse(t *testing.T) {
	ConsumeBootstrapToken()
	now := time.Unix(1_700_000_000, 0)
	token, err := EnsureBootstrapToken(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 {
		t.Fatalf("expected 256-bit hex token, got length %d", len(token))
	}

	remote := httptest.NewRequest("POST", "http://stash/graphql", nil)
	remote.RemoteAddr = "172.30.16.1:54321"
	remote.Header.Set("X-Forwarded-For", "127.0.0.1")
	remote.Header.Set("Forwarded", "for=127.0.0.1")
	remote.Header.Set(BootstrapTokenHeader, token)
	remote = SetBootstrapRequest(remote, now)
	if !IsBootstrapRequest(remote.Context()) {
		t.Fatal("valid explicit token was denied across a forwarded/NAT connection")
	}

	browser := httptest.NewRequest(
		"GET",
		"http://stash/?"+BootstrapTokenQuery+"="+token,
		nil,
	)
	browser = SetBootstrapRequest(browser, now)
	if !IsBootstrapRequest(browser.Context()) {
		t.Fatal("valid browser bootstrap-token exchange was denied")
	}

	recorder := httptest.NewRecorder()
	SetBootstrapCookie(recorder, token, now.Add(bootstrapTokenTTL))
	response := recorder.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("bootstrap cookie is not HttpOnly and SameSite=Strict")
	}

	forged := httptest.NewRequest("POST", "http://stash/graphql", nil)
	forged.Header.Set("X-Forwarded-For", "127.0.0.1")
	forged.Header.Set(BootstrapTokenHeader, token+"0")
	if IsBootstrapRequest(SetBootstrapRequest(forged, now).Context()) {
		t.Fatal("forged token or forwarding header created bootstrap authority")
	}

	expired := httptest.NewRequest("POST", "http://stash/graphql", nil)
	expired.Header.Set(BootstrapTokenHeader, token)
	if IsBootstrapRequest(SetBootstrapRequest(expired, now.Add(bootstrapTokenTTL)).Context()) {
		t.Fatal("expired bootstrap token remained valid")
	}

	ConsumeBootstrapToken()
	replay := httptest.NewRequest("POST", "http://stash/graphql", nil)
	replay.Header.Set(BootstrapTokenHeader, token)
	if IsBootstrapRequest(SetBootstrapRequest(replay, now).Context()) {
		t.Fatal("consumed bootstrap token was replayable")
	}
}

func TestBootstrapTokenRestartRotationAndLogging(t *testing.T) {
	ConsumeBootstrapToken()
	originalLogger := logger.Logger
	capture := &bootstrapLogCapture{}
	logger.Logger = capture
	t.Cleanup(func() {
		logger.Logger = originalLogger
		ConsumeBootstrapToken()
	})

	now := time.Unix(1_700_000_000, 0)
	first, err := EnsureBootstrapToken(now)
	if err != nil {
		t.Fatal(err)
	}
	if repeated, err := EnsureBootstrapToken(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	} else if repeated != first {
		t.Fatal("active bootstrap token rotated before expiry")
	}

	entries := capture.snapshot()
	if len(entries) != 1 || strings.Contains(entries[0], first) {
		t.Fatalf("operator log must record creation without the raw token, got %q", entries)
	}

	// Process restart semantics are represented by clearing all in-memory
	// bootstrap state. A new process must issue a fresh token and the old
	// process token must no longer authorize a request.
	ConsumeBootstrapToken()
	second, err := EnsureBootstrapToken(now.Add(2 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("bootstrap token was reused after in-memory state reset")
	}

	oldRequest := httptest.NewRequest("POST", "http://stash/graphql", nil)
	oldRequest.Header.Set(BootstrapTokenHeader, first)
	if IsBootstrapRequest(SetBootstrapRequest(oldRequest, now.Add(2*time.Minute)).Context()) {
		t.Fatal("pre-restart bootstrap token remained valid")
	}
	if entries = capture.snapshot(); len(entries) != 2 ||
		strings.Contains(entries[1], second) || strings.Contains(entries[1], first) {
		t.Fatalf("restart log disclosed a setup token: %q", entries)
	}
}

func TestBootstrapTokenConcurrentConsumptionFailsClosed(t *testing.T) {
	ConsumeBootstrapToken()
	t.Cleanup(ConsumeBootstrapToken)

	now := time.Unix(1_700_000_000, 0)
	token, err := EnsureBootstrapToken(now)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			ConsumeBootstrapToken()
		}()
	}
	close(start)
	wg.Wait()

	replay := httptest.NewRequest("POST", "http://stash/graphql", nil)
	replay.Header.Set(BootstrapTokenHeader, token)
	if IsBootstrapRequest(SetBootstrapRequest(replay, now).Context()) {
		t.Fatal("bootstrap token remained valid after concurrent consumption")
	}
	if !BootstrapTokenExpiry().IsZero() {
		t.Fatal("concurrent consumption left bootstrap expiry state behind")
	}
}

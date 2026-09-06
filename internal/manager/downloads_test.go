package manager

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
)

func downloadTestPrincipal(id string, role authz.Role) authz.Principal {
	return authz.Principal{UserID: id, Role: role, Status: authz.StatusActive}
}

func registerDownloadFixture(t *testing.T, store *DownloadStore, principal authz.Principal) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "download.txt")
	if err := os.WriteFile(path, []byte("download payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := store.RegisterFile(path, "text/plain", true, principal, authz.DataAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 32 {
		t.Fatalf("download token has only %d characters, want at least 128 bits", len(token))
	}
	return token
}

func serveDownload(store *DownloadStore, token string, principal authz.Principal) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/downloads/"+token+"/download.txt", nil)
	store.Serve(token, principal, recorder, request)
	return recorder
}

func TestDownloadStoreBindsPrincipalCapabilityExpiryAndReplay(t *testing.T) {
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	store := NewDownloadStore()
	store.now = func() time.Time { return now }
	store.tokenTTL = time.Minute
	admin := downloadTestPrincipal("1", authz.RoleAdmin)
	if _, err := store.RegisterFile("ignored", "", true, downloadTestPrincipal("1", authz.RoleModerator), authz.DataAdmin); err == nil {
		t.Fatal("registered data.admin download for Moderator")
	}

	for name, principal := range map[string]authz.Principal{
		"anonymous":  {},
		"wrong user": downloadTestPrincipal("2", authz.RoleAdmin),
		"downgraded": downloadTestPrincipal("1", authz.RoleModerator),
		"disabled":   {UserID: "1", Role: authz.RoleAdmin, Status: authz.StatusDisabled},
	} {
		t.Run(name, func(t *testing.T) {
			token := registerDownloadFixture(t, store, admin)
			if got := serveDownload(store, token, principal).Code; got != http.StatusNotFound {
				t.Fatalf("status = %d, want indistinguishable 404", got)
			}
		})
	}

	expired := registerDownloadFixture(t, store, admin)
	now = now.Add(time.Minute)
	if got := serveDownload(store, expired, admin).Code; got != http.StatusNotFound {
		t.Fatalf("expired status = %d, want 404", got)
	}

	token := registerDownloadFixture(t, store, admin)
	if got := serveDownload(store, token, admin).Code; got != http.StatusOK {
		t.Fatalf("first download status = %d, want 200", got)
	}
	if got := serveDownload(store, token, admin).Code; got != http.StatusNotFound {
		t.Fatalf("replay status = %d, want 404", got)
	}
}

func TestDownloadStoreConcurrentConsumeHasSingleWinner(t *testing.T) {
	store := NewDownloadStore()
	admin := downloadTestPrincipal("1", authz.RoleAdmin)
	token := registerDownloadFixture(t, store, admin)

	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if serveDownload(store, token, admin).Code == http.StatusOK {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful concurrent downloads = %d, want exactly 1", got)
	}
}

func TestDownloadStorePreservesHTTPRangeOnFirstUse(t *testing.T) {
	store := NewDownloadStore()
	admin := downloadTestPrincipal("1", authz.RoleAdmin)
	token := registerDownloadFixture(t, store, admin)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/downloads/"+token+"/download.txt", nil)
	request.Header.Set("Range", "bytes=0-7")
	store.Serve(token, admin, recorder, request)
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d, want 206", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Range"); got == "" {
		t.Fatal("range response omitted Content-Range")
	}
}

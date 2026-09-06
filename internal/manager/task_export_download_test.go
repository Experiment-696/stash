package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/authz"
)

func TestDelayedExportRegistrationUsesPrincipalSnapshotAfterRequestCancellation(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	requestContext = context.WithValue(requestContext, struct{ name string }{"unrelated"}, "request-only-data")
	principal := downloadTestPrincipal("1", authz.RoleAdmin)
	principal.TokenScopes = map[authz.Capability]struct{}{authz.DataAdmin: {}}
	snapshot := principal
	snapshot.TokenScopes = cloneCapabilities(principal.TokenScopes)
	task := &ExportTask{
		principal: snapshot,
		downloads: NewDownloadStore(),
	}
	delete(principal.TokenScopes, authz.DataAdmin)
	cancel()
	if requestContext.Err() == nil {
		t.Fatal("request context was not canceled")
	}

	path := filepath.Join(t.TempDir(), "delayed-export.zip")
	if err := os.WriteFile(path, []byte("export"), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := task.registerDownload(path)
	if err != nil {
		t.Fatalf("delayed registration after request cancellation failed: %v", err)
	}
	if token == "" {
		t.Fatal("delayed registration returned an empty token")
	}
	if task.principal.UserID != principal.UserID || task.principal.Role != principal.Role ||
		task.principal.Status != principal.Status || !task.principal.Allows(authz.DataAdmin) {
		t.Fatal("task did not retain the immutable principal snapshot")
	}
}

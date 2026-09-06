package manager

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/dlna"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func TestDLNAFailsClosedWhenMultiuserAccountsExist(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetBool(config.DLNADefaultEnabled, true)
	database := sqlite.NewDatabase()
	path := filepath.Join(t.TempDir(), "dlna.sqlite")
	if err := database.Open(path); err != nil {
		t.Fatal(err)
	}
	service := dlna.NewService(dlna.NewRepository(database.Repository()), cfg, nil, database.Scene, cfg.GetMinimumPlayPercent())
	manager := &Manager{Config: cfg, Database: database, DLNAService: service}
	if !manager.dlnaAllowedForCurrentAccountMode() {
		t.Fatal("zero-user setup mode unexpectedly disabled DLNA")
	}
	manager.RefreshDLNA()
	if !service.IsRunning() {
		t.Fatal("zero-user legacy mode did not start DLNA")
	}
	t.Cleanup(func() { service.Stop(nil) })
	if err := txn.WithTxn(context.Background(), database, func(ctx context.Context) error {
		_, err := database.User.Create(ctx, "dlna-admin", "correct-password", authz.RoleAdmin)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if manager.dlnaAllowedForCurrentAccountMode() {
		t.Fatal("DLNA remained enabled without a principal-to-media.stream binding")
	}
	manager.RefreshDLNA()
	if service.IsRunning() {
		t.Fatal("first account transition did not stop a running DLNA service")
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := sqlite.NewDatabase()
	if err := reopened.Open(path); err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restartedService := dlna.NewService(dlna.NewRepository(reopened.Repository()), cfg, nil, reopened.Scene, cfg.GetMinimumPlayPercent())
	restartedManager := &Manager{Config: cfg, Database: reopened, DLNAService: restartedService}
	if restartedManager.dlnaAllowedForCurrentAccountMode() {
		t.Fatal("DLNA became available after restart with existing accounts")
	}
	restartedManager.RefreshDLNA()
	if restartedService.IsRunning() {
		restartedService.Stop(nil)
		t.Fatal("persisted-account restart started DLNA")
	}
}

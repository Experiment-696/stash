package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func TestCamShowRepositoryContractsSurviveRestart(t *testing.T) {
	config.InitializeEmpty()
	path := filepath.Join(t.TempDir(), "cam-shows.sqlite")
	db := NewDatabase()
	if err := db.Open(path); err != nil {
		t.Fatal(err)
	}
	write, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	site, err := db.CamShow.CreateSite(write, "Example", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model, err := db.CamShow.CreateModel(write, "Example Model", nil)
	if err != nil {
		t.Fatal(err)
	}
	scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Scene.Create(write, scene, nil); err != nil {
		t.Fatal(err)
	}
	show, err := db.CamShow.CreateShow(write, int64(scene.ID), "RECORDED", &site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.LinkModel(write, show.ID, model.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.UpdateShowCategory(write, show.ID, "ARCHIVE"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(write); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restarted := NewDatabase()
	if err := restarted.Open(path); err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	read, err := restarted.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.CamShow.FindShowByScene(read, int64(scene.ID))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != show.ID || got.SiteID == nil || *got.SiteID != site.ID {
		t.Fatalf("restarted show=%+v", got)
	}
	if got.Category != "ARCHIVE" {
		t.Fatalf("category=%q", got.Category)
	}
	if shows, err := restarted.CamShow.ListShows(read); err != nil || len(shows) != 1 {
		t.Fatalf("shows=%+v err=%v", shows, err)
	}
}

func TestCamIdentityLifecycleOwnerIsolationAndBackupRestore(t *testing.T) {
	config.InitializeEmpty()
	dir := t.TempDir()
	path := filepath.Join(dir, "cam-identity.sqlite")
	backup := filepath.Join(dir, "cam-identity.backup.sqlite")
	db := NewDatabase()
	if err := db.Open(path); err != nil {
		t.Fatal(err)
	}
	write, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.User.Create(write, "cam-one", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.User.Create(write, "cam-two", "password-two", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	site, err := db.CamShow.CreateSite(write, "Site", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model, err := db.CamShow.CreateModel(write, "Model", nil)
	if err != nil {
		t.Fatal(err)
	}
	account, err := db.CamShow.CreateAccount(write, model.ID, site.ID, "  Ｍodel  ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if account.NormalizedHandle != "model" {
		t.Fatalf("normalized handle=%q", account.NormalizedHandle)
	}
	if _, err := db.CamShow.CreateAccount(write, model.ID, site.ID, "MODEL", nil); err == nil {
		t.Fatal("duplicate active normalized handle accepted")
	}
	closedAt := time.Now().UTC()
	if err := db.CamShow.CloseAccount(write, account.ID, closedAt); err != nil {
		t.Fatal(err)
	}
	reused, err := db.CamShow.CreateAccount(write, model.ID, site.ID, "model", &closedAt)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := db.CamShow.CreateAlias(write, model.ID, &reused.ID, &site.ID, "  Ａlias ")
	if err != nil {
		t.Fatal(err)
	}
	if alias.NormalizedAlias != "alias" {
		t.Fatalf("normalized alias=%q", alias.NormalizedAlias)
	}
	if err := db.CamShow.RetireAlias(write, alias.ID, closedAt); err != nil {
		t.Fatal(err)
	}
	rating := 90
	if err := db.CamShow.SetUserState(write, first.ID, model.ID, true, &rating); err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.SetUserState(write, second.ID, model.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.SetSiteEnabled(write, site.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.SetModelStatus(write, model.ID, "INACTIVE"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(write); err != nil {
		t.Fatal(err)
	}
	read, err := db.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	exported, err := db.CamShow.ExportIdentityState(read)
	if err != nil {
		t.Fatal(err)
	}
	if sites, err := db.CamShow.ListSites(read); err != nil || len(sites) != 1 || sites[0].Enabled {
		t.Fatalf("sites=%+v err=%v", sites, err)
	}
	if models, err := db.CamShow.ListModels(read); err != nil || len(models) != 1 || models[0].Status != "INACTIVE" {
		t.Fatalf("models=%+v err=%v", models, err)
	}
	if accounts, err := db.CamShow.ListAccounts(read, model.ID); err != nil || len(accounts) != 2 {
		t.Fatalf("accounts=%+v err=%v", accounts, err)
	}
	if aliases, err := db.CamShow.ListAliases(read, model.ID); err != nil || len(aliases) != 1 || aliases[0].IsCurrent {
		t.Fatalf("aliases=%+v err=%v", aliases, err)
	}
	if len(exported.Accounts) != 2 || len(exported.Aliases) != 1 || len(exported.States) != 2 {
		t.Fatalf("export=%+v", exported)
	}
	importWrite, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.ImportIdentityState(importWrite, *exported); err != nil {
		t.Fatalf("idempotent import: %v", err)
	}
	conflict := *exported
	conflict.Accounts = append([]CamModelAccount(nil), exported.Accounts...)
	conflict.Aliases = append([]CamModelAlias(nil), exported.Aliases...)
	newAccount := conflict.Accounts[0]
	newAccount.ID = 999
	newAccount.Handle = "rollback-proof"
	newAccount.NormalizedHandle = "rollback-proof"
	newAccount.Status = "INACTIVE"
	newAccount.ValidTo = &closedAt
	conflict.Accounts = append(conflict.Accounts, newAccount)
	conflict.Aliases[0].Alias = "conflicting overwrite"
	if err := db.CamShow.ImportIdentityState(importWrite, conflict); err == nil {
		t.Fatal("conflicting import succeeded")
	}
	if got, err := db.CamShow.FindAccount(importWrite, 999); err != nil || got != nil {
		t.Fatalf("failed import did not roll back: account=%+v err=%v", got, err)
	}
	invalidReference := *exported
	invalidReference.Accounts = append(append([]CamModelAccount(nil), exported.Accounts...), newAccount)
	invalidReference.States = append(append([]CamModelUserState(nil), exported.States...), CamModelUserState{UserID: 999999, ModelID: model.ID, Favorite: true, UpdatedAt: time.Now().UTC()})
	if err := db.CamShow.ImportIdentityState(importWrite, invalidReference); err == nil {
		t.Fatal("invalid-reference import succeeded")
	}
	if got, err := db.CamShow.FindAccount(importWrite, 999); err != nil || got != nil {
		t.Fatalf("invalid-reference import did not roll back: account=%+v err=%v", got, err)
	}
	if err := db.Commit(importWrite); err != nil {
		t.Fatal(err)
	}
	firstState, err := db.CamShow.GetUserState(read, first.ID, model.ID)
	if err != nil || firstState == nil || !firstState.Favorite {
		t.Fatalf("first state=%+v err=%v", firstState, err)
	}
	secondState, err := db.CamShow.GetUserState(read, second.ID, model.ID)
	if err != nil || secondState == nil || secondState.Favorite {
		t.Fatalf("second state=%+v err=%v", secondState, err)
	}
	if err := db.Backup(backup); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}
	restored := NewDatabase()
	if err := restored.Open(path); err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredRead, err := restored.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restoredExport, err := restored.CamShow.ExportIdentityState(restoredRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredExport.Accounts) != 2 || len(restoredExport.Aliases) != 1 || len(restoredExport.States) != 2 {
		t.Fatalf("restored export=%+v", restoredExport)
	}
}

func TestCamSnapshotRestoresFreshDatabaseWithStableIDs(t *testing.T) {
	config.InitializeEmpty()
	build := func(path, username string) (*Database, *User, *models.Scene) {
		db := NewDatabase()
		if err := db.Open(path); err != nil {
			t.Fatal(err)
		}
		ctx, err := db.Begin(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		user, err := db.User.Create(ctx, username, "snapshot-password", authz.RoleUser)
		if err != nil {
			t.Fatal(err)
		}
		scene := &models.Scene{CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := db.Scene.Create(ctx, scene, nil); err != nil {
			t.Fatal(err)
		}
		if err := db.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return db, user, scene
	}
	source, user, scene := build(filepath.Join(t.TempDir(), "source.sqlite"), "snapshot-user")
	defer source.Close()
	write, err := source.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	site, err := source.CamShow.CreateSite(write, "Snapshot Site", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model, err := source.CamShow.CreateModel(write, "Snapshot Model", nil)
	if err != nil {
		t.Fatal(err)
	}
	show, err := source.CamShow.CreateShow(write, int64(scene.ID), "RECORDED", &site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.CamShow.LinkModel(write, show.ID, model.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	account, err := source.CamShow.CreateAccount(write, model.ID, site.ID, "snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := source.CamShow.CreateAlias(write, model.ID, &account.ID, &site.ID, "snapshot alias")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(write, `UPDATE cam_model_accounts SET profile_url=?, external_account_id=?, first_seen_at=?, last_seen_at=?, last_synced_at=?, source=?, confidence=? WHERE id=?`, "https://example.test/profile", "external-account", time.Now().UTC().Add(-3*time.Hour), time.Now().UTC().Add(-2*time.Hour), time.Now().UTC().Add(-time.Hour), "snapshot-provider", 0.875, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(write, `UPDATE cam_model_aliases SET valid_from=?, source=?, confidence=?, last_verified_at=? WHERE id=?`, time.Now().UTC().Add(-4*time.Hour), "snapshot-alias-provider", 0.625, time.Now().UTC().Add(-30*time.Minute), alias.ID); err != nil {
		t.Fatal(err)
	}
	if err := source.CamShow.SetUserState(write, user.ID, model.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := source.Commit(write); err != nil {
		t.Fatal(err)
	}
	if _, err := source.CamShow.ExportSnapshot(context.Background()); err == nil {
		t.Fatal("snapshot export outside a transaction succeeded")
	}
	read, err := source.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.CamShow.ExportSnapshot(read)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Commit(read); err != nil {
		t.Fatal(err)
	}

	targetPath := filepath.Join(t.TempDir(), "target.sqlite")
	target, targetUser, targetScene := build(targetPath, "snapshot-target")
	if targetUser.ID != user.ID || targetScene.ID != scene.ID {
		t.Fatal("canonical fixture IDs differ")
	}
	if err := target.CamShow.ImportSnapshot(context.Background(), *snapshot); err == nil {
		t.Fatal("snapshot import outside a transaction succeeded")
	}
	importCtx, err := target.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	snapshotBytes, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var incoherent CamSnapshot
	if err := json.Unmarshal(snapshotBytes, &incoherent); err != nil {
		t.Fatal(err)
	}
	for tableIndex := range incoherent.Tables {
		if incoherent.Tables[tableIndex].Name == "cam_model_aliases" {
			incoherent.Tables[tableIndex].Rows[0][1] = float64(model.ID + 100)
		}
	}
	rollbackFailure := errors.New("injected snapshot rollback failure")
	rollbackExec := func(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
		result, execErr := dbWrapper.Exec(ctx, query, args...)
		if strings.HasPrefix(query, "ROLLBACK TO ") {
			return result, rollbackFailure
		}
		return result, execErr
	}
	if err := target.CamShow.importSnapshot(importCtx, incoherent, rollbackExec); !errors.Is(err, rollbackFailure) || !strings.Contains(err.Error(), "account/model/site mismatch") {
		t.Fatalf("rollback failure not joined with import error: %v", err)
	}
	if err := target.CamShow.ImportSnapshot(importCtx, incoherent); err == nil {
		t.Fatal("incoherent alias snapshot imported")
	}
	if sites, err := target.CamShow.ListSites(importCtx); err != nil || len(sites) != 0 {
		t.Fatalf("failed full import did not roll back sites=%+v err=%v", sites, err)
	}
	releaseFailure := errors.New("injected snapshot release failure")
	releaseExec := func(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
		result, execErr := dbWrapper.Exec(ctx, query, args...)
		if strings.HasPrefix(query, "RELEASE ") {
			return result, releaseFailure
		}
		return result, execErr
	}
	if err := target.CamShow.importSnapshot(importCtx, *snapshot, releaseExec); !errors.Is(err, releaseFailure) {
		t.Fatalf("release failure not propagated: %v", err)
	}
	if err := target.CamShow.ImportSnapshot(importCtx, *snapshot); err != nil {
		t.Fatalf("idempotent full import: %v", err)
	}
	roundTrip, err := target.CamShow.ExportSnapshot(importCtx)
	if err != nil {
		t.Fatal(err)
	}
	roundTripBytes, err := json.Marshal(roundTrip)
	if err != nil {
		t.Fatal(err)
	}
	if string(roundTripBytes) != string(snapshotBytes) {
		t.Fatalf("restored snapshot differs from source\nsource=%s\nrestored=%s", snapshotBytes, roundTripBytes)
	}
	newSite, err := target.CamShow.CreateSite(importCtx, "After Import", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if newSite.ID <= site.ID {
		t.Fatalf("sqlite sequence did not advance: %d <= %d", newSite.ID, site.ID)
	}
	if err := target.Commit(importCtx); err != nil {
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := NewDatabase()
	if err := restarted.Open(targetPath); err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restartRead, err := restarted.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	shows, err := restarted.CamShow.ListShows(restartRead)
	if err != nil || len(shows) != 1 || shows[0].ID != show.ID {
		t.Fatalf("restored shows=%+v err=%v", shows, err)
	}
}

func TestCamSnapshotUsesOneCallerOwnedTransactionDuringConcurrentWrite(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "snapshot-concurrency.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer seedCancel()
	seed, err := db.Begin(seedCtx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CamShow.CreateSite(seed, "Before Snapshot", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit(seed); err != nil {
		t.Fatal(err)
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	read, err := db.Begin(readCtx, false)
	if err != nil {
		t.Fatal(err)
	}
	readOpen := true
	defer func() {
		if readOpen {
			_ = db.Rollback(read)
		}
	}()
	before, err := db.CamShow.ExportSnapshot(read)
	if err != nil {
		t.Fatal(err)
	}
	if got := camSnapshotRowCount(t, before, "cam_sites"); got != 1 {
		t.Fatalf("initial site count=%d", got)
	}

	writerDone := make(chan error, 1)
	writerCtx, writerCancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer writerCancel()
	go func() {
		write, err := db.Begin(writerCtx, true)
		if err == nil {
			_, err = db.CamShow.CreateSite(write, "During Snapshot", nil, nil)
		}
		if err == nil {
			err = db.Commit(write)
		} else if write != nil {
			_ = db.Rollback(write)
		}
		writerDone <- err
	}()
	writerCompleted := false
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("concurrent writer: %v", err)
		}
		writerCompleted = true
	case <-time.After(250 * time.Millisecond):
	}

	during, err := db.CamShow.ExportSnapshot(read)
	if err != nil {
		t.Fatal(err)
	}
	if got := camSnapshotRowCount(t, during, "cam_sites"); got != 1 {
		t.Fatalf("active snapshot observed concurrent row: sites=%d", got)
	}
	if err := db.Commit(read); err != nil {
		t.Fatal(err)
	}
	readOpen = false
	if !writerCompleted {
		select {
		case err := <-writerDone:
			if err != nil {
				t.Fatalf("writer after reader completion: %v", err)
			}
		case <-writerCtx.Done():
			t.Fatalf("writer timed out after reader completion: %v", writerCtx.Err())
		}
	}

	freshCtx, freshCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer freshCancel()
	fresh, err := db.Begin(freshCtx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(fresh)
	after, err := db.CamShow.ExportSnapshot(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if got := camSnapshotRowCount(t, after, "cam_sites"); got != 2 {
		t.Fatalf("fresh snapshot site count=%d, want 2", got)
	}
}

func camSnapshotRowCount(t *testing.T, snapshot *CamSnapshot, tableName string) int {
	t.Helper()
	for _, table := range snapshot.Tables {
		if table.Name == tableName {
			return len(table.Rows)
		}
	}
	t.Fatalf("snapshot table %s missing", tableName)
	return 0
}

func TestCamShowLibraryJoinsSceneCategoryAndTagNames(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "cam-show-library.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(ctx)
	tag := &models.Tag{Name: "Captured Live", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Tag.Create(ctx, &models.CreateTagInput{Tag: tag}); err != nil {
		t.Fatal(err)
	}
	title := "Owner capture"
	scene := &models.Scene{Title: title, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Scene.Create(ctx, scene, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CamShow.CreateShow(ctx, int64(scene.ID), "LIVE CAPTURE", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(ctx, `INSERT INTO scenes_tags(scene_id,tag_id) VALUES(?,?)`, scene.ID, tag.ID); err != nil {
		t.Fatal(err)
	}
	shows, err := db.CamShow.ListShowLibrary(ctx)
	if err != nil || len(shows) != 1 || shows[0].SceneID != int64(scene.ID) || shows[0].Title != title || shows[0].Category != "LIVE CAPTURE" || len(shows[0].Tags) != 1 || shows[0].Tags[0].Name != tag.Name {
		t.Fatalf("shows=%+v err=%v", shows, err)
	}
}

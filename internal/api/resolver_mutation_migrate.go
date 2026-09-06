package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/task"
	"github.com/stashapp/stash/pkg/job"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/utils"
)

func (r *mutationResolver) MigrateSceneScreenshots(ctx context.Context, input MigrateSceneScreenshotsInput) (string, error) {
	mgr := manager.GetInstance()
	t := &task.MigrateSceneScreenshotsJob{
		ScreenshotsPath: manager.GetInstance().Paths.Generated.Screenshots,
		Input: scene.MigrateSceneScreenshotsInput{
			DeleteFiles:       utils.IsTrue(input.DeleteFiles),
			OverwriteExisting: utils.IsTrue(input.OverwriteExisting),
		},
		SceneRepo:  mgr.Repository.Scene,
		TxnManager: mgr.Repository.TxnManager,
	}
	jobID := mgr.JobManager.Add(ctx, "Migrating scene screenshots to blobs...", t)

	return strconv.Itoa(jobID), nil
}

func (r *mutationResolver) MigrateBlobs(ctx context.Context, input MigrateBlobsInput) (string, error) {
	mgr := manager.GetInstance()
	t := &task.MigrateBlobsJob{
		TxnManager: mgr.Database,
		BlobStore:  mgr.Database.Blobs,
		Vacuumer:   mgr.Database,
		DeleteOld:  utils.IsTrue(input.DeleteOld),
	}
	jobID := mgr.JobManager.Add(ctx, "Migrating blobs...", t)

	return strconv.Itoa(jobID), nil
}

func (r *mutationResolver) Migrate(ctx context.Context) (string, error) {
	mgr := manager.GetInstance()
	if !migrationWindowOpen(mgr.Database) {
		return "", errors.New("database migration is not required")
	}
	t := &task.MigrateJob{
		BackupPath: "",
		Config:     mgr.Config,
		Database:   mgr.Database,
	}
	if session.IsMigrationRequest(ctx) {
		// The reduced migration principal cannot read the general job surface.
		// Execute inline so failures return to the migration form and success can
		// immediately hand off to persisted database authentication.
		if !session.AcquireMigrationLease() {
			return "", errors.New("migration execution is already in progress")
		}
		if !migrationWindowOpen(mgr.Database) {
			session.ReleaseMigrationLease()
			return "", errors.New("database migration is not required")
		}
		if err := t.Execute(ctx, &job.Progress{}); err != nil {
			session.ReleaseMigrationLease()
			return "", err
		}
		session.ConsumeMigrationToken()
		if response, ok := ctx.Value(migrationResponseWriterContextKey{}).(http.ResponseWriter); ok {
			session.ClearMigrationCookie(response)
		}
		return "migration-complete", nil
	}

	jobID := mgr.JobManager.Add(ctx, "Migrating database...", t)

	return strconv.Itoa(jobID), nil
}

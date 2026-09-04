package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	encodinghex "encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/cammodel"
)

var ErrCompletedImportConflict = errors.New("completed-recording import identity conflicts with stored metadata")

type CompletedImportRepository struct {
	DB *Database
}

type completedImportSQLiteTx struct {
	ctx context.Context
}

type completedImportStored struct {
	ID                 string    `db:"id"`
	RootID             string    `db:"root_id"`
	PathHash           string    `db:"path_hash"`
	ParserVersion      string    `db:"parser_version"`
	CapturedTimezone   string    `db:"captured_timezone"`
	CapturedPrecision  string    `db:"captured_precision"`
	MatchState         string    `db:"match_state"`
	SceneID            int64     `db:"scene_id"`
	ShowID             int64     `db:"show_id"`
	SiteID             int64     `db:"site_id"`
	ModelID            int64     `db:"model_id"`
	FingerprintSize    int64     `db:"fingerprint_size"`
	FingerprintMTimeNS int64     `db:"fingerprint_mtime_ns"`
	FingerprintMode    int64     `db:"fingerprint_mode"`
	FingerprintDevice  int64     `db:"fingerprint_device"`
	FingerprintInode   int64     `db:"fingerprint_inode"`
	CapturedAt         time.Time `db:"captured_at"`
}

func NewCompletedImportRepository(db *Database) *CompletedImportRepository {
	return &CompletedImportRepository{DB: db}
}

func (r *CompletedImportRepository) WithCompletedImportTransaction(ctx context.Context, fn func(context.Context, cammodel.CompletedImportTx) error) error {
	if r == nil || r.DB == nil || fn == nil {
		return errors.New("completed-recording repository is not configured")
	}
	txCtx, err := r.DB.Begin(ctx, true)
	if err != nil {
		return err
	}
	tx := &completedImportSQLiteTx{ctx: txCtx}
	if err := fn(txCtx, tx); err != nil {
		if rollbackErr := r.DB.Rollback(txCtx); rollbackErr != nil {
			return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
		}
		return err
	}
	if err := r.DB.Commit(txCtx); err != nil {
		return err
	}
	return nil
}

func (t *completedImportSQLiteTx) LinkCamShowMetadata(_ context.Context, item cammodel.CompletedRecording) (bool, error) {
	if item.Outcome != cammodel.CompletedExactReady || item.MatchState != cammodel.CompletedAliasCurrent {
		return false, errors.New("only an exact active/current completed-recording match may be linked")
	}
	if item.SceneID <= 0 || item.SiteID <= 0 || item.ModelID <= 0 || item.ConfiguredRootID == "" ||
		item.RelativePath == "" || item.ParserVersion == "" || item.CompletedAt.IsZero() {
		return false, errors.New("completed-recording import metadata is incomplete")
	}
	if item.Fingerprint.Device > math.MaxInt64 || item.Fingerprint.Inode > math.MaxInt64 {
		return false, errors.New("completed-recording fingerprint exceeds SQLite range")
	}
	pathHash := completedImportHash(item.RelativePath)
	stableID := completedImportStableID(item, pathHash)
	var showID int64
	if err := dbWrapper.Get(t.ctx, &showID, "SELECT id FROM cam_shows WHERE scene_id=?", item.SceneID); err != nil {
		return false, err
	}
	stored, err := findCompletedImport(t.ctx, stableID)
	if err == nil {
		if !sameCompletedImport(stored, item, showID, pathHash) {
			return false, ErrCompletedImportConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	now := time.Now().UTC()
	_, err = dbWrapper.Exec(t.ctx, `INSERT INTO cam_completed_recording_imports(
		id,scene_id,show_id,site_id,model_id,configured_root_id,relative_path_hash,
		fingerprint_size,fingerprint_mtime_ns,fingerprint_mode,fingerprint_device,fingerprint_inode,
		parser_version,captured_at,captured_timezone,captured_precision,match_state,outcome,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'APPLIED',?)`,
		stableID, item.SceneID, showID, item.SiteID, item.ModelID, item.ConfiguredRootID, pathHash,
		item.Fingerprint.Size, item.Fingerprint.ModTimeNS, int64(item.Fingerprint.Mode),
		int64(item.Fingerprint.Device), int64(item.Fingerprint.Inode), item.ParserVersion,
		item.CompletedAt.UTC(), item.Timezone, string(item.TimePrecision), string(item.MatchState), now)
	if err != nil {
		return false, err
	}
	if _, err := dbWrapper.Exec(t.ctx, "INSERT OR IGNORE INTO cam_show_sites(show_id,site_id,created_at) VALUES(?,?,?)", showID, item.SiteID, now); err != nil {
		return false, err
	}
	if _, err := dbWrapper.Exec(t.ctx, "INSERT OR IGNORE INTO cam_show_models(show_id,model_id,billing_order,participation_role) VALUES(?,?,0,'PARTICIPANT')", showID, item.ModelID); err != nil {
		return false, err
	}
	return true, nil
}

func (t *completedImportSQLiteTx) WriteCompletedImportAudit(_ context.Context, audit cammodel.CompletedImportAudit) error {
	if strings.TrimSpace(audit.ActorID) == "" || len(audit.ActorID) > 64 ||
		!completedImportHexLength(audit.PreviewID, 32) || !completedImportHexLength(audit.CandidateID, 64) ||
		!completedImportHexLength(audit.RelativePathHash, 64) {
		return errors.New("completed-recording audit identity is invalid")
	}
	switch cammodel.CompletedImportOutcome(audit.Outcome) {
	case cammodel.CompletedApplied, cammodel.CompletedAlreadyApplied, cammodel.CompletedSkipped,
		cammodel.CompletedChanged, cammodel.CompletedReviewRequired:
	default:
		return errors.New("completed-recording audit outcome is invalid")
	}
	var reviewCode interface{}
	if audit.ReviewCode != cammodel.CompletedReviewNone {
		switch audit.ReviewCode {
		case cammodel.CompletedReviewMultipleScenes, cammodel.CompletedReviewMultipleSites,
			cammodel.CompletedReviewMultipleAliases, cammodel.CompletedReviewAliasReused:
			reviewCode = string(audit.ReviewCode)
		default:
			return errors.New("completed-recording audit review code is invalid")
		}
	}
	reason := redactCompletedImportAuditReason(audit.Reason)
	_, err := dbWrapper.Exec(t.ctx, `INSERT INTO cam_completed_recording_audits(
		actor_user_id,preview_id,candidate_id,relative_path_hash,outcome,review_reason_code,
		redacted_reason,scene_id,site_id,model_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		audit.ActorID, audit.PreviewID, audit.CandidateID, audit.RelativePathHash, audit.Outcome,
		reviewCode, reason, nullablePositiveID(audit.SceneID), nullablePositiveID(audit.SiteID),
		nullablePositiveID(audit.ModelID), audit.At.UTC())
	return err
}

func findCompletedImport(ctx context.Context, id string) (completedImportStored, error) {
	var value completedImportStored
	err := dbWrapper.Get(ctx, &value, `SELECT id,scene_id,show_id,site_id,model_id,
		configured_root_id AS root_id,relative_path_hash AS path_hash,
		fingerprint_size,fingerprint_mtime_ns,fingerprint_mode,fingerprint_device,fingerprint_inode,
		parser_version,captured_at,captured_timezone,captured_precision,match_state
		FROM cam_completed_recording_imports WHERE id=?`, id)
	return value, err
}

func sameCompletedImport(stored completedImportStored, item cammodel.CompletedRecording, showID int64, pathHash string) bool {
	return stored.SceneID == item.SceneID && stored.ShowID == showID && stored.SiteID == item.SiteID &&
		stored.ModelID == item.ModelID && stored.RootID == item.ConfiguredRootID && stored.PathHash == pathHash &&
		stored.FingerprintSize == item.Fingerprint.Size && stored.FingerprintMTimeNS == item.Fingerprint.ModTimeNS &&
		stored.FingerprintMode == int64(item.Fingerprint.Mode) && stored.FingerprintDevice == int64(item.Fingerprint.Device) &&
		stored.FingerprintInode == int64(item.Fingerprint.Inode) && stored.ParserVersion == item.ParserVersion &&
		stored.CapturedAt.Equal(item.CompletedAt.UTC()) && stored.CapturedTimezone == item.Timezone &&
		stored.CapturedPrecision == string(item.TimePrecision) && stored.MatchState == string(item.MatchState)
}

func completedImportStableID(item cammodel.CompletedRecording, pathHash string) string {
	return completedImportHash(fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%d\x00%d\x00%d\x00%s\x00%d",
		item.ConfiguredRootID, pathHash, item.Fingerprint.Size, item.Fingerprint.ModTimeNS,
		item.Fingerprint.Mode, item.Fingerprint.Device, item.Fingerprint.Inode, item.ParserVersion, item.SceneID))
}

func completedImportHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return encodinghex.EncodeToString(sum[:])
}

func completedImportHexLength(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := encodinghex.DecodeString(value)
	return err == nil
}

func redactCompletedImportAuditReason(value string) interface{} {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 512 || strings.ContainsAny(value, "/\\") {
		return "redacted:" + completedImportHash(value)[:16]
	}
	return value
}

func nullablePositiveID(value int64) interface{} {
	if value <= 0 {
		return nil
	}
	return value
}

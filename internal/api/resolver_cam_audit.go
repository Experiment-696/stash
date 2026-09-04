package api

import (
	"context"
	"strconv"

	"github.com/stashapp/stash/pkg/sqlite"
)

const (
	camAuditProfileCreated        = "cam_model_profile_created"
	camAuditProfileUpdated        = "cam_model_profile_updated"
	camAuditAccountAdded          = "cam_model_account_added"
	camAuditAccountRetired        = "cam_model_account_retired"
	camAuditEvidenceRecorded      = "cam_model_evidence_recorded"
	camAuditEvidenceReviewed      = "cam_model_evidence_reviewed"
	camAuditSocialProfileAdded    = "cam_model_social_profile_added"
	camAuditSocialProfileRetired  = "cam_model_social_profile_retired"
	camAuditDiscoveryIngested     = "cam_girl_finder_evidence_ingested"
	camAuditShowUpdated           = "cam_show_updated"
	camAuditRuleCreated           = "cam_classification_rule_created"
	camAuditRuleUpdated           = "cam_classification_rule_updated"
	camAuditRuleEnabledChanged    = "cam_classification_rule_enabled_changed"
	camAuditClassificationApplied = "cam_classification_applied"
)

func recordCamAudit(ctx context.Context, database *sqlite.Database, actorID int64, eventType, targetType string, targetID int64, result string) error {
	return recordCamAuditTarget(ctx, database, actorID, eventType, targetType, strconv.FormatInt(targetID, 10), result)
}

func recordCamAuditTarget(ctx context.Context, database *sqlite.Database, actorID int64, eventType, targetType, targetID, result string) error {
	return database.Audit.Record(ctx, actorID, eventType, targetType, targetID, result)
}

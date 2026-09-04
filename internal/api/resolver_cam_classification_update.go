package api

import (
	"context"
	"strconv"

	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func (r *mutationResolver) CamClassificationRuleUpdate(ctx context.Context, input CamClassificationRuleUpdateInput) (*CamClassificationRule, error) {
	ruleID, err := strconv.ParseInt(input.ID, 10, 64)
	if err != nil || ruleID <= 0 {
		return nil, ErrInput
	}
	tagIDs := make([]int, len(input.TagIDs))
	for i, value := range input.TagIDs {
		id, parseErr := strconv.Atoi(value)
		if parseErr != nil || id <= 0 {
			return nil, ErrInput
		}
		tagIDs[i] = id
	}
	database := r.tokenDatabase()
	var rule *sqlite.CamClassificationRule
	err = txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, database)
		if authErr != nil {
			return authErr
		}
		var updateErr error
		rule, updateErr = database.CamShow.UpdateClassificationRule(txCtx, ruleID, input.Name, input.Pattern, input.Target, input.Category, input.Enabled, tagIDs)
		if updateErr != nil {
			return updateErr
		}
		return recordCamAudit(txCtx, database, actorID, camAuditRuleUpdated, "cam_classification_rule", rule.ID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camClassificationRuleModel(*rule), nil
}

package api

import (
	"context"
	"errors"
	"strconv"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func camClassificationRuleModel(rule sqlite.CamClassificationRule) *CamClassificationRule {
	tagIDs := make([]string, len(rule.TagIDs))
	for i, id := range rule.TagIDs {
		tagIDs[i] = strconv.Itoa(id)
	}
	tags := make([]*CamShowLibraryTag, len(rule.Tags))
	for i, tag := range rule.Tags {
		tags[i] = &CamShowLibraryTag{ID: strconv.FormatInt(tag.ID, 10), Name: tag.Name}
	}
	return &CamClassificationRule{ID: strconv.FormatInt(rule.ID, 10), Name: rule.Name, Pattern: rule.Pattern, Target: rule.Target, Category: rule.Category, Enabled: rule.Enabled, TagIDs: tagIDs, Tags: tags, CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt}
}

func camShowLibraryModel(show sqlite.CamShowDomainItem) *CamShowLibraryItem {
	tags := make([]*CamShowLibraryTag, len(show.Tags))
	for i, v := range show.Tags {
		tags[i] = &CamShowLibraryTag{ID: strconv.FormatInt(v.ID, 10), Name: v.Name}
	}
	sites := make([]*CamShowDomainSite, len(show.Sites))
	for i, v := range show.Sites {
		sites[i] = &CamShowDomainSite{ID: strconv.FormatInt(v.ID, 10), Name: v.Name, Icon: v.Icon}
	}
	links := make([]*CamShowDomainLink, len(show.Links))
	for i, v := range show.Links {
		links[i] = &CamShowDomainLink{ID: strconv.FormatInt(v.ID, 10), LinkType: v.LinkType, URL: v.URL, Label: v.Label}
	}
	models := make([]*CamShowDomainModel, len(show.Models))
	for i, v := range show.Models {
		models[i] = &CamShowDomainModel{ModelID: strconv.FormatInt(v.ModelID, 10), DisplayName: v.DisplayName, Role: v.Role}
	}
	return &CamShowLibraryItem{ID: strconv.FormatInt(show.ID, 10), SceneID: strconv.FormatInt(show.SceneID, 10), Title: show.Title, ShowType: show.ShowType, ShowDate: show.ShowDate, CapturedAt: show.CapturedAt, CapturedTimezone: show.CapturedTimezone, CapturedPrecision: show.CapturedPrecision, DurationSeconds: show.DurationSeconds, DurationOverridden: show.DurationOverridden, DurationOverrideReason: show.DurationOverrideReason, Rate: show.Rate, Extras: show.Extras, Request: show.Request, Rating100: show.Rating100, Rating100Average: show.Rating100Average, Rating100Count: show.Rating100Count, Tags: tags, Sites: sites, Links: links, Models: models}
}

func camClassificationResultModel(result *sqlite.CamClassificationResult) *CamClassificationResult {
	ret := &CamClassificationResult{Matched: result.Matched, Applied: result.Applied, Skipped: result.Skipped, Conflicted: result.Conflicted, Items: make([]*CamClassificationItem, len(result.Items))}
	for i, item := range result.Items {
		tagIDs := make([]string, len(item.TagIDs))
		for j, id := range item.TagIDs {
			tagIDs[j] = strconv.Itoa(id)
		}
		ret.Items[i] = &CamClassificationItem{SceneID: strconv.FormatInt(item.SceneID, 10), Matched: item.Matched, Applied: item.Applied, Skipped: item.Skipped, Conflict: item.Conflict, Category: item.Category, TagIDs: tagIDs}
	}
	return ret
}

func requirePersistedCamClassificationAdmin(ctx, txCtx context.Context, database *sqlite.Database) error {
	principal, err := authz.RequireContext(ctx, authz.DataAdmin)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || id <= 0 {
		return authz.UnauthenticatedError{}
	}
	user, err := database.User.Find(txCtx, id)
	if err != nil || user == nil {
		return authz.UnauthenticatedError{}
	}
	persisted := authz.Principal{UserID: principal.UserID, Role: user.Role, Status: user.Status, TokenScopes: principal.TokenScopes}
	if !persisted.Allows(authz.DataAdmin) {
		return authz.DeniedError{Capability: authz.DataAdmin}
	}
	return nil
}

func classificationRoots() []string { return manager.GetInstance().Config.GetStashPaths().Paths() }

func (r *queryResolver) CamShows(ctx context.Context, sort CamShowSortMode) ([]*CamShowLibraryItem, error) {
	database := r.tokenDatabase()
	var shows []sqlite.CamShowDomainItem
	err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		userID, _, err := requirePersistedCamModelReadUser(ctx, txCtx, database)
		if err != nil {
			return err
		}
		favoriteModelsFirst := sort == CamShowSortModeFavoriteModelsFirst
		shows, err = database.CamShow.ListShowDomainForUser(txCtx, userID, favoriteModelsFirst)
		return err
	})
	if err != nil {
		return nil, err
	}
	ret := make([]*CamShowLibraryItem, len(shows))
	for i, show := range shows {
		tags := make([]*CamShowLibraryTag, len(show.Tags))
		for j, tag := range show.Tags {
			tags[j] = &CamShowLibraryTag{ID: strconv.FormatInt(tag.ID, 10), Name: tag.Name}
		}
		sites := make([]*CamShowDomainSite, len(show.Sites))
		for j, site := range show.Sites {
			sites[j] = &CamShowDomainSite{ID: strconv.FormatInt(site.ID, 10), Name: site.Name, Icon: site.Icon}
		}
		links := make([]*CamShowDomainLink, len(show.Links))
		for j, link := range show.Links {
			links[j] = &CamShowDomainLink{ID: strconv.FormatInt(link.ID, 10), LinkType: link.LinkType, URL: link.URL, Label: link.Label}
		}
		models := make([]*CamShowDomainModel, len(show.Models))
		for j, model := range show.Models {
			models[j] = &CamShowDomainModel{ModelID: strconv.FormatInt(model.ModelID, 10), DisplayName: model.DisplayName, Role: model.Role}
		}
		ret[i] = &CamShowLibraryItem{ID: strconv.FormatInt(show.ID, 10), SceneID: strconv.FormatInt(show.SceneID, 10), Title: show.Title, ShowType: show.ShowType, ShowDate: show.ShowDate, CapturedAt: show.CapturedAt, CapturedTimezone: show.CapturedTimezone, CapturedPrecision: show.CapturedPrecision, DurationSeconds: show.DurationSeconds, DurationOverridden: show.DurationOverridden, DurationOverrideReason: show.DurationOverrideReason, Rate: show.Rate, Extras: show.Extras, Request: show.Request, Rating100: show.Rating100, Rating100Average: show.Rating100Average, Rating100Count: show.Rating100Count, Tags: tags, Sites: sites, Links: links, Models: models}
	}
	return ret, nil
}

func (r *queryResolver) CamClassificationRules(ctx context.Context) ([]*CamClassificationRule, error) {
	database := r.tokenDatabase()
	var rules []sqlite.CamClassificationRule
	err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		if err := requirePersistedCamClassificationAdmin(ctx, txCtx, database); err != nil {
			return err
		}
		var err error
		rules, err = database.CamShow.ListClassificationRules(txCtx, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	ret := make([]*CamClassificationRule, len(rules))
	for i, rule := range rules {
		ret[i] = camClassificationRuleModel(rule)
	}
	return ret, nil
}

func (r *queryResolver) CamClassificationPreview(ctx context.Context) (*CamClassificationResult, error) {
	database := r.tokenDatabase()
	var result *sqlite.CamClassificationResult
	err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		if err := requirePersistedCamClassificationAdmin(ctx, txCtx, database); err != nil {
			return err
		}
		candidates, err := database.CamShow.EnumerateClassificationCandidates(txCtx, classificationRoots())
		if err != nil {
			return err
		}
		result, err = database.CamShow.PreviewClassification(txCtx, candidates)
		return err
	})
	if err != nil {
		return nil, err
	}
	return camClassificationResultModel(result), nil
}

func (r *mutationResolver) CamClassificationRuleCreate(ctx context.Context, input CamClassificationRuleCreateInput) (*CamClassificationRule, error) {
	tagIDs := make([]int, len(input.TagIDs))
	for i, value := range input.TagIDs {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			return nil, ErrInput
		}
		tagIDs[i] = id
	}
	database := r.tokenDatabase()
	var rule *sqlite.CamClassificationRule
	err := txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		actorID, err := requirePersistedCamModelAdmin(ctx, txCtx, database)
		if err != nil {
			return err
		}
		rule, err = database.CamShow.CreateClassificationRule(txCtx, input.Name, input.Pattern, input.Target, input.Category, input.Enabled, tagIDs)
		if err != nil {
			return err
		}
		return recordCamAudit(txCtx, database, actorID, camAuditRuleCreated, "cam_classification_rule", rule.ID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camClassificationRuleModel(*rule), nil
}

func (r *mutationResolver) CamClassificationRuleSetEnabled(ctx context.Context, id string, enabled bool) (bool, error) {
	ruleID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || ruleID <= 0 {
		return false, ErrInput
	}
	database := r.tokenDatabase()
	err = txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		actorID, err := requirePersistedCamModelAdmin(ctx, txCtx, database)
		if err != nil {
			return err
		}
		if err := database.CamShow.SetClassificationRuleEnabled(txCtx, ruleID, enabled); err != nil {
			return err
		}
		result := "disabled"
		if enabled {
			result = "enabled"
		}
		return recordCamAudit(txCtx, database, actorID, camAuditRuleEnabledChanged, "cam_classification_rule", ruleID, result)
	})
	return err == nil, err
}

func (r *mutationResolver) CamClassificationApply(ctx context.Context) (*CamClassificationResult, error) {
	database := r.tokenDatabase()
	var result *sqlite.CamClassificationResult
	err := txn.WithTxn(ctx, database, func(txCtx context.Context) error {
		actorID, err := requirePersistedCamModelAdmin(ctx, txCtx, database)
		if err != nil {
			return err
		}
		candidates, err := database.CamShow.EnumerateClassificationCandidates(txCtx, classificationRoots())
		if err != nil {
			return err
		}
		result, err = applyCamClassificationWithAudit(txCtx, database, actorID, candidates)
		return err
	})
	if err != nil {
		return nil, err
	}
	return camClassificationResultModel(result), nil
}

func applyCamClassificationWithAudit(ctx context.Context, database *sqlite.Database, actorID int64, candidates []sqlite.CamClassificationCandidate) (*sqlite.CamClassificationResult, error) {
	result, err := database.CamShow.ApplyClassification(ctx, candidates)
	if err != nil {
		return nil, err
	}
	if err := recordCamAuditTarget(ctx, database, actorID, camAuditClassificationApplied, "cam_classification", "enabled-rules", "success"); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *mutationResolver) CamShowUpdate(ctx context.Context, input CamShowCoreUpdateInput) (*CamShowLibraryItem, error) {
	id, e := parseCamID(input.ID)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	var show *sqlite.CamShowDomainItem
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, e := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if e != nil {
			return e
		}
		show, e = db.CamShow.UpdateShowCore(txCtx, sqlite.CamShowCoreUpdateInput{ID: id, Title: input.Title, ShowType: input.ShowType, ShowDate: input.ShowDate, CapturedAt: input.CapturedAt, CapturedTimezone: input.CapturedTimezone, CapturedPrecision: input.CapturedPrecision, DurationOverrideSeconds: input.DurationOverrideSeconds, DurationOverrideReason: input.DurationOverrideReason, Rate: input.Rate, Extras: input.Extras, Request: input.Request})
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditShowUpdated, "cam_show", show.ID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camShowLibraryModel(*show), nil
}

func (r *mutationResolver) CamShowSetRating(ctx context.Context, id string, rating100 *int) (*CamShowLibraryItem, error) {
	showID, e := parseCamID(id)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	var show *sqlite.CamShowDomainItem
	e = txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		userID, err := requirePersistedCamModelPreference(ctx, txCtx, db)
		if err != nil {
			return err
		}
		if err := db.CamShow.SetShowRating(txCtx, userID, showID, rating100); err != nil {
			return err
		}
		shows, err := db.CamShow.ListShowDomainForUser(txCtx, userID, false)
		if err != nil {
			return err
		}
		for i := range shows {
			if shows[i].ID == showID {
				show = &shows[i]
				return nil
			}
		}
		return errors.New("Cam Show not found")
	})
	if e != nil {
		return nil, e
	}
	return camShowLibraryModel(*show), nil
}

func (r *mutationResolver) CamShowSetAssociations(ctx context.Context, input CamShowAssociationsInput) (*CamShowLibraryItem, error) {
	showID, err := parseCamID(input.ID)
	if err != nil {
		return nil, err
	}
	siteIDs := make([]int64, len(input.SiteIDs))
	for i, value := range input.SiteIDs {
		siteIDs[i], err = parseCamID(value)
		if err != nil {
			return nil, err
		}
	}
	models := make([]sqlite.CamShowModelAssignment, len(input.Models))
	for i, value := range input.Models {
		models[i].ModelID, err = parseCamID(value.ModelID)
		if err != nil {
			return nil, err
		}
		models[i].Role = value.Role
	}

	db := r.tokenDatabase()
	var show *sqlite.CamShowDomainItem
	err = txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		if setErr := db.CamShow.SetShowAssociations(txCtx, showID, siteIDs, models); setErr != nil {
			return setErr
		}
		shows, listErr := db.CamShow.ListShowDomainForUser(txCtx, actorID, false)
		if listErr != nil {
			return listErr
		}
		for i := range shows {
			if shows[i].ID == showID {
				show = &shows[i]
				break
			}
		}
		if show == nil {
			return errors.New("Cam Show not found")
		}
		return recordCamAudit(txCtx, db, actorID, camAuditShowUpdated, "cam_show", showID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camShowLibraryModel(*show), nil
}

package api

import (
	"context"
	"strconv"
	"time"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func requirePersistedCamModelAdmin(ctx, txCtx context.Context, database *sqlite.Database) (int64, error) {
	principal, err := authz.RequireContext(ctx, authz.DataAdmin)
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || id <= 0 {
		return 0, authz.UnauthenticatedError{}
	}
	user, err := database.User.Find(txCtx, id)
	if err != nil || user == nil {
		return 0, authz.UnauthenticatedError{}
	}
	persisted := authz.Principal{UserID: principal.UserID, Role: user.Role, Status: user.Status, TokenScopes: principal.TokenScopes}
	if !persisted.Allows(authz.DataAdmin) {
		return 0, authz.DeniedError{Capability: authz.DataAdmin}
	}
	return id, nil
}

func requirePersistedCamModelReadUser(ctx, txCtx context.Context, database *sqlite.Database) (int64, bool, error) {
	principal, err := authz.RequireContext(ctx, authz.LibraryRead)
	if err != nil {
		return 0, false, err
	}
	id, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || id <= 0 {
		return 0, false, authz.UnauthenticatedError{}
	}
	user, err := database.User.Find(txCtx, id)
	if err != nil || user == nil {
		return 0, false, authz.UnauthenticatedError{}
	}
	persisted := authz.Principal{UserID: principal.UserID, Role: user.Role, Status: user.Status, TokenScopes: principal.TokenScopes}
	if !persisted.Allows(authz.LibraryRead) {
		return 0, false, authz.DeniedError{Capability: authz.LibraryRead}
	}
	return id, persisted.Allows(authz.DataAdmin), nil
}

func requirePersistedCamModelRead(ctx, txCtx context.Context, database *sqlite.Database) (bool, error) {
	_, admin, err := requirePersistedCamModelReadUser(ctx, txCtx, database)
	return admin, err
}

func parseCamID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInput
	}
	return id, nil
}
func parseOptionalCamID(value *string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	id, err := parseCamID(*value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
func camAccountInput(modelID int64, input *CamModelAccountCreateInput) (sqlite.CamModelAccountInput, error) {
	if input == nil {
		return sqlite.CamModelAccountInput{}, ErrInput
	}
	siteID, err := parseCamID(input.SiteID)
	if err != nil {
		return sqlite.CamModelAccountInput{}, err
	}
	return sqlite.CamModelAccountInput{ModelID: modelID, SiteID: siteID, Handle: input.Handle, ProfileURL: input.ProfileURL, ExternalAccountID: input.ExternalAccountID, FirstSeenAt: input.FirstSeenAt, LastSeenAt: input.LastSeenAt, ValidFrom: input.ValidFrom, Confidence: input.Confidence}, nil
}
func camSiteModel(v sqlite.CamSite) *CamModelSite {
	return &CamModelSite{ID: strconv.FormatInt(v.ID, 10), Name: v.Name, BaseURL: v.BaseURL, ExternalKey: v.ExternalKey, Enabled: v.Enabled}
}
func camEvidenceModel(v sqlite.CamModelProfileProvenance, includeSensitive bool) *CamModelEvidence {
	var accountID, reviewedBy *string
	if v.AccountID != nil {
		x := strconv.FormatInt(*v.AccountID, 10)
		accountID = &x
	}
	if v.ReviewedBy != nil {
		x := strconv.FormatInt(*v.ReviewedBy, 10)
		reviewedBy = &x
	}
	ret := &CamModelEvidence{ID: strconv.FormatInt(v.ID, 10), ModelID: strconv.FormatInt(v.ModelID, 10), AccountID: accountID, Provider: v.Provider, EvidenceKey: v.EvidenceKey, SourceURL: v.SourceURL, ObservedAt: v.ObservedAt, PayloadJSON: "", Confidence: v.Confidence, ReviewState: v.ReviewState, ReviewedBy: reviewedBy, ReviewedAt: v.ReviewedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
	if includeSensitive {
		ret.ProviderRecordID = v.ProviderRecordID
		ret.PayloadJSON = v.PayloadJSON
	}
	return ret
}
func camProfileModel(v *sqlite.CamModelProfile, includeSensitive bool) *CamModelProfile {
	var performerID *string
	if v.Model.PerformerID != nil {
		x := strconv.FormatInt(*v.Model.PerformerID, 10)
		performerID = &x
	}
	favorite := false
	var rating *int
	if v.UserState != nil {
		favorite, rating = v.UserState.Favorite, v.UserState.Rating
	}
	ret := &CamModelProfile{Favorite: favorite, Rating100: rating, ID: strconv.FormatInt(v.Model.ID, 10), DisplayName: v.Model.DisplayName, Image: v.Model.Image, Notes: v.Model.Notes, PerformerID: performerID, Status: v.Model.Status, CreatedAt: v.Model.CreatedAt, UpdatedAt: v.Model.UpdatedAt, Accounts: make([]*CamModelAccountProfile, len(v.Accounts)), Evidence: make([]*CamModelEvidence, len(v.Provenance)), SocialProfiles: make([]*CamModelSocialProfile, len(v.SocialProfiles))}
	for i, a := range v.Accounts {
		ret.Accounts[i] = &CamModelAccountProfile{ID: strconv.FormatInt(a.ID, 10), Site: &CamModelSite{ID: strconv.FormatInt(a.SiteID, 10), Name: a.SiteName, BaseURL: a.SiteBaseURL, ExternalKey: a.SiteExternalKey, Enabled: a.SiteEnabled}, Handle: a.Handle, Status: a.Status, ValidFrom: a.ValidFrom, ValidTo: a.ValidTo, ProfileURL: a.ProfileURL, FirstSeenAt: a.FirstSeenAt, LastSeenAt: a.LastSeenAt, Source: a.Source, Confidence: a.Confidence}
		if includeSensitive {
			ret.Accounts[i].ExternalAccountID = a.ExternalAccountID
		}
	}
	for i, e := range v.Provenance {
		ret.Evidence[i] = camEvidenceModel(e, includeSensitive)
	}
	for i, social := range v.SocialProfiles {
		ret.SocialProfiles[i] = &CamModelSocialProfile{ID: strconv.FormatInt(social.ID, 10), ModelID: strconv.FormatInt(social.ModelID, 10), Platform: social.Platform, Icon: social.Icon, Handle: social.Handle, ProfileURL: social.URL, Status: social.Status, ValidFrom: social.ValidFrom, ValidTo: social.ValidTo, Source: social.Source, Confidence: social.Confidence, Provenance: social.Provenance}
	}
	return ret
}

func (r *queryResolver) CamModelProfiles(ctx context.Context, favoritesOnly bool) ([]*CamModelProfile, error) {
	db := r.tokenDatabase()
	var profiles []sqlite.CamModelProfile
	var includeSensitive bool
	err := txn.WithReadTxn(ctx, db, func(txCtx context.Context) error {
		userID, admin, e := requirePersistedCamModelReadUser(ctx, txCtx, db)
		if e != nil {
			return e
		}
		includeSensitive = admin
		profiles, e = db.CamShow.ListModelProfilesForUser(txCtx, userID, favoritesOnly)
		return e
	})
	if err != nil {
		return nil, err
	}
	ret := make([]*CamModelProfile, len(profiles))
	for i := range profiles {
		ret[i] = camProfileModel(&profiles[i], includeSensitive)
	}
	return ret, nil
}
func (r *queryResolver) CamModelProfile(ctx context.Context, id string) (*CamModelProfile, error) {
	modelID, e := parseCamID(id)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	var p *sqlite.CamModelProfile
	var includeSensitive bool
	err := txn.WithReadTxn(ctx, db, func(txCtx context.Context) error {
		userID, admin, readErr := requirePersistedCamModelReadUser(ctx, txCtx, db)
		if readErr != nil {
			return readErr
		}
		includeSensitive = admin
		p, e = db.CamShow.FindModelProfileForUser(txCtx, modelID, userID)
		return e
	})
	if err != nil || p == nil {
		return nil, err
	}
	return camProfileModel(p, includeSensitive), nil
}
func (r *queryResolver) CamModelSites(ctx context.Context) ([]*CamModelSite, error) {
	db := r.tokenDatabase()
	var sites []sqlite.CamSite
	err := txn.WithReadTxn(ctx, db, func(txCtx context.Context) error {
		if _, e := requirePersistedCamModelRead(ctx, txCtx, db); e != nil {
			return e
		}
		var e error
		sites, e = db.CamShow.ListSites(txCtx)
		return e
	})
	if err != nil {
		return nil, err
	}
	ret := make([]*CamModelSite, len(sites))
	for i := range sites {
		ret[i] = camSiteModel(sites[i])
	}
	return ret, nil
}

func (r *mutationResolver) CamModelProfileCreate(ctx context.Context, input CamModelProfileCreateInput) (*CamModelProfile, error) {
	performerID, e := parseOptionalCamID(input.PerformerID)
	if e != nil {
		return nil, e
	}
	accounts := make([]sqlite.CamModelAccountInput, len(input.Accounts))
	for i, a := range input.Accounts {
		x, e := camAccountInput(0, a)
		if e != nil {
			return nil, e
		}
		accounts[i] = x
	}
	db := r.tokenDatabase()
	svc := manager.CamModelProfileService{Database: db}
	var p *sqlite.CamModelProfile
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		p, e = svc.Create(txCtx, manager.CamModelProfileCreateServiceInput{DisplayName: input.DisplayName, Image: input.Image, Notes: input.Notes, PerformerID: performerID, Status: input.Status, Accounts: accounts})
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditProfileCreated, "cam_model", p.Model.ID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camProfileModel(p, true), nil
}
func (r *mutationResolver) CamModelProfileUpdate(ctx context.Context, input CamModelProfileUpdateInput) (*CamModelProfile, error) {
	id, e := parseCamID(input.ID)
	if e != nil {
		return nil, e
	}
	performerID, e := parseOptionalCamID(input.PerformerID)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	svc := manager.CamModelProfileService{Database: db}
	var p *sqlite.CamModelProfile
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		p, e = svc.Update(txCtx, sqlite.CamModelProfileUpdateInput{ID: id, DisplayName: input.DisplayName, Image: input.Image, Notes: input.Notes, PerformerID: performerID, Status: input.Status})
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditProfileUpdated, "cam_model", id, "success")
	})
	if err != nil {
		return nil, err
	}
	return camProfileModel(p, true), nil
}
func (r *mutationResolver) CamModelAccountAdd(ctx context.Context, input CamModelAccountAddInput) (*CamModelProfile, error) {
	modelID, e := parseCamID(input.ModelID)
	if e != nil {
		return nil, e
	}
	account, e := camAccountInput(modelID, input.Account)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	svc := manager.CamModelProfileService{Database: db}
	var p *sqlite.CamModelProfile
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		p, e = svc.AddAccount(txCtx, account)
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditAccountAdded, "cam_model", modelID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camProfileModel(p, true), nil
}
func (r *mutationResolver) CamModelAccountRetire(ctx context.Context, id string, validTo time.Time) (*CamModelProfile, error) {
	accountID, e := parseCamID(id)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	svc := manager.CamModelProfileService{Database: db}
	var p *sqlite.CamModelProfile
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		p, e = svc.RetireAccount(txCtx, accountID, validTo)
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditAccountRetired, "cam_model_account", accountID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camProfileModel(p, true), nil
}
func (r *mutationResolver) CamModelEvidenceCreate(ctx context.Context, input CamModelEvidenceCreateInput) (*CamModelEvidenceIngestResult, error) {
	modelID, e := parseCamID(input.ModelID)
	if e != nil {
		return nil, e
	}
	accountID, e := parseOptionalCamID(input.AccountID)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	svc := manager.CamModelProfileService{Database: db}
	var result *sqlite.CamModelProvenanceIngestResult
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		result, e = svc.IngestEvidence(txCtx, sqlite.CamModelProvenanceInput{ModelID: modelID, AccountID: accountID, Provider: input.Provider, EvidenceKey: input.EvidenceKey, ProviderRecordID: input.ProviderRecordID, SourceURL: input.SourceURL, ObservedAt: input.ObservedAt, PayloadJSON: input.PayloadJSON, Confidence: input.Confidence})
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditEvidenceRecorded, "cam_model_evidence", result.Provenance.ID, string(result.Status))
	})
	if err != nil {
		return nil, err
	}
	return &CamModelEvidenceIngestResult{Status: string(result.Status), Evidence: camEvidenceModel(result.Provenance, true)}, nil
}
func (r *mutationResolver) CamModelEvidenceReview(ctx context.Context, id string, state string) (*CamModelEvidence, error) {
	evidenceID, e := parseCamID(id)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	svc := manager.CamModelProfileService{Database: db}
	var result *sqlite.CamModelProfileProvenance
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		reviewerID, e := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if e != nil {
			return e
		}
		result, e = svc.ReviewEvidence(txCtx, evidenceID, reviewerID, state)
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, reviewerID, camAuditEvidenceReviewed, "cam_model_evidence", evidenceID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camEvidenceModel(*result, true), nil
}

func (r *mutationResolver) CamModelSocialProfileCreate(ctx context.Context, input CamModelSocialProfileCreateInput) (*CamModelProfile, error) {
	modelID, e := parseCamID(input.ModelID)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	var p *sqlite.CamModelProfile
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		social, e := db.CamShow.AddModelSocialProfile(txCtx, sqlite.CamModelSocialProfileInput{ModelID: modelID, Platform: input.Platform, Icon: input.Icon, Handle: input.Handle, URL: input.ProfileURL, ValidFrom: input.ValidFrom, Source: input.Source, Confidence: input.Confidence, Provenance: input.Provenance})
		if e != nil {
			return e
		}
		p, e = db.CamShow.FindModelProfile(txCtx, modelID)
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditSocialProfileAdded, "cam_model_social_profile", social.ID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camProfileModel(p, true), nil
}
func (r *mutationResolver) CamModelSocialProfileRetire(ctx context.Context, id string, validTo time.Time) (*CamModelProfile, error) {
	profileID, e := parseCamID(id)
	if e != nil {
		return nil, e
	}
	db := r.tokenDatabase()
	var p *sqlite.CamModelProfile
	err := txn.WithTxn(ctx, db, func(txCtx context.Context) error {
		actorID, authErr := requirePersistedCamModelAdmin(ctx, txCtx, db)
		if authErr != nil {
			return authErr
		}
		social, e := db.CamShow.RetireModelSocialProfile(txCtx, profileID, validTo)
		if e != nil {
			return e
		}
		p, e = db.CamShow.FindModelProfile(txCtx, social.ModelID)
		if e != nil {
			return e
		}
		return recordCamAudit(txCtx, db, actorID, camAuditSocialProfileRetired, "cam_model_social_profile", profileID, "success")
	})
	if err != nil {
		return nil, err
	}
	return camProfileModel(p, true), nil
}

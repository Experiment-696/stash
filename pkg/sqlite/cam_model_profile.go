package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/cammodel"
)

const (
	CamModelReviewPending  = "PENDING"
	CamModelReviewApproved = "APPROVED"
	CamModelReviewRejected = "REJECTED"

	CamModelProvenanceInserted  CamModelProvenanceIngestStatus = "INSERTED"
	CamModelProvenanceUnchanged CamModelProvenanceIngestStatus = "UNCHANGED"
)

var ErrCamModelProvenanceConflict = errors.New("cam model provenance evidence key conflicts with existing observation")

type CamModelProvenanceIngestStatus string

type CamModelProvenanceIngestResult struct {
	Provenance CamModelProfileProvenance
	Status     CamModelProvenanceIngestStatus
}

type CamModelProfileAccount struct {
	CamModelAccount
	SiteName          string     `db:"site_name"`
	SiteExternalKey   *string    `db:"site_external_key"`
	SiteBaseURL       *string    `db:"site_base_url"`
	SiteEnabled       bool       `db:"site_enabled"`
	ProfileURL        *string    `db:"profile_url"`
	ExternalAccountID *string    `db:"external_account_id"`
	FirstSeenAt       *time.Time `db:"first_seen_at"`
	LastSeenAt        *time.Time `db:"last_seen_at"`
	LastSyncedAt      *time.Time `db:"last_synced_at"`
	Source            string     `db:"source"`
	Confidence        *float64   `db:"confidence"`
}

type CamModelProfileProvenance struct {
	ID               int64      `db:"id"`
	ModelID          int64      `db:"model_id"`
	AccountID        *int64     `db:"account_id"`
	Provider         string     `db:"provider"`
	EvidenceKey      string     `db:"evidence_key"`
	ProviderRecordID *string    `db:"provider_record_id"`
	SourceURL        *string    `db:"source_url"`
	ObservedAt       time.Time  `db:"observed_at"`
	PayloadJSON      string     `db:"payload_json"`
	Confidence       *float64   `db:"confidence"`
	ReviewState      string     `db:"review_state"`
	ReviewedBy       *int64     `db:"reviewed_by"`
	ReviewedAt       *time.Time `db:"reviewed_at"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at"`
}

type CamModelProfile struct {
	Model      CamModel
	Accounts   []CamModelProfileAccount
	Aliases    []CamModelAlias
	Provenance []CamModelProfileProvenance
	SocialProfiles []CamModelSocialProfile
	UserState  *CamModelUserState
}

type CamModelAccountInput struct {
	ModelID, SiteID               int64
	Handle                        string
	ProfileURL, ExternalAccountID *string
	FirstSeenAt, LastSeenAt       *time.Time
	ValidFrom                     *time.Time
	Confidence                    *float64
}

type CamModelProvenanceInput struct {
	ModelID          int64
	AccountID        *int64
	Provider         string
	EvidenceKey      string
	ProviderRecordID *string
	SourceURL        *string
	ObservedAt       time.Time
	PayloadJSON      string
	Confidence       *float64
}

// AddManualModelAccount is the only public account-creation seam. Provider
// observations must first be stored as pending provenance and cannot select an
// account source or bypass review.
func (s *CamShowStore) AddManualModelAccount(ctx context.Context, input CamModelAccountInput) (*CamModelAccount, error) {
	handle := strings.TrimSpace(input.Handle)
	normalized := normalizeCamIdentity(handle)
	if input.ModelID <= 0 || input.SiteID <= 0 || normalized == "" {
		return nil, errors.New("model, site, and handle are required")
	}
	if input.Confidence != nil && (*input.Confidence < 0 || *input.Confidence > 1) {
		return nil, errors.New("confidence must be between zero and one")
	}
	now := time.Now().UTC()
	result, err := dbWrapper.Exec(ctx, `INSERT INTO cam_model_accounts
		(model_id,site_id,handle,normalized_handle,profile_url,external_account_id,status,first_seen_at,last_seen_at,valid_from,source,confidence,created_at,updated_at)
		VALUES(?,?,?,?,?,?,'ACTIVE',?,?,?,'MANUAL',?,?,?)`, input.ModelID, input.SiteID, handle, normalized, input.ProfileURL, input.ExternalAccountID, input.FirstSeenAt, input.LastSeenAt, input.ValidFrom, input.Confidence, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.FindAccount(ctx, id)
}

// IngestPendingProfileObservation implements the provider-facing service seam
// without exposing account creation or merge operations.
func (s *CamShowStore) IngestPendingProfileObservation(ctx context.Context, modelID int64, accountID *int64, observation cammodel.ProfileObservation) (cammodel.ObservationIngestResult, error) {
	result, err := s.IngestModelProvenance(ctx, CamModelProvenanceInput{
		ModelID: modelID, AccountID: accountID, Provider: observation.Provider,
		EvidenceKey: observation.EvidenceKey, ProviderRecordID: observation.ProviderRecordID,
		SourceURL: observation.SourceURL, ObservedAt: observation.ObservedAt,
		PayloadJSON: observation.PayloadJSON, Confidence: observation.Confidence,
	})
	if err != nil {
		return cammodel.ObservationIngestResult{}, err
	}
	status := cammodel.ObservationInserted
	if result.Status == CamModelProvenanceUnchanged {
		status = cammodel.ObservationUnchanged
	}
	return cammodel.ObservationIngestResult{EvidenceID: result.Provenance.ID, Status: status}, nil
}

func (s *CamShowStore) IngestModelProvenance(ctx context.Context, input CamModelProvenanceInput) (*CamModelProvenanceIngestResult, error) {
	input.Provider = strings.TrimSpace(input.Provider)
	input.EvidenceKey = strings.TrimSpace(input.EvidenceKey)
	if input.ModelID <= 0 || input.Provider == "" || input.EvidenceKey == "" || input.ObservedAt.IsZero() || !json.Valid([]byte(input.PayloadJSON)) {
		return nil, errors.New("valid model, provider, evidence key, observation time, and JSON payload are required")
	}
	if input.Confidence != nil && (*input.Confidence < 0 || *input.Confidence > 1) {
		return nil, errors.New("confidence must be between zero and one")
	}
	if input.AccountID != nil {
		account, err := s.FindAccount(ctx, *input.AccountID)
		if err != nil {
			return nil, err
		}
		if account == nil || account.ModelID != input.ModelID {
			return nil, errors.New("provenance account does not belong to model")
		}
	}
	now := time.Now().UTC()
	result, err := dbWrapper.Exec(ctx, `INSERT INTO cam_model_profile_provenance
		(model_id,account_id,provider,evidence_key,provider_record_id,source_url,observed_at,payload_json,confidence,review_state,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,'PENDING',?,?)
		ON CONFLICT(provider,evidence_key) DO NOTHING`, input.ModelID, input.AccountID, input.Provider, input.EvidenceKey, input.ProviderRecordID, input.SourceURL, input.ObservedAt.UTC(), input.PayloadJSON, input.Confidence, now, now)
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed == 1 {
		id, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		value, err := s.FindModelProvenance(ctx, id)
		if err != nil {
			return nil, err
		}
		return &CamModelProvenanceIngestResult{Provenance: *value, Status: CamModelProvenanceInserted}, nil
	}
	existing, err := s.findModelProvenanceByEvidenceKey(ctx, input.Provider, input.EvidenceKey)
	if err != nil {
		return nil, err
	}
	if existing == nil || !sameModelProvenanceObservation(*existing, input) {
		return nil, ErrCamModelProvenanceConflict
	}
	return &CamModelProvenanceIngestResult{Provenance: *existing, Status: CamModelProvenanceUnchanged}, nil
}

func (s *CamShowStore) findModelProvenanceByEvidenceKey(ctx context.Context, provider, evidenceKey string) (*CamModelProfileProvenance, error) {
	var value CamModelProfileProvenance
	err := dbWrapper.Get(ctx, &value, `SELECT id,model_id,account_id,provider,evidence_key,provider_record_id,source_url,observed_at,payload_json,confidence,review_state,reviewed_by,reviewed_at,created_at,updated_at FROM cam_model_profile_provenance WHERE provider=? AND evidence_key=?`, provider, evidenceKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func sameModelProvenanceObservation(existing CamModelProfileProvenance, input CamModelProvenanceInput) bool {
	return existing.ModelID == input.ModelID && equalInt64Pointers(existing.AccountID, input.AccountID) &&
		equalStringPointers(existing.ProviderRecordID, input.ProviderRecordID) && equalStringPointers(existing.SourceURL, input.SourceURL) &&
		existing.ObservedAt.Equal(input.ObservedAt.UTC()) && existing.PayloadJSON == input.PayloadJSON &&
		equalFloat64Pointers(existing.Confidence, input.Confidence)
}
func equalInt64Pointers(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func equalStringPointers(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func equalFloat64Pointers(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func (s *CamShowStore) FindModelProvenance(ctx context.Context, id int64) (*CamModelProfileProvenance, error) {
	var value CamModelProfileProvenance
	err := dbWrapper.Get(ctx, &value, `SELECT id,model_id,account_id,provider,evidence_key,provider_record_id,source_url,observed_at,payload_json,confidence,review_state,reviewed_by,reviewed_at,created_at,updated_at FROM cam_model_profile_provenance WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func (s *CamShowStore) ReviewModelProvenance(ctx context.Context, id, reviewerID int64, state string) (*CamModelProfileProvenance, error) {
	if reviewerID <= 0 || (state != CamModelReviewApproved && state != CamModelReviewRejected) {
		return nil, errors.New("reviewer and approved or rejected state are required")
	}
	now := time.Now().UTC()
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_model_profile_provenance SET review_state=?,reviewed_by=?,reviewed_at=?,updated_at=? WHERE id=? AND review_state='PENDING'`, state, reviewerID, now, now, id)
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed != 1 {
		return nil, fmt.Errorf("provenance %d is missing or already reviewed", id)
	}
	return s.FindModelProvenance(ctx, id)
}

type CamModelProfileUpdateInput struct {
	ID          int64
	DisplayName string
	Image       *string
	Notes       *string
	PerformerID *int64
	Status      string
}

func (s *CamShowStore) UpdateModelProfile(ctx context.Context, input CamModelProfileUpdateInput) (*CamModelProfile, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.ID <= 0 || input.DisplayName == "" || (input.Status != "ACTIVE" && input.Status != "INACTIVE" && input.Status != "UNKNOWN") {
		return nil, errors.New("valid model, display name, and status are required")
	}
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_models SET display_name=?,image=?,notes=?,performer_id=?,status=?,updated_at=? WHERE id=?`, input.DisplayName, input.Image, input.Notes, input.PerformerID, input.Status, time.Now().UTC(), input.ID)
	if err := requireOneRow(result, err); err != nil {
		return nil, err
	}
	return s.FindModelProfile(ctx, input.ID)
}

func (s *CamShowStore) ListModelProfiles(ctx context.Context) ([]CamModelProfile, error) {
	models, err := s.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]CamModelProfile, 0, len(models))
	for _, model := range models {
		profile, err := s.FindModelProfile(ctx, model.ID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, *profile)
	}
	return ret, nil
}

func (s *CamShowStore) ListModelProfilesForUser(ctx context.Context, userID int64, favoritesOnly bool) ([]CamModelProfile, error) {
	if userID <= 0 {
		return nil, errors.New("valid user is required")
	}
	var ids []int64
	query := `SELECT m.id FROM cam_models m LEFT JOIN cam_model_user_state us ON us.model_id=m.id AND us.user_id=?`
	if favoritesOnly {
		query += ` WHERE COALESCE(us.favorite,0)=1`
	}
	query += ` ORDER BY COALESCE(us.favorite,0) DESC,m.display_name COLLATE NOCASE,m.id`
	if err := dbWrapper.Select(ctx, &ids, query, userID); err != nil {
		return nil, err
	}
	ret := make([]CamModelProfile, 0, len(ids))
	for _, id := range ids {
		profile, err := s.FindModelProfileForUser(ctx, id, userID)
		if err != nil {
			return nil, err
		}
		if profile != nil {
			ret = append(ret, *profile)
		}
	}
	return ret, nil
}

func (s *CamShowStore) FindModelProfileForUser(ctx context.Context, modelID, userID int64) (*CamModelProfile, error) {
	if userID <= 0 {
		return nil, errors.New("valid user is required")
	}
	profile, err := s.FindModelProfile(ctx, modelID)
	if err != nil || profile == nil {
		return profile, err
	}
	profile.UserState, err = s.GetUserState(ctx, userID, modelID)
	return profile, err
}

func (s *CamShowStore) FindModelProfile(ctx context.Context, modelID int64) (*CamModelProfile, error) {
	model, err := s.FindModel(ctx, modelID)
	if err != nil || model == nil {
		return nil, err
	}
	ret := &CamModelProfile{Model: *model}
	err = dbWrapper.Select(ctx, &ret.Accounts, `SELECT a.id,a.model_id,a.site_id,a.handle,a.normalized_handle,a.status,a.valid_from,a.valid_to,a.profile_url,a.external_account_id,a.first_seen_at,a.last_seen_at,a.last_synced_at,a.source,a.confidence,s.name AS site_name,s.external_key AS site_external_key,s.base_url AS site_base_url,s.enabled AS site_enabled FROM cam_model_accounts a JOIN cam_sites s ON s.id=a.site_id WHERE a.model_id=? ORDER BY s.name COLLATE NOCASE,a.valid_to IS NULL DESC,a.valid_from,a.id`, modelID)
	if err != nil {
		return nil, err
	}
	ret.Aliases, err = s.ListAliases(ctx, modelID)
	if err != nil {
		return nil, err
	}
	err = dbWrapper.Select(ctx, &ret.Provenance, `SELECT id,model_id,account_id,provider,evidence_key,provider_record_id,source_url,observed_at,payload_json,confidence,review_state,reviewed_by,reviewed_at,created_at,updated_at FROM cam_model_profile_provenance WHERE model_id=? ORDER BY observed_at,id`, modelID)
	if err != nil { return nil, err }
	ret.SocialProfiles, err = s.ListModelSocialProfiles(ctx, modelID)
	return ret, err
}

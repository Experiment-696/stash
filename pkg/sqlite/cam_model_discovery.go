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

var ErrCamModelDiscoverySite = errors.New("discovery platform is not configured as an enabled cam site")

type camModelDiscoveryChangeRow struct {
	ID       int64
	ModelID  *int64
	Proposal string
}

type CamModelDiscoveryReviewResult struct {
	EvidenceID, ChangeID int64
	EvidenceStatus       CamModelProvenanceIngestStatus
	Disposition          string
}

// IngestDiscoveryReview stores evidence plus a review proposal and never mutates identity rows.
func (s *CamShowStore) IngestDiscoveryReview(ctx context.Context, modelID int64, o cammodel.ProfileObservation) (*CamModelDiscoveryReviewResult, error) {
	if modelID <= 0 || strings.TrimSpace(o.Platform) == "" || strings.TrimSpace(o.Username) == "" {
		return nil, errors.New("model, platform, and username are required")
	}
	var site CamSite
	if err := dbWrapper.Get(ctx, &site, "SELECT id,name,base_url,external_key,enabled,created_at,updated_at FROM cam_sites WHERE external_key=? AND enabled=1", o.Platform); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrCamModelDiscoverySite, o.Platform)
		}
		return nil, err
	}
	if model, err := s.FindModel(ctx, modelID); err != nil {
		return nil, err
	} else if model == nil {
		return nil, sql.ErrNoRows
	}
	disposition := "NEW_ACCOUNT"
	var owner int64
	err := dbWrapper.Get(ctx, &owner, "SELECT model_id FROM cam_model_accounts WHERE site_id=? AND normalized_handle=? ORDER BY CASE status WHEN 'ACTIVE' THEN 0 ELSE 1 END,id LIMIT 1", site.ID, normalizeCamIdentity(o.Username))
	if err == nil {
		if owner == modelID {
			disposition = "EXISTING_ACCOUNT"
		} else {
			disposition = "HANDLE_CONFLICT"
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	evidence, err := s.IngestModelProvenance(ctx, CamModelProvenanceInput{ModelID: modelID, Provider: o.Provider, EvidenceKey: o.EvidenceKey, ProviderRecordID: o.ProviderRecordID, SourceURL: o.SourceURL, ObservedAt: o.ObservedAt, PayloadJSON: o.PayloadJSON, Confidence: o.Confidence})
	if err != nil {
		return nil, err
	}
	proposal, err := json.Marshal(map[string]interface{}{"action": "REVIEW_ACCOUNT_EVIDENCE", "disposition": disposition, "evidence_id": evidence.Provenance.ID, "model_id": modelID, "site_id": site.ID, "platform": o.Platform, "username": o.Username, "source_url": o.SourceURL})
	if err != nil {
		return nil, err
	}
	result, err := dbWrapper.Exec(ctx, "INSERT OR IGNORE INTO cam_sync_changes(provider,external_event_id,entity_type,entity_id,proposed_change_json,status,created_at) VALUES(?,?,'CAM_MODEL_ACCOUNT_REVIEW',?,?,'PENDING',?)", o.Provider, o.EvidenceKey, modelID, string(proposal), time.Now().UTC())
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	var changeID int64
	if changed == 1 {
		changeID, err = result.LastInsertId()
	} else {
		var stored camModelDiscoveryChangeRow
		err = dbWrapper.Get(ctx, &stored, "SELECT id,entity_id AS modelid,proposed_change_json AS proposal FROM cam_sync_changes WHERE provider=? AND external_event_id=?", o.Provider, o.EvidenceKey)
		storedModelID, storedJSON := stored.ModelID, stored.Proposal
		changeID = stored.ID
		if err == nil && (storedModelID == nil || *storedModelID != modelID || storedJSON != string(proposal)) {
			err = ErrCamModelProvenanceConflict
		}
	}
	if err != nil {
		return nil, err
	}
	return &CamModelDiscoveryReviewResult{EvidenceID: evidence.Provenance.ID, ChangeID: changeID, EvidenceStatus: evidence.Status, Disposition: disposition}, nil
}

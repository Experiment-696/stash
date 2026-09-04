package manager

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/stashapp/stash/pkg/sqlite"
)

type CamModelProfileCreateServiceInput struct {
	DisplayName  string
	Image, Notes *string
	PerformerID  *int64
	Status       string
	Accounts     []sqlite.CamModelAccountInput
}

type CamModelProfileService struct{ Database *sqlite.Database }

func (s CamModelProfileService) Create(ctx context.Context, input CamModelProfileCreateServiceInput) (*sqlite.CamModelProfile, error) {
	model, err := s.Database.CamShow.CreateModel(ctx, input.DisplayName, input.PerformerID)
	if err != nil {
		return nil, err
	}
	profile, err := s.Database.CamShow.UpdateModelProfile(ctx, sqlite.CamModelProfileUpdateInput{ID: model.ID, DisplayName: input.DisplayName, Image: input.Image, Notes: input.Notes, PerformerID: input.PerformerID, Status: input.Status})
	if err != nil {
		return nil, err
	}
	for _, account := range input.Accounts {
		account.ModelID = model.ID
		if _, err := s.Database.CamShow.AddManualModelAccount(ctx, account); err != nil {
			return nil, err
		}
	}
	return s.Database.CamShow.FindModelProfile(ctx, profile.Model.ID)
}

func (s CamModelProfileService) Update(ctx context.Context, input sqlite.CamModelProfileUpdateInput) (*sqlite.CamModelProfile, error) {
	return s.Database.CamShow.UpdateModelProfile(ctx, input)
}
func (s CamModelProfileService) AddAccount(ctx context.Context, input sqlite.CamModelAccountInput) (*sqlite.CamModelProfile, error) {
	if _, err := s.Database.CamShow.AddManualModelAccount(ctx, input); err != nil {
		return nil, err
	}
	return s.Database.CamShow.FindModelProfile(ctx, input.ModelID)
}
func (s CamModelProfileService) RetireAccount(ctx context.Context, accountID int64, validTo time.Time) (*sqlite.CamModelProfile, error) {
	account, err := s.Database.CamShow.FindAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, sql.ErrNoRows
	}
	if err := s.Database.CamShow.CloseAccount(ctx, accountID, validTo); err != nil {
		return nil, err
	}
	return s.Database.CamShow.FindModelProfile(ctx, account.ModelID)
}
func (s CamModelProfileService) IngestEvidence(ctx context.Context, input sqlite.CamModelProvenanceInput) (*sqlite.CamModelProvenanceIngestResult, error) {
	return s.Database.CamShow.IngestModelProvenance(ctx, input)
}
func (s CamModelProfileService) ReviewEvidence(ctx context.Context, id, reviewerID int64, state string) (*sqlite.CamModelProfileProvenance, error) {
	if state != sqlite.CamModelReviewApproved && state != sqlite.CamModelReviewRejected {
		return nil, errors.New("review state must be APPROVED or REJECTED")
	}
	return s.Database.CamShow.ReviewModelProvenance(ctx, id, reviewerID, state)
}

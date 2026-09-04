package cammodel

import "context"

type ObservationIngestStatus string

const (
	ObservationInserted  ObservationIngestStatus = "INSERTED"
	ObservationUnchanged ObservationIngestStatus = "UNCHANGED"
)

type ObservationIngestResult struct {
	EvidenceID int64
	Status     ObservationIngestStatus
}

// PendingObservationStore is the complete provider-facing persistence seam. It
// exposes pending evidence ingestion only: account creation and identity merge
// operations are intentionally absent and therefore cannot be bypassed by a
// provider implementation.
type PendingObservationStore interface {
	IngestPendingProfileObservation(context.Context, int64, *int64, ProfileObservation) (ObservationIngestResult, error)
}

type IngestionService struct {
	store PendingObservationStore
}

func NewIngestionService(store PendingObservationStore) IngestionService {
	return IngestionService{store: store}
}

func (s IngestionService) IngestProfileObservation(ctx context.Context, modelID int64, accountID *int64, observation ProfileObservation) (ObservationIngestResult, error) {
	return s.store.IngestPendingProfileObservation(ctx, modelID, accountID, observation)
}

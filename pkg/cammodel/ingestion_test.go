package cammodel

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type pendingObservationStoreFixture struct {
	calls int
}

func (f *pendingObservationStoreFixture) IngestPendingProfileObservation(_ context.Context, _ int64, _ *int64, _ ProfileObservation) (ObservationIngestResult, error) {
	f.calls++
	return ObservationIngestResult{EvidenceID: 7, Status: ObservationInserted}, nil
}

func TestProviderIngestionServiceHasPendingEvidenceOnlyBoundary(t *testing.T) {
	store := &pendingObservationStoreFixture{}
	service := NewIngestionService(store)
	result, err := service.IngestProfileObservation(context.Background(), 3, nil, ProfileObservation{Provider: "fixture", EvidenceKey: "one", ObservedAt: time.Now().UTC(), PayloadJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || result.EvidenceID != 7 || result.Status != ObservationInserted {
		t.Fatalf("calls=%d result=%+v", store.calls, result)
	}
	storeType := reflect.TypeOf((*PendingObservationStore)(nil)).Elem()
	if storeType.NumMethod() != 1 || storeType.Method(0).Name != "IngestPendingProfileObservation" {
		t.Fatalf("provider store methods=%v", storeType.NumMethod())
	}
	serviceType := reflect.TypeOf(service)
	if serviceType.NumMethod() != 1 || serviceType.Method(0).Name != "IngestProfileObservation" {
		t.Fatalf("provider service methods=%v", serviceType.NumMethod())
	}
}

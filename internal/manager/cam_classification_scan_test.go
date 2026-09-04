package manager

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stashapp/stash/pkg/sqlite"
)

type fakeCamClassificationScanStore struct {
	candidates     []sqlite.CamClassificationCandidate
	enumerateErr   error
	applyErr       error
	enumeratedWith []string
	applied        []sqlite.CamClassificationCandidate
}

func (f *fakeCamClassificationScanStore) EnumerateClassificationCandidates(_ context.Context, roots []string) ([]sqlite.CamClassificationCandidate, error) {
	f.enumeratedWith = append([]string(nil), roots...)
	return f.candidates, f.enumerateErr
}

func (f *fakeCamClassificationScanStore) ApplyClassification(_ context.Context, candidates []sqlite.CamClassificationCandidate) (*sqlite.CamClassificationResult, error) {
	f.applied = append([]sqlite.CamClassificationCandidate(nil), candidates...)
	return &sqlite.CamClassificationResult{}, f.applyErr
}

func TestApplyCamClassificationAfterScanOrchestratesEnumerationAndApply(t *testing.T) {
	candidates := []sqlite.CamClassificationCandidate{{SceneID: 7, Basename: "2026-05-23 18-56-43.mp4", RelativePath: "captures/2026-05-23 18-56-43.mp4"}}
	store := &fakeCamClassificationScanStore{candidates: candidates}
	roots := []string{"/media/one", "/media/two"}

	if err := applyCamClassificationAfterScan(context.Background(), store, roots); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.enumeratedWith, roots) {
		t.Fatalf("enumeration roots=%v want=%v", store.enumeratedWith, roots)
	}
	if !reflect.DeepEqual(store.applied, candidates) {
		t.Fatalf("applied candidates=%+v want=%+v", store.applied, candidates)
	}
}

func TestApplyCamClassificationAfterScanStopsOnEnumerationError(t *testing.T) {
	want := errors.New("enumeration failed")
	store := &fakeCamClassificationScanStore{enumerateErr: want}

	if err := applyCamClassificationAfterScan(context.Background(), store, []string{"/media"}); !errors.Is(err, want) {
		t.Fatalf("error=%v want=%v", err, want)
	}
	if store.applied != nil {
		t.Fatalf("apply called with %+v", store.applied)
	}
}

func TestApplyCamClassificationAfterScanPropagatesApplyError(t *testing.T) {
	want := errors.New("apply failed")
	store := &fakeCamClassificationScanStore{
		candidates: []sqlite.CamClassificationCandidate{{SceneID: 9}},
		applyErr:   want,
	}

	if err := applyCamClassificationAfterScan(context.Background(), store, nil); !errors.Is(err, want) {
		t.Fatalf("error=%v want=%v", err, want)
	}
}

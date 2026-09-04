package manager

import (
	"context"

	"github.com/stashapp/stash/pkg/sqlite"
)

type camClassificationScanStore interface {
	EnumerateClassificationCandidates(context.Context, []string) ([]sqlite.CamClassificationCandidate, error)
	ApplyClassification(context.Context, []sqlite.CamClassificationCandidate) (*sqlite.CamClassificationResult, error)
}

func applyCamClassificationAfterScan(ctx context.Context, store camClassificationScanStore, roots []string) error {
	candidates, err := store.EnumerateClassificationCandidates(ctx, roots)
	if err != nil {
		return err
	}
	_, err = store.ApplyClassification(ctx, candidates)
	return err
}

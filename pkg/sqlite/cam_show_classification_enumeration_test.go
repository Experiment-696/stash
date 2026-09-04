package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
)

func TestEnumerateClassificationCandidatesExecutesOnSQLite(t *testing.T) {
	config.InitializeEmpty()
	db := NewDatabase()
	if err := db.Open(filepath.Join(t.TempDir(), "classification-enumeration.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := db.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Rollback(ctx)
	candidates, err := db.CamShow.EnumerateClassificationCandidates(ctx, []string{"/data"})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates=%+v", candidates)
	}
}

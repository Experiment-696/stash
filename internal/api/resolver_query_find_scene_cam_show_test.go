package api

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

func TestDefaultSceneLibraryCamShowSeparation(t *testing.T) {
	fromNil := defaultSceneLibraryCamShowSeparation(nil)
	if fromNil == nil || fromNil.ExcludeCamShows == nil || !*fromNil.ExcludeCamShows {
		t.Fatalf("nil filter default = %#v, want exclude_cam_shows=true", fromNil)
	}

	original := &models.SceneFilterType{}
	fromOmitted := defaultSceneLibraryCamShowSeparation(original)
	if fromOmitted == original {
		t.Fatal("defaulting mutated the caller-owned filter")
	}
	if original.ExcludeCamShows != nil {
		t.Fatal("caller-owned filter was modified")
	}
	if fromOmitted.ExcludeCamShows == nil || !*fromOmitted.ExcludeCamShows {
		t.Fatalf("omitted flag default = %#v, want true", fromOmitted.ExcludeCamShows)
	}

	include := false
	explicit := &models.SceneFilterType{ExcludeCamShows: &include}
	if got := defaultSceneLibraryCamShowSeparation(explicit); got != explicit || got.ExcludeCamShows == nil || *got.ExcludeCamShows {
		t.Fatalf("explicit false was not preserved: %#v", got)
	}
}

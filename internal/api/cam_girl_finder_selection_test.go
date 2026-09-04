package api

import (
	"github.com/stashapp/stash/pkg/cammodel"
	"testing"
)

func TestSelectCamGirlFinderObservationsRejectsStaleDuplicateAndExcludesUnselected(t *testing.T) {
	items := []cammodel.ProfileObservation{{EvidenceKey: "one"}, {EvidenceKey: "two"}}
	selected, err := selectCamGirlFinderObservations(items, []string{"two"})
	if err != nil || len(selected) != 1 || selected[0].EvidenceKey != "two" {
		t.Fatalf("selected=%+v err=%v", selected, err)
	}
	for name, keys := range map[string][]string{"empty": {}, "duplicate": {"one", "one"}, "stale": {"missing"}, "blank": {" "}} {
		t.Run(name, func(t *testing.T) {
			if _, err := selectCamGirlFinderObservations(items, keys); err == nil {
				t.Fatal("selection accepted")
			}
		})
	}
	if _, err := selectCamGirlFinderObservations([]cammodel.ProfileObservation{{EvidenceKey: "one"}, {EvidenceKey: "one"}}, []string{"one"}); err == nil {
		t.Fatal("provider duplicate accepted")
	}
}

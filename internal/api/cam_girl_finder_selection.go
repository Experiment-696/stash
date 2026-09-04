package api

import (
	"fmt"
	"strings"

	"github.com/stashapp/stash/pkg/cammodel"
)

func selectCamGirlFinderObservations(observations []cammodel.ProfileObservation, evidenceKeys []string) ([]cammodel.ProfileObservation, error) {
	if len(evidenceKeys) == 0 {
		return nil, ErrInput
	}
	available := make(map[string]cammodel.ProfileObservation, len(observations))
	for _, observation := range observations {
		if _, duplicate := available[observation.EvidenceKey]; duplicate {
			return nil, fmt.Errorf("%w: provider returned duplicate evidence key %q", ErrInput, observation.EvidenceKey)
		}
		available[observation.EvidenceKey] = observation
	}
	selected := make([]cammodel.ProfileObservation, 0, len(evidenceKeys))
	seen := make(map[string]struct{}, len(evidenceKeys))
	for _, key := range evidenceKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%w: selected evidence key is empty", ErrInput)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate selected evidence key %q", ErrInput, key)
		}
		seen[key] = struct{}{}
		observation, ok := available[key]
		if !ok {
			return nil, fmt.Errorf("%w: selected evidence key %q is stale or missing", ErrInput, key)
		}
		selected = append(selected, observation)
	}
	return selected, nil
}

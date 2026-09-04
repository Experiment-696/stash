package api

import (
	"context"
	"fmt"
	"sort"

	"github.com/stashapp/stash/internal/authz"
)

var moderatorSceneUpdateFields = fieldSet(
	"clientMutationId", "id", "title", "code", "details", "director",
	"url", "urls", "date", "studio_id", "gallery_ids", "performer_ids",
	"tag_ids", "groups", "movies", "stash_ids",
)

var moderatorBulkSceneUpdateFields = fieldSet(
	"clientMutationId", "ids", "title", "code", "details", "director",
	"url", "urls", "date", "studio_id", "gallery_ids", "performer_ids",
	"tag_ids", "group_ids", "movie_ids",
)

var moderatorPerformerUpdateFields = fieldSet(
	"id", "name", "disambiguation", "url", "urls", "gender", "birthdate",
	"ethnicity", "country", "eye_color", "height_cm", "measurements",
	"fake_tits", "penis_length", "circumcised", "career_length", "career_start",
	"career_end", "tattoos", "piercings", "alias_list", "twitter",
	"instagram", "tag_ids", "details", "death_date", "hair_color", "weight",
	"stash_ids",
)

var moderatorBulkPerformerUpdateFields = fieldSet(
	"clientMutationId", "ids", "disambiguation", "url", "urls", "gender",
	"birthdate", "ethnicity", "country", "eye_color", "height_cm", "measurements",
	"fake_tits", "penis_length", "circumcised", "career_length", "career_start",
	"career_end", "tattoos", "piercings", "alias_list", "twitter", "instagram",
	"tag_ids", "details", "death_date", "hair_color", "weight",
)

func fieldSet(fields ...string) map[string]struct{} {
	ret := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		ret[field] = struct{}{}
	}
	return ret
}

// validateModeratorMetadataFields is deliberately based on the GraphQL input
// map rather than decoded pointer values. That makes an explicitly supplied
// null forbidden field fail closed too, and lets validation finish before any
// image processing, hook execution, or transaction write occurs.
func validateModeratorMetadataFields(ctx context.Context, input map[string]interface{}, allowed map[string]struct{}) error {
	principal, err := authz.PrincipalFromContext(ctx)
	if err != nil || principal.Role != authz.RoleModerator {
		return nil
	}

	var forbidden []string
	for field := range input {
		if _, ok := allowed[field]; !ok {
			forbidden = append(forbidden, field)
		}
	}
	if len(forbidden) == 0 {
		return nil
	}

	sort.Strings(forbidden)
	return fmt.Errorf("moderator metadata update contains forbidden field(s): %v: %w", forbidden, authz.DeniedError{Capability: authz.LibraryDestructive})
}

func validateModeratorMetadataInputList(ctx context.Context, inputs []map[string]interface{}, allowed map[string]struct{}) error {
	for i, input := range inputs {
		if err := validateModeratorMetadataFields(ctx, input, allowed); err != nil {
			return fmt.Errorf("input %d: %w", i, err)
		}
	}
	return nil
}

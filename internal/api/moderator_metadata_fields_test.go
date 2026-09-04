package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stashapp/stash/internal/authz"
)

func metadataFieldTestContext(role authz.Role) context.Context {
	return authz.WithPrincipal(context.Background(), authz.Principal{
		UserID: "field-test-user",
		Role:   role,
		Status: authz.StatusActive,
	})
}

func assertModeratorFieldPolicy(t *testing.T, allowed map[string]struct{}, allowedField, forbiddenField string) {
	t.Helper()
	ctx := metadataFieldTestContext(authz.RoleModerator)
	if err := validateModeratorMetadataFields(ctx, map[string]interface{}{allowedField: "value"}, allowed); err != nil {
		t.Fatalf("allowed field %q rejected: %v", allowedField, err)
	}

	for _, value := range []interface{}{true, nil} {
		err := validateModeratorMetadataFields(ctx, map[string]interface{}{
			allowedField:   "value",
			forbiddenField: value,
		}, allowed)
		var denied authz.DeniedError
		if !errors.As(err, &denied) {
			t.Fatalf("mixed payload with forbidden field %q (%v) did not fail as forbidden: %v", forbiddenField, value, err)
		}
	}
}

func TestModeratorSceneUpdateFieldPolicy(t *testing.T) {
	assertModeratorFieldPolicy(t, moderatorSceneUpdateFields, "title", "primary_file_id")
	assertModeratorFieldPolicy(t, moderatorSceneUpdateFields, "stash_ids", "cover_image")
	assertModeratorFieldPolicy(t, moderatorSceneUpdateFields, "performer_ids", "rating100")
}

func TestModeratorScenesUpdatePreflightsEveryInput(t *testing.T) {
	ctx := metadataFieldTestContext(authz.RoleModerator)
	err := validateModeratorMetadataInputList(ctx, []map[string]interface{}{
		{"id": "1", "title": "allowed"},
		{"id": "2", "details": "allowed", "play_count": 1},
	}, moderatorSceneUpdateFields)
	var denied authz.DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("forbidden second input was not rejected before the batch: %v", err)
	}
}

func TestModeratorBulkSceneUpdateFieldPolicy(t *testing.T) {
	assertModeratorFieldPolicy(t, moderatorBulkSceneUpdateFields, "tag_ids", "organized")
	assertModeratorFieldPolicy(t, moderatorBulkSceneUpdateFields, "details", "custom_fields")
}

func TestModeratorPerformerUpdateFieldPolicy(t *testing.T) {
	assertModeratorFieldPolicy(t, moderatorPerformerUpdateFields, "name", "favorite")
	assertModeratorFieldPolicy(t, moderatorPerformerUpdateFields, "stash_ids", "image")
	assertModeratorFieldPolicy(t, moderatorPerformerUpdateFields, "details", "ignore_auto_tag")
}

func TestModeratorBulkPerformerUpdateFieldPolicy(t *testing.T) {
	assertModeratorFieldPolicy(t, moderatorBulkPerformerUpdateFields, "alias_list", "rating100")
	assertModeratorFieldPolicy(t, moderatorBulkPerformerUpdateFields, "details", "custom_fields")
}

func TestModeratorFieldPolicyLeavesAdminBehaviorIntact(t *testing.T) {
	ctx := metadataFieldTestContext(authz.RoleAdmin)
	for name, allowed := range map[string]map[string]struct{}{
		"scene":          moderatorSceneUpdateFields,
		"bulk scene":     moderatorBulkSceneUpdateFields,
		"performer":      moderatorPerformerUpdateFields,
		"bulk performer": moderatorBulkPerformerUpdateFields,
	} {
		if err := validateModeratorMetadataFields(ctx, map[string]interface{}{"future_admin_field": true}, allowed); err != nil {
			t.Errorf("%s Admin payload rejected: %v", name, err)
		}
	}
}

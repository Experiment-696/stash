package sqlite

import (
	"context"
	"strconv"

	"github.com/stashapp/stash/internal/authz"
)

const personalOHistoryKind = "O"

// personalActivityUserID returns the persisted user ID for authenticated API
// request contexts. Repository callers without a principal retain the legacy
// global activity view used by migrations, imports, and maintenance jobs.
func personalActivityUserID(ctx context.Context) (int64, bool) {
	principal, err := authz.PrincipalFromContext(ctx)
	if err != nil {
		return 0, false
	}
	if !principal.IsAuthenticated() {
		return 0, true
	}

	userID, err := strconv.ParseInt(principal.UserID, 10, 64)
	if err != nil || userID <= 0 {
		// A request principal must never fall back to shared legacy activity.
		return 0, true
	}

	return userID, true
}

func personalSceneOCountSQL(userID int64, sceneExpression string) string {
	return "SELECT COUNT(*) FROM user_scene_history WHERE user_id = " + strconv.FormatInt(userID, 10) +
		" AND scene_id = " + sceneExpression + " AND kind = '" + personalOHistoryKind + "'"
}

func personalImageOCountSQL(userID int64, imageExpression string) string {
	return "SELECT COALESCE(o_count, 0) FROM user_image_activity WHERE user_id = " + strconv.FormatInt(userID, 10) +
		" AND image_id = " + imageExpression
}

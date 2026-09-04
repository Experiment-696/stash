package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/pkg/cammodel"
)

type completedAliasSnapshotRow struct {
	SiteID          int64      `db:"site_id"`
	ModelID         int64      `db:"model_id"`
	NormalizedAlias string     `db:"normalized_alias"`
	ValidFrom       *time.Time `db:"valid_from"`
	ValidTo         *time.Time `db:"valid_to"`
	Current         bool       `db:"is_current"`
}

func (s *CamShowStore) ResolveCompletedRecording(ctx context.Context, root, relativePath string, parsed cammodel.CompletedParsedName) (cammodel.CompletedResolution, error) {
	fullPath := filepath.Clean(filepath.Join(root, relativePath))
	var sceneIDs []int64
	if err := dbWrapper.Select(ctx, &sceneIDs, `SELECT sf.scene_id
		FROM scenes_files sf JOIN files f ON f.id=sf.file_id JOIN folders fo ON fo.id=f.parent_folder_id
		WHERE fo.path || ? || f.basename = ? ORDER BY sf.scene_id`, string(filepath.Separator), fullPath); err != nil {
		return cammodel.CompletedResolution{}, err
	}
	normalize := func(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
	var siteIDs []int64
	if err := dbWrapper.Select(ctx, &siteIDs, `SELECT id FROM cam_sites
		WHERE lower(trim(COALESCE(external_key,'')))=? OR lower(trim(name))=? ORDER BY id`,
		normalize(parsed.Site), normalize(parsed.Site)); err != nil {
		return cammodel.CompletedResolution{}, err
	}
	var rows []completedAliasSnapshotRow
	if len(siteIDs) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(siteIDs)), ",")
		args := make([]interface{}, 0, len(siteIDs)+1)
		for _, id := range siteIDs {
			args = append(args, id)
		}
		args = append(args, normalize(parsed.Handle))
		if err := dbWrapper.Select(ctx, &rows, `SELECT site_id,model_id,normalized_alias,valid_from,valid_to,is_current
			FROM cam_model_aliases WHERE site_id IN (`+placeholders+`) AND lower(trim(normalized_alias))=? ORDER BY id`, args...); err != nil {
			return cammodel.CompletedResolution{}, err
		}
	}
	aliases := make([]cammodel.CompletedAliasBinding, len(rows))
	for i, row := range rows {
		aliases[i] = cammodel.CompletedAliasBinding{
			SiteID: row.SiteID, ModelID: row.ModelID, NormalizedAlias: row.NormalizedAlias,
			ValidFrom: row.ValidFrom, ValidTo: row.ValidTo, Current: row.Current,
		}
	}
	resolver := cammodel.CompletedIdentitySnapshotResolver{
		Normalize: normalize,
		Scenes:    map[string][]int64{filepath.Clean(relativePath): sceneIDs},
		Sites:     map[string][]int64{normalize(parsed.Site): siteIDs},
		Aliases:   aliases,
	}
	return resolver.ResolveCompletedRecording(ctx, root, relativePath, parsed)
}

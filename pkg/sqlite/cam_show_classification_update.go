package sqlite

import (
	"context"
	"errors"
	"strings"
	"time"
)

func (s *CamShowStore) UpdateClassificationRule(ctx context.Context, id int64, name, pattern, target, category string, enabled bool, tagIDs []int) (*CamClassificationRule, error) {
	if id <= 0 {
		return nil, errors.New("classification rule ID must be positive")
	}
	name, category = strings.TrimSpace(name), strings.TrimSpace(category)
	if name == "" || category == "" {
		return nil, errors.New("classification rule name and category are required")
	}
	if target != CamClassificationTargetBasename && target != CamClassificationTargetRelativePath {
		return nil, errors.New("classification target must be BASENAME or RELATIVE_PATH")
	}
	if _, err := compileCamClassificationPattern(pattern); err != nil {
		return nil, err
	}
	for _, tagID := range tagIDs {
		if tagID <= 0 {
			return nil, errors.New("classification tag IDs must be positive")
		}
	}
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_show_classification_rules SET name=?,pattern=?,target=?,category=?,enabled=?,updated_at=? WHERE id=?`, name, strings.TrimSpace(pattern), target, category, enabled, time.Now().UTC(), id)
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed != 1 {
		return nil, errors.New("classification rule not found")
	}
	if _, err := dbWrapper.Exec(ctx, `DELETE FROM cam_show_classification_rule_tags WHERE rule_id=?`, id); err != nil {
		return nil, err
	}
	for _, tagID := range uniqueSortedInts(tagIDs) {
		if _, err := dbWrapper.Exec(ctx, `INSERT INTO cam_show_classification_rule_tags(rule_id,tag_id) VALUES(?,?)`, id, tagID); err != nil {
			return nil, err
		}
	}
	return s.FindClassificationRule(ctx, id)
}

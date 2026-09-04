package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	CamClassificationTargetBasename     = "BASENAME"
	CamClassificationTargetRelativePath = "RELATIVE_PATH"
)

type CamClassificationRule struct {
	ID        int64               `db:"id"`
	Name      string              `db:"name"`
	Pattern   string              `db:"pattern"`
	Target    string              `db:"target"`
	Category  string              `db:"category"`
	Enabled   bool                `db:"enabled"`
	TagIDs    []int               `db:"-"`
	Tags      []CamShowLibraryTag `db:"-"`
	CreatedAt time.Time           `db:"created_at"`
	UpdatedAt time.Time           `db:"updated_at"`
}

type CamClassificationCandidate struct {
	SceneID      int64
	Basename     string
	RelativePath string
}

type CamClassificationItem struct {
	SceneID  int64
	Matched  bool
	Applied  bool
	Skipped  bool
	Conflict string
	Category string
	TagIDs   []int
}

type CamClassificationResult struct {
	Matched    int
	Applied    int
	Skipped    int
	Conflicted int
	Items      []CamClassificationItem
}

func compileCamClassificationPattern(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.New("classification pattern is required")
	}
	if !strings.HasPrefix(pattern, "(?i)") {
		pattern = "(?i)" + pattern
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid classification regex: %w", err)
	}
	return compiled, nil
}

func normalizeCamRelativePath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	return strings.TrimPrefix(path.Clean("/"+value), "/")
}

func (s *CamShowStore) CreateClassificationRule(ctx context.Context, name, pattern, target, category string, enabled bool, tagIDs []int) (*CamClassificationRule, error) {
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
	now := time.Now().UTC()
	result, err := dbWrapper.Exec(ctx, `INSERT INTO cam_show_classification_rules(name,pattern,target,category,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, name, strings.TrimSpace(pattern), target, category, enabled, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	for _, tagID := range uniqueSortedInts(tagIDs) {
		if _, err := dbWrapper.Exec(ctx, `INSERT INTO cam_show_classification_rule_tags(rule_id,tag_id) VALUES(?,?)`, id, tagID); err != nil {
			return nil, err
		}
	}
	return s.FindClassificationRule(ctx, id)
}

func (s *CamShowStore) FindClassificationRule(ctx context.Context, id int64) (*CamClassificationRule, error) {
	var rule CamClassificationRule
	err := dbWrapper.Get(ctx, &rule, `SELECT id,name,pattern,target,category,enabled,created_at,updated_at FROM cam_show_classification_rules WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := dbWrapper.Select(ctx, &rule.TagIDs, `SELECT tag_id FROM cam_show_classification_rule_tags WHERE rule_id=? ORDER BY tag_id`, id); err != nil {
		return nil, err
	}
	if err := dbWrapper.Select(ctx, &rule.Tags, `SELECT t.id,t.name FROM tags t JOIN cam_show_classification_rule_tags rt ON rt.tag_id=t.id WHERE rt.rule_id=? ORDER BY t.name COLLATE NOCASE,t.id`, id); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *CamShowStore) ListClassificationRules(ctx context.Context, enabledOnly bool) ([]CamClassificationRule, error) {
	query := `SELECT id,name,pattern,target,category,enabled,created_at,updated_at FROM cam_show_classification_rules`
	if enabledOnly {
		query += ` WHERE enabled=1`
	}
	query += ` ORDER BY id`
	var rules []CamClassificationRule
	if err := dbWrapper.Select(ctx, &rules, query); err != nil {
		return nil, err
	}
	for i := range rules {
		if err := dbWrapper.Select(ctx, &rules[i].TagIDs, `SELECT tag_id FROM cam_show_classification_rule_tags WHERE rule_id=? ORDER BY tag_id`, rules[i].ID); err != nil {
			return nil, err
		}
		if err := dbWrapper.Select(ctx, &rules[i].Tags, `SELECT t.id,t.name FROM tags t JOIN cam_show_classification_rule_tags rt ON rt.tag_id=t.id WHERE rt.rule_id=? ORDER BY t.name COLLATE NOCASE,t.id`, rules[i].ID); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func (s *CamShowStore) SetClassificationRuleEnabled(ctx context.Context, id int64, enabled bool) error {
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_show_classification_rules SET enabled=?,updated_at=? WHERE id=?`, enabled, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("classification rule not found")
	}
	return nil
}

func (s *CamShowStore) PreviewClassification(ctx context.Context, candidates []CamClassificationCandidate) (*CamClassificationResult, error) {
	rules, err := s.ListClassificationRules(ctx, true)
	if err != nil {
		return nil, err
	}
	type compiledRule struct {
		rule  CamClassificationRule
		regex *regexp.Regexp
	}
	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		re, compileErr := compileCamClassificationPattern(rule.Pattern)
		if compileErr != nil {
			return nil, fmt.Errorf("rule %d (%s): %w", rule.ID, rule.Name, compileErr)
		}
		compiled = append(compiled, compiledRule{rule, re})
	}
	result := &CamClassificationResult{Items: make([]CamClassificationItem, 0, len(candidates))}
	for _, candidate := range candidates {
		item := CamClassificationItem{SceneID: candidate.SceneID}
		categories, tagSet := map[string]struct{}{}, map[int]struct{}{}
		for _, entry := range compiled {
			target := candidate.Basename
			if entry.rule.Target == CamClassificationTargetRelativePath {
				target = normalizeCamRelativePath(candidate.RelativePath)
			}
			if entry.regex.MatchString(target) {
				item.Matched = true
				categories[entry.rule.Category] = struct{}{}
				for _, id := range entry.rule.TagIDs {
					tagSet[id] = struct{}{}
				}
			}
		}
		if !item.Matched {
			result.Items = append(result.Items, item)
			continue
		}
		result.Matched++
		if len(categories) != 1 {
			item.Skipped, item.Conflict = true, "matching rules produce distinct categories"
		} else {
			for category := range categories {
				item.Category = category
			}
			existing, findErr := s.FindShowByScene(ctx, candidate.SceneID)
			if findErr != nil {
				return nil, findErr
			}
			if existing != nil && existing.Category != item.Category {
				item.Skipped, item.Conflict = true, "existing Cam Show has a different category"
			}
		}
		for id := range tagSet {
			item.TagIDs = append(item.TagIDs, id)
		}
		sort.Ints(item.TagIDs)
		if item.Conflict != "" {
			result.Skipped++
			result.Conflicted++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (s *CamShowStore) ApplyClassification(ctx context.Context, candidates []CamClassificationCandidate) (*CamClassificationResult, error) {
	result, err := s.PreviewClassification(ctx, candidates)
	if err != nil {
		return nil, err
	}
	for i := range result.Items {
		item := &result.Items[i]
		if !item.Matched || item.Conflict != "" {
			continue
		}
		show, err := s.FindShowByScene(ctx, item.SceneID)
		if err != nil {
			return nil, err
		}
		if show == nil {
			if _, err = s.CreateShow(ctx, item.SceneID, item.Category, nil); err != nil {
				return nil, err
			}
		}
		for _, tagID := range item.TagIDs {
			if _, err := dbWrapper.Exec(ctx, `INSERT INTO scenes_tags(scene_id,tag_id) VALUES(?,?) ON CONFLICT(scene_id,tag_id) DO NOTHING`, item.SceneID, tagID); err != nil {
				return nil, err
			}
		}
		item.Applied = true
		result.Applied++
	}
	return result, nil
}

func uniqueSortedInts(values []int) []int {
	set := map[int]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	ret := make([]int, 0, len(set))
	for value := range set {
		ret = append(ret, value)
	}
	sort.Ints(ret)
	return ret
}

func camClassificationRelativePath(fullPath string, roots []string) (string, bool) {
	cleanPath := filepath.Clean(fullPath)
	bestRoot := ""
	for _, root := range roots {
		cleanRoot := filepath.Clean(root)
		relative, err := filepath.Rel(cleanRoot, cleanPath)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			continue
		}
		if bestRoot == "" || len(cleanRoot) > len(bestRoot) {
			bestRoot = cleanRoot
		}
	}
	if bestRoot == "" {
		return "", false
	}
	relative, err := filepath.Rel(bestRoot, cleanPath)
	if err != nil {
		return "", false
	}
	return normalizeCamRelativePath(relative), true
}

func (s *CamShowStore) EnumerateClassificationCandidates(ctx context.Context, roots []string) ([]CamClassificationCandidate, error) {
	type candidateRow struct {
		SceneID    int64  `db:"scene_id"`
		FolderPath string `db:"folder_path"`
		Basename   string `db:"basename"`
	}
	var rows []candidateRow
	err := dbWrapper.Select(ctx, &rows, `SELECT s.id AS scene_id, fo.path AS folder_path, f.basename AS basename
		FROM scenes s
		JOIN scenes_files sf ON sf.scene_id=s.id AND sf."primary"=1
		JOIN files f ON f.id=sf.file_id
		JOIN folders fo ON fo.id=f.parent_folder_id
		ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	ret := make([]CamClassificationCandidate, 0, len(rows))
	for _, row := range rows {
		relative, ok := camClassificationRelativePath(filepath.Join(row.FolderPath, row.Basename), roots)
		if !ok {
			continue
		}
		ret = append(ret, CamClassificationCandidate{SceneID: row.SceneID, Basename: row.Basename, RelativePath: relative})
	}
	return ret, nil
}

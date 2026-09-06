package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

const (
	savedFilterTable       = "saved_filters"
	savedFilterDefaultName = ""
)

func defaultFilterPreferenceKey(mode models.FilterMode) (string, error) {
	if !mode.IsValid() {
		return "", fmt.Errorf("invalid filter mode %q", mode)
	}
	return "default_filter:" + mode.String(), nil
}

func (qb *SavedFilterStore) SetDefaultForUser(ctx context.Context, userID int64, mode models.FilterMode, filterID *int) error {
	key, err := defaultFilterPreferenceKey(mode)
	if err != nil {
		return err
	}
	if filterID == nil {
		_, err = dbWrapper.Exec(ctx, `DELETE FROM user_preferences WHERE user_id = ? AND key = ?`, userID, key)
		return err
	}
	filter, err := qb.FindForUser(ctx, *filterID, userID)
	if err != nil {
		return err
	}
	if filter == nil || filter.Mode != mode {
		return errors.New("saved filter does not belong to this user and mode")
	}
	_, err = dbWrapper.Exec(ctx, `INSERT INTO user_preferences (user_id, key, value_json, updated_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(user_id, key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		userID, key, strconv.Itoa(*filterID), time.Now().UTC())
	return err
}

func (qb *SavedFilterStore) FindDefaultForUser(ctx context.Context, userID int64, mode models.FilterMode) (*models.SavedFilter, error) {
	key, err := defaultFilterPreferenceKey(mode)
	if err != nil {
		return nil, err
	}
	var raw string
	if err := dbWrapper.Get(ctx, &raw, `SELECT value_json FROM user_preferences WHERE user_id = ? AND key = ?`, userID, key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return nil, nil
	}
	filter, err := qb.FindForUser(ctx, id, userID)
	if err != nil || filter == nil || filter.Mode != mode {
		return nil, err
	}
	return filter, nil
}

type savedFilterRow struct {
	ID           int               `db:"id" goqu:"skipinsert"`
	UserID       *int64            `db:"user_id"`
	Mode         models.FilterMode `db:"mode"`
	Name         string            `db:"name"`
	FindFilter   string            `db:"find_filter"`
	ObjectFilter string            `db:"object_filter"`
	UIOptions    string            `db:"ui_options"`
}

func encodeJSONOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}

	encoded, err := json.Marshal(v)
	if err != nil {
		logger.Errorf("error encoding json %v: %v", v, err)
	}

	return string(encoded)
}

func decodeJSON(s string, v interface{}) {
	if s == "" {
		return
	}

	if err := json.Unmarshal([]byte(s), v); err != nil {
		logger.Errorf("error decoding json %q: %v", s, err)
	}
}

func (r *savedFilterRow) fromSavedFilter(o models.SavedFilter) {
	r.ID = o.ID
	r.UserID = o.UserID
	r.Mode = o.Mode
	r.Name = o.Name

	// encode the filters as json
	r.FindFilter = encodeJSONOrEmpty(o.FindFilter)
	r.ObjectFilter = encodeJSONOrEmpty(o.ObjectFilter)
	r.UIOptions = encodeJSONOrEmpty(o.UIOptions)
}

func (r *savedFilterRow) resolve() *models.SavedFilter {
	ret := &models.SavedFilter{
		ID:     r.ID,
		UserID: r.UserID,
		Mode:   r.Mode,
		Name:   r.Name,
	}

	// decode the filters from json
	if r.FindFilter != "" {
		ret.FindFilter = &models.FindFilterType{}
		decodeJSON(r.FindFilter, &ret.FindFilter)
	}
	if r.ObjectFilter != "" {
		ret.ObjectFilter = make(map[string]interface{})
		decodeJSON(r.ObjectFilter, &ret.ObjectFilter)
	}
	if r.UIOptions != "" {
		ret.UIOptions = make(map[string]interface{})
		decodeJSON(r.UIOptions, &ret.UIOptions)
	}

	return ret
}

func (qb *SavedFilterStore) FindForUser(ctx context.Context, id int, userID int64) (*models.SavedFilter, error) {
	q := qb.selectDataset().Where(qb.tableMgr.byID(id), qb.table().Col("user_id").Eq(userID))
	ret, err := qb.get(ctx, q)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ret, err
}

func (qb *SavedFilterStore) AllForUser(ctx context.Context, userID int64) ([]*models.SavedFilter, error) {
	return qb.getMany(ctx, qb.selectDataset().Where(qb.table().Col("user_id").Eq(userID)))
}

func (qb *SavedFilterStore) FindByModeForUser(ctx context.Context, mode models.FilterMode, userID int64) ([]*models.SavedFilter, error) {
	filters, err := qb.FindByMode(ctx, mode)
	if err != nil {
		return nil, err
	}
	ret := filters[:0]
	for _, filter := range filters {
		if filter.UserID != nil && *filter.UserID == userID {
			ret = append(ret, filter)
		}
	}
	return ret, nil
}

func (qb *SavedFilterStore) DestroyForUser(ctx context.Context, id int, userID int64) error {
	filter, err := qb.FindForUser(ctx, id, userID)
	if err != nil {
		return err
	}
	if filter == nil {
		return sql.ErrNoRows
	}
	key, err := defaultFilterPreferenceKey(filter.Mode)
	if err != nil {
		return err
	}
	if _, err := dbWrapper.Exec(ctx, `DELETE FROM user_preferences WHERE user_id = ? AND key = ? AND value_json = ?`, userID, key, strconv.Itoa(id)); err != nil {
		return err
	}
	result, err := dbWrapper.Exec(ctx, `DELETE FROM saved_filters WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return sql.ErrNoRows
	}
	return nil
}

type SavedFilterStore struct {
	repository
	tableMgr *table
}

func NewSavedFilterStore() *SavedFilterStore {
	return &SavedFilterStore{
		repository: repository{
			tableName: savedFilterTable,
			idColumn:  idColumn,
		},
		tableMgr: savedFilterTableMgr,
	}
}

func (qb *SavedFilterStore) table() exp.IdentifierExpression {
	return qb.tableMgr.table
}

func (qb *SavedFilterStore) selectDataset() *goqu.SelectDataset {
	return dialect.From(qb.table()).Select(qb.table().All())
}

func (qb *SavedFilterStore) Create(ctx context.Context, newObject *models.SavedFilter) error {
	var r savedFilterRow
	r.fromSavedFilter(*newObject)

	id, err := qb.tableMgr.insertID(ctx, r)
	if err != nil {
		return err
	}

	updated, err := qb.Find(ctx, id)
	if err != nil {
		return fmt.Errorf("finding after create: %w", err)
	}

	*newObject = *updated

	return nil
}

func (qb *SavedFilterStore) Update(ctx context.Context, updatedObject *models.SavedFilter) error {
	var r savedFilterRow
	r.fromSavedFilter(*updatedObject)

	if err := qb.tableMgr.updateByID(ctx, updatedObject.ID, r); err != nil {
		return err
	}

	return nil
}

func (qb *SavedFilterStore) Destroy(ctx context.Context, id int) error {
	return qb.destroyExisting(ctx, []int{id})
}

// returns nil, nil if not found
func (qb *SavedFilterStore) Find(ctx context.Context, id int) (*models.SavedFilter, error) {
	ret, err := qb.find(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ret, err
}

func (qb *SavedFilterStore) FindMany(ctx context.Context, ids []int, ignoreNotFound bool) ([]*models.SavedFilter, error) {
	ret := make([]*models.SavedFilter, len(ids))

	table := qb.table()
	q := qb.selectDataset().Prepared(true).Where(table.Col(idColumn).In(ids))
	unsorted, err := qb.getMany(ctx, q)
	if err != nil {
		return nil, err
	}

	for _, s := range unsorted {
		i := slices.Index(ids, s.ID)
		ret[i] = s
	}

	if !ignoreNotFound {
		for i := range ret {
			if ret[i] == nil {
				return nil, fmt.Errorf("filter with id %d not found", ids[i])
			}
		}
	}

	return ret, nil
}

// returns nil, sql.ErrNoRows if not found
func (qb *SavedFilterStore) find(ctx context.Context, id int) (*models.SavedFilter, error) {
	q := qb.selectDataset().Where(qb.tableMgr.byID(id))

	ret, err := qb.get(ctx, q)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (qb *SavedFilterStore) get(ctx context.Context, q *goqu.SelectDataset) (*models.SavedFilter, error) {
	ret, err := qb.getMany(ctx, q)
	if err != nil {
		return nil, err
	}

	if len(ret) == 0 {
		return nil, sql.ErrNoRows
	}

	return ret[0], nil
}

func (qb *SavedFilterStore) getMany(ctx context.Context, q *goqu.SelectDataset) ([]*models.SavedFilter, error) {
	const single = false
	var ret []*models.SavedFilter
	if err := queryFunc(ctx, q, single, func(r *sqlx.Rows) error {
		var f savedFilterRow
		if err := r.StructScan(&f); err != nil {
			return err
		}

		s := f.resolve()

		ret = append(ret, s)
		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (qb *SavedFilterStore) FindByMode(ctx context.Context, mode models.FilterMode) ([]*models.SavedFilter, error) {
	// SELECT * FROM %s WHERE mode = ? AND name != ? ORDER BY name ASC
	table := qb.table()

	// TODO - querying on groups needs to include movies
	// remove this when we migrate to remove the movies filter mode in the database
	var whereClause exp.Expression

	if mode == models.FilterModeGroups || mode == models.FilterModeMovies {
		whereClause = goqu.Or(
			table.Col("mode").Eq(models.FilterModeGroups),
			table.Col("mode").Eq(models.FilterModeMovies),
		)
	} else {
		whereClause = table.Col("mode").Eq(mode)
	}

	sq := qb.selectDataset().Prepared(true).Where(whereClause).Order(table.Col("name").Asc())
	ret, err := qb.getMany(ctx, sq)

	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (qb *SavedFilterStore) All(ctx context.Context) ([]*models.SavedFilter, error) {
	return qb.getMany(ctx, qb.selectDataset())
}

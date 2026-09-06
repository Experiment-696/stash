package sqlite

import (
	"context"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

type viewDateManager struct {
	tableMgr *viewHistoryTable
}

func (qb *viewDateManager) GetViewDates(ctx context.Context, id int) ([]time.Time, error) {
	return qb.tableMgr.getDates(ctx, id)
}

func (qb *viewDateManager) GetManyViewDates(ctx context.Context, ids []int) ([][]time.Time, error) {
	return qb.tableMgr.getManyDates(ctx, ids)
}

func (qb *viewDateManager) CountViews(ctx context.Context, id int) (int, error) {
	return qb.tableMgr.getCount(ctx, id)
}

func (qb *viewDateManager) GetManyViewCount(ctx context.Context, ids []int) ([]int, error) {
	return qb.tableMgr.getManyCount(ctx, ids)
}

func (qb *viewDateManager) CountAllViews(ctx context.Context) (int, error) {
	return qb.tableMgr.getAllCount(ctx)
}

func (qb *viewDateManager) CountUniqueViews(ctx context.Context) (int, error) {
	return qb.tableMgr.getUniqueCount(ctx)
}

func (qb *viewDateManager) LastView(ctx context.Context, id int) (*time.Time, error) {
	return qb.tableMgr.getLastDate(ctx, id)
}

func (qb *viewDateManager) GetManyLastViewed(ctx context.Context, ids []int) ([]*time.Time, error) {
	return qb.tableMgr.getManyLastDate(ctx, ids)

}

func (qb *viewDateManager) AddViews(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.addDates(ctx, id, dates)
}

func (qb *viewDateManager) DeleteViews(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.deleteDates(ctx, id, dates)
}

func (qb *viewDateManager) DeleteAllViews(ctx context.Context, id int) (int, error) {
	return qb.tableMgr.deleteAllDates(ctx, id)
}

type oDateManager struct {
	tableMgr *viewHistoryTable
}

func (qb *oDateManager) GetODates(ctx context.Context, id int) ([]time.Time, error) {
	if userID, ok := personalActivityUserID(ctx); ok {
		var ret []time.Time
		err := dbWrapper.Select(ctx, &ret, `SELECT occurred_at FROM user_scene_history
			WHERE user_id = ? AND scene_id = ? AND kind = ? ORDER BY occurred_at DESC, id DESC`, userID, id, personalOHistoryKind)
		return ret, err
	}
	return qb.tableMgr.getDates(ctx, id)
}

func (qb *oDateManager) GetManyODates(ctx context.Context, ids []int) ([][]time.Time, error) {
	if userID, ok := personalActivityUserID(ctx); ok {
		q := dialect.Select("scene_id", "occurred_at").From("user_scene_history").Where(
			goqu.Ex{"user_id": userID, "scene_id": ids, "kind": personalOHistoryKind},
		).Order(goqu.I("occurred_at").Desc(), goqu.I("id").Desc())
		ret := make([][]time.Time, len(ids))
		idToIndex := idToIndexMap(ids)
		err := queryFunc(ctx, q, false, func(rows *sqlx.Rows) error {
			var sceneID int
			var occurred Timestamp
			if err := rows.Scan(&sceneID, &occurred); err != nil {
				return err
			}
			ret[idToIndex[sceneID]] = append(ret[idToIndex[sceneID]], occurred.Timestamp)
			return nil
		})
		return ret, err
	}
	return qb.tableMgr.getManyDates(ctx, ids)
}

func (qb *oDateManager) GetOCount(ctx context.Context, id int) (int, error) {
	if userID, ok := personalActivityUserID(ctx); ok {
		var ret int
		err := dbWrapper.Get(ctx, &ret, `SELECT COUNT(*) FROM user_scene_history WHERE user_id = ? AND scene_id = ? AND kind = ?`, userID, id, personalOHistoryKind)
		return ret, err
	}
	return qb.tableMgr.getCount(ctx, id)
}

func (qb *oDateManager) GetManyOCount(ctx context.Context, ids []int) ([]int, error) {
	if userID, ok := personalActivityUserID(ctx); ok {
		q := dialect.Select("scene_id", goqu.COUNT("occurred_at")).From("user_scene_history").Where(
			goqu.Ex{"user_id": userID, "scene_id": ids, "kind": personalOHistoryKind},
		).GroupBy("scene_id")
		ret := make([]int, len(ids))
		idToIndex := idToIndexMap(ids)
		err := queryFunc(ctx, q, false, func(rows *sqlx.Rows) error {
			var sceneID, count int
			if err := rows.Scan(&sceneID, &count); err != nil {
				return err
			}
			ret[idToIndex[sceneID]] = count
			return nil
		})
		return ret, err
	}
	return qb.tableMgr.getManyCount(ctx, ids)
}

func (qb *oDateManager) GetAllOCount(ctx context.Context) (int, error) {
	if userID, ok := personalActivityUserID(ctx); ok {
		var ret int
		err := dbWrapper.Get(ctx, &ret, `SELECT COUNT(*) FROM user_scene_history WHERE user_id = ? AND kind = ?`, userID, personalOHistoryKind)
		return ret, err
	}
	return qb.tableMgr.getAllCount(ctx)
}

func (qb *oDateManager) GetUniqueOCount(ctx context.Context) (int, error) {
	if userID, ok := personalActivityUserID(ctx); ok {
		var ret int
		err := dbWrapper.Get(ctx, &ret, `SELECT COUNT(DISTINCT scene_id) FROM user_scene_history WHERE user_id = ? AND kind = ?`, userID, personalOHistoryKind)
		return ret, err
	}
	return qb.tableMgr.getUniqueCount(ctx)
}

func (qb *oDateManager) AddO(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.addDates(ctx, id, dates)
}

func (qb *oDateManager) DeleteO(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.deleteDates(ctx, id, dates)
}

func (qb *oDateManager) ResetO(ctx context.Context, id int) (int, error) {
	return qb.tableMgr.deleteAllDates(ctx, id)
}

package api

import (
	"context"
	"errors"
	"strconv"

	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func (r *queryResolver) AuditEvents(ctx context.Context, limit int, offset int) ([]*AuditEvent, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, errors.New("invalid audit page")
	}
	database := r.tokenDatabase()
	var rows []sqlite.AuditEvent
	var storageErr bool
	err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		if err := requirePersistedAdmin(ctx, txCtx, database); err != nil {
			return err
		}
		var listErr error
		rows, listErr = database.Audit.ListPage(txCtx, limit, offset)
		storageErr = listErr != nil
		return listErr
	})
	if err != nil {
		if storageErr {
			return nil, personalDataError("list security audit events", err)
		}
		return nil, err
	}
	ret := make([]*AuditEvent, len(rows))
	for i, row := range rows {
		var actorID *string
		if row.ActorUserID != nil {
			value := strconv.FormatInt(*row.ActorUserID, 10)
			actorID = &value
		}
		ret[i] = &AuditEvent{
			ID: strconv.FormatInt(row.ID, 10), OccurredAt: row.OccurredAt,
			ActorUserID: actorID, EventType: row.EventType, TargetType: row.TargetType,
			TargetID: row.TargetID, Result: row.Result,
		}
	}
	return ret, nil
}

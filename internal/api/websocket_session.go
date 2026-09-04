package api

import (
	"context"
	"time"

	gqlTransport "github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/pkg/txn"
)

const websocketSessionRevalidationInterval = 5 * time.Second

type activeSessionValidator interface {
	RequireActive(ctx context.Context, userID int64, id string, now time.Time) error
}

type activeTokenValidator interface {
	RequireActive(ctx context.Context, userID int64, id string, now time.Time) (authz.Principal, error)
}

func websocketSessionInit(manager txn.Manager, sessions activeSessionValidator, tokens activeTokenValidator) gqlTransport.WebsocketInitFunc {
	return websocketSessionInitWithInterval(manager, sessions, tokens, websocketSessionRevalidationInterval)
}

// websocketSessionInitWithInterval is websocketSessionInit with an
// injectable revalidation interval, so tests can exercise the monitor loop
// without waiting on the real (5s) production interval.
func websocketSessionInitWithInterval(manager txn.Manager, sessions activeSessionValidator, tokens activeTokenValidator, interval time.Duration) gqlTransport.WebsocketInitFunc {
	return func(ctx context.Context, _ gqlTransport.InitPayload) (context.Context, *gqlTransport.InitPayload, error) {
		if binding, ok := dbTokenBindingFromContext(ctx); ok {
			connectionCtx, cancel := context.WithCancel(ctx)
			go monitorWebsocketToken(connectionCtx, cancel, manager, tokens, binding, interval)
			return connectionCtx, nil, nil
		}
		binding, ok := dbSessionBindingFromContext(ctx)
		if !ok {
			return ctx, nil, nil
		}

		connectionCtx, cancel := context.WithCancel(ctx)
		go monitorWebsocketSession(connectionCtx, cancel, manager, sessions, binding, interval)
		return connectionCtx, nil, nil
	}
}

func monitorWebsocketToken(ctx context.Context, cancel context.CancelFunc, manager txn.Manager, tokens activeTokenValidator, binding dbTokenBinding, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			checkCtx, checkCancel := context.WithTimeout(ctx, interval)
			principal := authz.Principal{}
			err := txn.WithReadTxn(checkCtx, manager, func(txCtx context.Context) error {
				var validateErr error
				principal, validateErr = tokens.RequireActive(txCtx, binding.UserID, binding.ID, now)
				return validateErr
			})
			checkCancel()
			if err != nil || !sameTokenPrincipal(binding.Principal, principal) {
				return
			}
		}
	}
}

func sameTokenPrincipal(left, right authz.Principal) bool {
	if left.UserID != right.UserID || left.Role != right.Role || left.Status != right.Status || len(left.TokenScopes) != len(right.TokenScopes) {
		return false
	}
	for capability := range left.TokenScopes {
		if _, ok := right.TokenScopes[capability]; !ok {
			return false
		}
	}
	return true
}

func monitorWebsocketSession(ctx context.Context, cancel context.CancelFunc, manager txn.Manager, sessions activeSessionValidator, binding dbSessionBinding, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			checkCtx, checkCancel := context.WithTimeout(ctx, interval)
			err := txn.WithReadTxn(checkCtx, manager, func(txCtx context.Context) error {
				return sessions.RequireActive(txCtx, binding.UserID, binding.ID, now)
			})
			checkCancel()
			if err != nil {
				return
			}
		}
	}
}

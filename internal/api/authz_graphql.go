package api

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

func graphqlAuthorizationMiddleware(registry *authz.Registry, database *sqlite.Database) graphql.FieldMiddleware {
	return func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		field := graphql.GetFieldContext(ctx)
		if field == nil {
			return nil, fmt.Errorf("GraphQL authorization field context is missing")
		}
		if !isGraphQLRootObject(field.Object) {
			return next(ctx)
		}
		if err := authorizeGraphQLRootWithBootstrap(ctx, registry, database, field.Object, field.Field.Name); err != nil {
			return nil, err
		}
		return next(ctx)
	}
}

func authorizeGraphQLRootWithBootstrap(ctx context.Context, registry *authz.Registry, database *sqlite.Database, object, fieldName string) error {
	if session.IsMigrationRequest(ctx) {
		if !migrationWindowOpen(database) {
			session.ConsumeMigrationToken()
			return authz.UnauthenticatedError{}
		}
		if (object == "Query" && fieldName == "migrationStatus") ||
			(object == "Mutation" && fieldName == "migrate") {
			return nil
		}
		return authz.DeniedError{Capability: authz.SystemConfigure}
	}
	if isBootstrapWindowRoot(object, fieldName) && bootstrapWindowOpen(ctx, database, object, fieldName) {
		return nil
	}
	return authorizeGraphQLRoot(ctx, registry, object, fieldName)
}

func isBootstrapWindowRoot(object, name string) bool {
	if object == "Query" {
		return name == "bootstrapConfiguration"
	}
	if object == "Mutation" {
		return name == "setup" || name == "bootstrapConfigureUI" || name == "bootstrapFirstAdmin"
	}
	return false
}

func bootstrapWindowOpen(ctx context.Context, database *sqlite.Database, object, fieldName string) bool {
	if (!session.IsLocalRequest(ctx) && !session.IsBootstrapRequest(ctx)) ||
		database == nil {
		return false
	}
	if database.Ready() != nil {
		// A genuinely fresh installation has no open database yet. Permit only
		// the minimal status query and the mutation that creates it; account and
		// UI bootstrap remain unavailable until the database is ready.
		if manager.GetInstance().GetSystemStatus().Status != manager.SystemStatusEnumSetup {
			return false
		}
		return (object == "Query" && fieldName == "bootstrapConfiguration") ||
			(object == "Mutation" && fieldName == "setup")
	}
	count := -1
	if err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		var countErr error
		count, countErr = database.User.Count(txCtx)
		return countErr
	}); err != nil {
		return false
	}
	return count == 0
}

func isGraphQLRootObject(object string) bool {
	return object == "Query" || object == "Mutation" || object == "Subscription"
}

func authorizeGraphQLRoot(ctx context.Context, registry *authz.Registry, object, fieldName string) error {
	if fieldName == "__schema" || fieldName == "__type" || fieldName == "__typename" {
		return nil
	}
	var kind authz.SurfaceKind
	switch object {
	case "Query":
		kind = authz.SurfaceGraphQLQuery
	case "Mutation":
		kind = authz.SurfaceGraphQLMutation
	case "Subscription":
		kind = authz.SurfaceGraphQLSubscription
	default:
		return fmt.Errorf("unsupported GraphQL root object %q", object)
	}
	surface, err := registry.Lookup(kind, fieldName)
	if err != nil {
		return err
	}
	if surface.Capability == authz.PublicBootstrap {
		return authz.UnauthenticatedError{}
	}
	if surface.OwnerScoped {
		if isIntrinsicSelfGraphQLRoot(fieldName) {
			principal, principalErr := authz.PrincipalFromContext(ctx)
			if principalErr != nil {
				return principalErr
			}
			_, err = authz.RequireSurfaceContext(ctx, registry, kind, fieldName, principal.UserID)
			return err
		}
		if isResolverOwnedGraphQLRoot(fieldName) {
			principal, principalErr := authz.PrincipalFromContext(ctx)
			if principalErr != nil {
				return principalErr
			}
			return authz.Require(principal, surface.Capability)
		}
		// Root middleware cannot infer target ownership from a caller identity.
		// Resolver/repository integration must resolve the target record's owner
		// and call RequireSurfaceContext with that value before this is enabled.
		return authz.OwnerResolutionRequiredError{Kind: kind, Name: fieldName}
	}
	_, err = authz.RequireSurfaceContext(ctx, registry, kind, fieldName, "")
	return err
}

func isResolverOwnedGraphQLRoot(name string) bool {
	switch name {
	case "findSavedFilter", "findSavedFilters", "findDefaultFilter", "saveFilter", "destroySavedFilter", "setDefaultFilter",
		"performerSetFavorite", "performerSetRating", "sceneSetRating",
		"sceneSaveActivity", "sceneResetActivity", "imageIncrementO", "imageDecrementO", "imageResetO",
		"sceneIncrementO", "sceneDecrementO", "sceneAddO", "sceneDeleteO", "sceneResetO",
		"sceneIncrementPlayCount", "sceneAddPlay", "sceneDeletePlay", "sceneResetPlayCount":
		return true
	default:
		return false
	}
}

// Intrinsic-self roots expose no caller-selectable owner: their resolvers
// derive every record owner from the persisted principal. Resource-ID roots
// are intentionally excluded until their repository owner lookup is wired.
func isIntrinsicSelfGraphQLRoot(name string) bool {
	switch name {
	case "me", "myAPITokens", "myPreferences", "setMyHomepageRoute", "setMyTheme", "createMyAPIToken", "revokeMyAPIToken":
		return true
	default:
		return false
	}
}

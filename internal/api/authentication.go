package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/authservice"
	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/signedurl"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

const csrfHeader = "X-CSRF-Token"

const bearerTokenPrefix = "Bearer "

var errInvalidCSRF = errors.New("invalid CSRF token")

type dbSessionBinding struct {
	ID     string
	UserID int64
}

type dbSessionBindingContextKey struct{}

type dbTokenBinding struct {
	ID        string
	UserID    int64
	Principal authz.Principal
}

type dbTokenBindingContextKey struct{}

// Retained only as a compile-time marker for historical regression tests. It
// grants no authority; migration requests must carry the purpose-bound session.
type legacyMigrationPrincipalContextKey struct{}
type migrationResponseWriterContextKey struct{}

func isLegacyCookieAuthentication(r *http.Request, userID string, err error) bool {
	return err == nil && userID != "" && r.Header.Get(session.ApiKeyHeader) == "" && r.URL.Query().Get(session.ApiKeyParameter) == ""
}

func localNoCredentialMigrationRequest(_ *http.Request, _ *config.Config, _ *sqlite.Database) bool {
	return false
}

func dbSessionBindingFromContext(ctx context.Context) (dbSessionBinding, bool) {
	binding, ok := ctx.Value(dbSessionBindingContextKey{}).(dbSessionBinding)
	return binding, ok && binding.ID != "" && binding.UserID > 0
}

func dbTokenBindingFromContext(ctx context.Context) (dbTokenBinding, bool) {
	binding, ok := ctx.Value(dbTokenBindingContextKey{}).(dbTokenBinding)
	return binding, ok && binding.ID != "" && binding.UserID > 0 && binding.Principal.IsAuthenticated()
}

func isUnsafeMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

// readDBBearerToken parses the v2 token wire format. The identifier permits an
// indexed lookup while the independently random secret is only ever hashed.
func readDBBearerToken(r *http.Request) (id, secret string, ok bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, bearerTokenPrefix) {
		return "", "", false
	}
	credential := strings.TrimSpace(strings.TrimPrefix(header, bearerTokenPrefix))
	id, secret, found := strings.Cut(credential, ".")
	if !found || id == "" || secret == "" || strings.Contains(secret, ".") {
		return "", "", false
	}
	return id, secret, true
}

func rejectGraphQLMutationOverGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != gqlEndpoint {
		return false
	}
	document, err := parser.ParseQuery(&ast.Source{Input: r.URL.Query().Get("query")})
	if err != nil {
		return false // gqlgen returns its normal parse error.
	}
	for _, operation := range document.Operations {
		if operation.Operation == ast.Mutation {
			http.Error(w, "GraphQL mutations require POST", http.StatusMethodNotAllowed)
			return true
		}
	}
	return false
}

type migrationGraphQLRequest struct {
	Query         string `json:"query"`
	OperationName string `json:"operationName"`
}

const (
	maxMigrationFragmentDepth = 32
	maxMigrationSelections    = 128
)

type migrationSelectionWalk struct {
	document *ast.QueryDocument
	active   map[string]bool
	expanded map[string]bool
	count    int
}

func (w *migrationSelectionWalk) rootFields(selections ast.SelectionSet, depth int) ([]*ast.Field, bool) {
	if depth > maxMigrationFragmentDepth {
		return nil, false
	}
	var fields []*ast.Field
	for _, selection := range selections {
		w.count++
		if w.count > maxMigrationSelections {
			return nil, false
		}
		switch selected := selection.(type) {
		case *ast.Field:
			fields = append(fields, selected)
		case *ast.FragmentSpread:
			if w.active[selected.Name] || w.expanded[selected.Name] {
				return nil, false
			}
			fragment := w.document.Fragments.ForName(selected.Name)
			if fragment == nil || depth == maxMigrationFragmentDepth {
				return nil, false
			}
			w.active[selected.Name] = true
			w.expanded[selected.Name] = true
			nested, ok := w.rootFields(fragment.SelectionSet, depth+1)
			delete(w.active, selected.Name)
			if !ok {
				return nil, false
			}
			fields = append(fields, nested...)
		case *ast.InlineFragment:
			if depth == maxMigrationFragmentDepth {
				return nil, false
			}
			nested, ok := w.rootFields(selected.SelectionSet, depth+1)
			if !ok {
				return nil, false
			}
			fields = append(fields, nested...)
		default:
			return nil, false
		}
	}
	return fields, true
}

func migrationRootFields(document *ast.QueryDocument, selections ast.SelectionSet) ([]*ast.Field, bool) {
	walk := migrationSelectionWalk{
		document: document,
		active:   make(map[string]bool),
		expanded: make(map[string]bool),
	}
	return walk.rootFields(selections, 0)
}

func migrationRequestRouteAllowed(r *http.Request) bool {
	switch r.URL.Path {
	case "/", "/index.html", gqlEndpoint, "/css", "/javascript", "/customlocales", "/favicon.ico", "/manifest.json", "/stash_icon.svg", "/apple-touch-icon.png":
		return true
	default:
		return strings.HasPrefix(r.URL.Path, "/assets/") && len(r.URL.Path) > len("/assets/")
	}
}

func validateMigrationGraphQLRequest(r *http.Request) error {
	request := migrationGraphQLRequest{Query: r.URL.Query().Get("query"), OperationName: r.URL.Query().Get("operationName")}
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return errors.New("request denied")
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) == 0 || body[0] == '[' || json.Unmarshal(body, &request) != nil {
			return errors.New("request denied")
		}
	}
	document, err := parser.ParseQuery(&ast.Source{Input: request.Query})
	if err != nil || len(document.Operations) != 1 {
		return errors.New("request denied")
	}
	operation := document.Operations[0]
	if operation.Name == "" || request.OperationName == "" || request.OperationName != operation.Name {
		return errors.New("request denied")
	}
	fields, ok := migrationRootFields(document, operation.SelectionSet)
	if !ok || len(fields) != 1 {
		return errors.New("request denied")
	}
	allowed := operation.Operation == ast.Query && fields[0].Name == "migrationStatus"
	allowed = allowed || operation.Operation == ast.Mutation && fields[0].Name == "migrate"
	if !allowed {
		return errors.New("request denied")
	}
	return nil
}

func isPublicUIAsset(r *http.Request) bool {
	return r.URL.Path == "/css" || r.URL.Path == "/favicon.ico" || r.URL.Path == "/manifest.json" ||
		r.URL.Path == "/apple-touch-icon.png" || strings.HasPrefix(r.URL.Path, "/assets/")
}

func allowUnauthenticated(r *http.Request) bool {
	// #2715 - allow access to UI files
	return strings.HasPrefix(r.URL.Path, loginEndpoint) || r.URL.Path == logoutEndpoint ||
		isPublicUIAsset(r)
}

// authenticateSignedRequest checks if the request is a valid signed media request.
// Returns the matched username and true if valid, or empty string and false otherwise.
func authenticateSignedRequest(r *http.Request) (string, bool) {
	if !strings.HasPrefix(r.URL.Path, "/scene/") {
		return "", false
	}
	c := config.GetInstance()
	if !c.HasCredentials() {
		return "", false
	}
	q := r.URL.Query()
	if q.Get(signedurl.CIDParam) == "" || q.Get(signedurl.ExpiresParam) == "" || q.Get(signedurl.SigParam) == "" {
		return "", false
	}
	username, secret, found := resolveCredentialID(c, q.Get(signedurl.CIDParam))
	if !found {
		logger.Warnf("signed URL credential ID mismatch")
		return "", false
	}
	if _, err := signedurl.VerifyURL(r.URL.Path, q, secret); err != nil {
		logger.Warnf("signed URL verification failed: %v", err)
		return "", false
	}
	return username, true
}

func shouldRecoverInvalidDBSession(r *http.Request) bool {
	if r.URL.Path == loginEndpoint || r.URL.Path == logoutEndpoint {
		return true
	}
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && isPublicUIAsset(r) {
		return true
	}
	if r.Method != http.MethodGet || r.URL.Path == gqlEndpoint {
		return false
	}
	ext := path.Ext(r.URL.Path)
	return ext == "" || ext == ".html"
}

func migrationWindowOpen(database *sqlite.Database) bool {
	return database != nil && database.Ready() != nil &&
		database.Version() < database.AppSchemaVersion() &&
		manager.GetInstance().GetSystemStatus().Status == manager.SystemStatusEnumNeedsMigration
}

func authenticateHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cache-Control", "no-store")
			if rejectGraphQLMutationOverGET(w, r) {
				return
			}
			c := config.GetInstance()

			r = session.SetLocalRequest(r)
			ctx := r.Context()

			// Check for signed media requests
			if username, ok := authenticateSignedRequest(r); ok {
				ctx := r.Context()
				ctx = session.SetCurrentUserID(ctx, username)
				r = r.WithContext(ctx)
				next.ServeHTTP(w, r)
				return
			}

			r = session.SetLocalRequest(r)

			mgr := manager.GetInstance()
			migrationRequest := false
			if migrationWindowOpen(mgr.Database) {
				_, _, tokenErr := session.EnsureMigrationToken(time.Now(), c.GetConfigPathAbs())
				if tokenErr != nil {
					logger.Errorf("Unable to create protected migration-token handoff: %v", tokenErr)
					http.Error(w, "Database migration access is unavailable", http.StatusServiceUnavailable)
					return
				}
				if isUnsafeMethod(r.Method) && (r.Header.Get(session.MigrationTokenHeader) != "" || r.URL.Query().Get(session.MigrationTokenQuery) != "") {
					http.Error(w, "request denied", http.StatusForbidden)
					return
				}
				var exchanged bool
				var sessionToken string
				r, sessionToken, _, exchanged = session.SetMigrationRequest(r, time.Now())
				migrationRequest = session.IsMigrationRequest(r.Context())
				if exchanged {
					session.SetMigrationCookie(w, sessionToken, session.MigrationTokenExpiry())
					if r.Method == http.MethodGet && r.URL.Query().Get(session.MigrationTokenQuery) != "" {
						cleanURL := *r.URL
						query := cleanURL.Query()
						query.Del(session.MigrationTokenQuery)
						cleanURL.RawQuery = query.Encode()
						http.Redirect(w, r, cleanURL.String(), http.StatusSeeOther)
						return
					}
				}
				if migrationRequest && !migrationRequestRouteAllowed(r) {
					http.Error(w, "request denied", http.StatusForbidden)
					return
				}
				if migrationRequest && isUnsafeMethod(r.Method) &&
					!session.VerifyMigrationCSRF(r.Header.Get(session.MigrationCSRFHeader)) {
					http.Error(w, "request denied", http.StatusForbidden)
					return
				}
				if migrationRequest && r.URL.Path == gqlEndpoint {
					if err := validateMigrationGraphQLRequest(r); err != nil {
						http.Error(w, "request denied", http.StatusForbidden)
						return
					}
				}
			} else {
				session.ConsumeMigrationToken()
				session.ClearMigrationCookie(w)
			}
			if !c.HasCredentials() && mgr.Database != nil && !migrationWindowOpen(mgr.Database) {
				bootstrapWindow := false
				if mgr.Database.Ready() == nil {
					count := 0
					if countErr := txn.WithReadTxn(r.Context(), mgr.Database, func(ctx context.Context) error {
						var readErr error
						count, readErr = mgr.Database.User.Count(ctx)
						return readErr
					}); countErr == nil {
						bootstrapWindow = count == 0
					}
				}
				if bootstrapWindow {
					token, tokenErr := session.EnsureBootstrapToken(time.Now())
					if tokenErr != nil {
						logger.Errorf("Unable to create one-time setup token: %v", tokenErr)
					}
					r = session.SetBootstrapRequest(r, time.Now())
					if tokenErr == nil && session.IsBootstrapRequest(r.Context()) &&
						r.Method == http.MethodGet &&
						r.URL.Query().Get(session.BootstrapTokenQuery) != "" {
						session.SetBootstrapCookie(
							w,
							token,
							session.BootstrapTokenExpiry(),
						)
						cleanURL := *r.URL
						query := cleanURL.Query()
						query.Del(session.BootstrapTokenQuery)
						cleanURL.RawQuery = query.Encode()
						http.Redirect(w, r, cleanURL.String(), http.StatusSeeOther)
						return
					}
				}
			}
			userID := ""
			var principal *authz.Principal
			var sessionBinding *dbSessionBinding
			var tokenBinding *dbTokenBinding
			usedDBSession := false
			var err error
			if migrationRequest {
				err = nil
			} else if tokenID, tokenSecret, ok := readDBBearerToken(r); ok {
				if mgr.Database.Ready() != nil {
					err = session.ErrUnauthorized
				} else {
					err = txn.WithReadTxn(r.Context(), mgr.Database, func(ctx context.Context) error {
						resolved, authErr := mgr.Database.APIToken.Authenticate(ctx, tokenID, tokenSecret, time.Now())
						if authErr != nil {
							return authErr
						}
						principal = &resolved
						userID = resolved.UserID
						persistedUserID, parseErr := strconv.ParseInt(resolved.UserID, 10, 64)
						if parseErr != nil || persistedUserID <= 0 {
							return session.ErrUnauthorized
						}
						tokenBinding = &dbTokenBinding{ID: tokenID, UserID: persistedUserID, Principal: resolved}
						return nil
					})
				}
			} else if strings.HasPrefix(r.Header.Get("Authorization"), bearerTokenPrefix) {
				err = session.ErrUnauthorized
			} else if sessionID, sessionSecret, ok := readDBSessionCookie(r); ok {
				usedDBSession = true
				retryer := txn.Retryer{Manager: mgr.Database, Retries: 5}
				err = retryer.WithTxn(r.Context(), func(ctx context.Context) error {
					sessionRecord, resolved, authErr := mgr.Database.Session.AuthenticatePrincipal(ctx, sessionID, sessionSecret, time.Now())
					if authErr != nil {
						return authErr
					}
					if isUnsafeMethod(r.Method) && !mgr.Database.Session.VerifyCSRF(sessionRecord, r.Header.Get(csrfHeader)) {
						return errInvalidCSRF
					}
					if touchErr := mgr.Database.Session.Touch(ctx, sessionRecord.UserID, sessionRecord.ID, time.Now(), authservice.DefaultSessionIdle); touchErr != nil {
						return touchErr
					}
					principal = &resolved
					sessionBinding = &dbSessionBinding{ID: sessionRecord.ID, UserID: sessionRecord.UserID}
					userID = resolved.UserID
					return nil
				})
			} else {
				userID, err = mgr.SessionStore.Authenticate(w, r)
				if err == nil && userID != "" && isLocalPluginCallback(mgr.SessionStore, r) {
					resolved, resolveErr := resolvePluginPrincipal(r.Context(), mgr.Database, userID)
					if resolveErr != nil {
						err = resolveErr
					} else {
						principal = &resolved
					}
				}
			}
			if err != nil && usedDBSession &&
				(errors.Is(err, session.ErrUnauthorized) || errors.Is(err, sql.ErrNoRows)) &&
				shouldRecoverInvalidDBSession(r) {
				// A stale browser cookie must not strand the UI on a raw 401. Clear
				// it and continue anonymously so page requests redirect to login and
				// the login handler can establish a fresh session.
				clearDBSessionCookies(w, r)
				userID = ""
				err = nil
			}
			if err != nil {
				if errors.Is(err, errInvalidCSRF) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if !errors.Is(err, session.ErrUnauthorized) && !errors.Is(err, sql.ErrNoRows) {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				// unauthorized error
				w.Header().Add("WWW-Authenticate", "FormBased")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			if !c.IsNewSystem() && !c.HasCredentials() {
				requestIP, requestIPErr := getRequestIPFromCtx(ctx)
				if requestIPErr != nil {
					logger.Errorf("error getting request IP: %v", requestIPErr)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if accessErr := checkAllowPublicWithoutAuth(c, requestIP); accessErr != nil {
					httpError(w, r, "Access denied: Stash cannot be accessed from public IPs when authentication is not configured", http.StatusForbidden)
					return
				}
			}

			ctx = context.WithValue(r.Context(), migrationResponseWriterContextKey{}, w)
			if migrationRequest {
				migrationPrincipal := authz.Principal{
					UserID: "migration:bootstrap",
					Role:   authz.RoleAdmin,
					Status: authz.StatusActive,
				}
				principal = &migrationPrincipal
			}

			authRequired := c.HasCredentials()
			if !authRequired {
				if err := mgr.Database.Ready(); err != nil {
					if !migrationRequest {
						http.Error(w, "Database migration is required", http.StatusServiceUnavailable)
						return
					}
					// Permit the local browser shell and GraphQL endpoint. The
					// sentinel principal's exact GraphQL allowlist is enforced by
					// graphqlAuthorizationMiddleware; all resource routes remain
					// unavailable while the database is closed.
					// Use the same exact route predicate as the earlier migration
					// gate. A second partial asset list here previously rejected
					// manifest/customization requests only when legacy credentials
					// were disabled, leaving the migration SPA blank.
					if !migrationRequestRouteAllowed(r) {
						http.Error(w, "Database migration is required", http.StatusServiceUnavailable)
						return
					}
				} else {
					count := 0
					if countErr := txn.WithReadTxn(r.Context(), mgr.Database, func(ctx context.Context) error {
						var readErr error
						count, readErr = mgr.Database.User.Count(ctx)
						return readErr
					}); countErr != nil {
						http.Error(w, "request denied", http.StatusInternalServerError)
						return
					}
					authRequired = count > 0
				}
			}
			if authRequired {
				// authentication is required
				if userID == "" && !allowUnauthenticated(r) && !migrationRequest {
					// if graphql or a non-webpage was requested, we just return a forbidden error
					ext := path.Ext(r.URL.Path)
					if r.URL.Path == gqlEndpoint || (ext != "" && ext != ".html") {
						w.Header().Add("WWW-Authenticate", "FormBased")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}

					prefix := getProxyPrefix(r)

					// otherwise redirect to the login page
					returnURL := url.URL{
						Path:     prefix + r.URL.Path,
						RawQuery: r.URL.RawQuery,
					}
					q := make(url.Values)
					q.Set(returnURLParam, returnURL.String())
					u := url.URL{
						Path:     prefix + loginEndpoint,
						RawQuery: q.Encode(),
					}
					http.Redirect(w, r, u.String(), http.StatusFound)
					return
				}
			}

			ctx = session.SetCurrentUserID(ctx, userID)
			if principal != nil {
				ctx = authz.WithPrincipal(ctx, *principal)
			}
			if sessionBinding != nil {
				ctx = context.WithValue(ctx, dbSessionBindingContextKey{}, *sessionBinding)
			}
			if tokenBinding != nil {
				ctx = context.WithValue(ctx, dbTokenBindingContextKey{}, *tokenBinding)
			}

			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

func httpError(w http.ResponseWriter, r *http.Request, text string, status int) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"error": "%s"}`, text)
	} else {
		http.Error(w, text, status)
	}
}

func checkAllowPublicWithoutAuth(c *config.Config, requestIP net.IP) error {
	if c.IsNewSystem() || c.HasCredentials() {
		return nil
	}
	if !isLocalIP(requestIP) && !matchIPWhitelist(c, requestIP) {
		return fmt.Errorf("stash accessed from external IP %s", requestIP.String())
	}
	return nil
}

func matchIPWhitelist(c *config.Config, requestIP net.IP) bool {
	nets, addrs := c.GetPublicWhitelist()
	for _, addr := range addrs {
		if addr.Equal(requestIP) {
			return true
		}
	}
	for _, network := range nets {
		if network.Contains(requestIP) {
			return true
		}
	}
	return false
}

func isLocalPluginCallback(store *session.Store, r *http.Request) bool {
	return session.IsLocalRequest(r.Context()) && store.IsPluginRequest(r)
}

func resolvePluginPrincipal(ctx context.Context, database *sqlite.Database, userID string) (authz.Principal, error) {
	var principal authz.Principal
	err := txn.WithReadTxn(ctx, database, func(txCtx context.Context) error {
		id, parseErr := strconv.ParseInt(userID, 10, 64)
		if parseErr != nil || id <= 0 {
			return session.ErrUnauthorized
		}
		user, findErr := database.User.Find(txCtx, id)
		if findErr != nil || user == nil || user.Status != authz.StatusActive {
			return session.ErrUnauthorized
		}
		principal = authz.Principal{UserID: userID, Role: user.Role, Status: user.Status}
		return nil
	})
	return principal, err
}

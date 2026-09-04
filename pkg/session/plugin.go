package session

import (
	"context"
	"encoding/gob"
	"net/http"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/plugin/hook"
)

type VisitedPluginHook struct {
	PluginID string
	HookType hook.TriggerEnum
}

const pluginRequestKey = "plugin_request"

func init() {
	gob.Register([]VisitedPluginHook{})
}

func (s *Store) VisitedPluginHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// get the visited plugins from the cookie and set in the context
			session, err := s.sessionStore.Get(r, cookieName)

			// ignore errors
			if err == nil {
				val := session.Values[visitedPluginHooksKey]

				visitedPlugins, _ := val.([]VisitedPluginHook)

				ctx := setVisitedPluginHooks(r.Context(), visitedPlugins)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

func GetVisitedPluginHooks(ctx context.Context) []VisitedPluginHook {
	ctxVal := ctx.Value(contextVisitedPlugins)
	if ctxVal != nil {
		return ctxVal.([]VisitedPluginHook)
	}

	return nil
}

func AddVisitedPluginHook(ctx context.Context, pluginID string, hookType hook.TriggerEnum) context.Context {
	curVal := GetVisitedPluginHooks(ctx)
	curVal = append(curVal, VisitedPluginHook{PluginID: pluginID, HookType: hookType})
	return setVisitedPluginHooks(ctx, curVal)
}

func setVisitedPluginHooks(ctx context.Context, visitedPlugins []VisitedPluginHook) context.Context {
	return context.WithValue(ctx, contextVisitedPlugins, visitedPlugins)
}

func (s *Store) MakePluginCookie(ctx context.Context) *http.Cookie {
	currentUser := GetCurrentUserID(ctx)
	visitedPlugins := GetVisitedPluginHooks(ctx)

	session := sessions.NewSession(s.sessionStore, cookieName)
	if currentUser != nil {
		session.Values[userIDKey] = *currentUser
	}

	session.Values[visitedPluginHooksKey] = visitedPlugins
	session.Values[pluginRequestKey] = true

	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values,
		s.sessionStore.Codecs...)
	if err != nil {
		logger.Errorf("error creating session cookie: %s", err.Error())
		return nil
	}

	return sessions.NewCookie(session.Name(), encoded, session.Options)
}

func (s *Store) IsPluginRequest(r *http.Request) bool {
	pluginSession, err := s.sessionStore.Get(r, cookieName)
	if err != nil {
		return false
	}
	marked, _ := pluginSession.Values[pluginRequestKey].(bool)
	userID, _ := pluginSession.Values[userIDKey].(string)
	return marked && userID != ""
}

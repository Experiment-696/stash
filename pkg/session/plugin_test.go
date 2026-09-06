package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

type pluginSessionConfig struct{}

func (pluginSessionConfig) GetUsername() string { return "" }
func (pluginSessionConfig) GetAPIKey() string   { return "" }
func (pluginSessionConfig) GetSessionStoreKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}
func (pluginSessionConfig) GetMaxSessionAge() int                   { return 3600 }
func (pluginSessionConfig) ValidateCredentials(string, string) bool { return false }

func TestPluginCookieIsExplicitlyMarkedAndCarriesOnlyCallerIdentity(t *testing.T) {
	store := NewStore(pluginSessionConfig{})
	ctx := SetCurrentUserID(context.Background(), "42")
	cookie := store.MakePluginCookie(ctx)
	if cookie == nil {
		t.Fatal("MakePluginCookie returned nil")
	}
	request := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	request.AddCookie(cookie)
	if !store.IsPluginRequest(request) {
		t.Fatal("plugin cookie was not explicitly marked")
	}
	userID, err := store.Authenticate(httptest.NewRecorder(), request)
	if err != nil || userID != "42" {
		t.Fatalf("plugin cookie identity = %q err=%v", userID, err)
	}

	ordinary := sessions.NewSession(store.sessionStore, cookieName)
	ordinary.Values[userIDKey] = "42"
	encoded, err := securecookie.EncodeMulti(ordinary.Name(), ordinary.Values, store.sessionStore.Codecs...)
	if err != nil {
		t.Fatal(err)
	}
	unmarked := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	unmarked.AddCookie(sessions.NewCookie(ordinary.Name(), encoded, ordinary.Options))
	if store.IsPluginRequest(unmarked) {
		t.Fatal("ordinary legacy cookie was accepted as a plugin request")
	}

	missingIdentity := sessions.NewSession(store.sessionStore, cookieName)
	missingIdentity.Values[pluginRequestKey] = true
	encoded, err = securecookie.EncodeMulti(missingIdentity.Name(), missingIdentity.Values, store.sessionStore.Codecs...)
	if err != nil {
		t.Fatal(err)
	}
	noIdentity := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	noIdentity.AddCookie(sessions.NewCookie(missingIdentity.Name(), encoded, missingIdentity.Options))
	if store.IsPluginRequest(noIdentity) {
		t.Fatal("signed plugin marker without caller identity was accepted")
	}

	tampered := *cookie
	tampered.Value += "tampered"
	tamperedRequest := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	tamperedRequest.AddCookie(&tampered)
	if store.IsPluginRequest(tamperedRequest) {
		t.Fatal("tampered plugin marker was accepted")
	}
}

package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

func independentlyValidPluginCookie(store *Store, request *http.Request) bool {
	decoded, err := store.sessionStore.Get(request, cookieName)
	if err != nil {
		return false
	}
	marked, markerOK := decoded.Values[pluginRequestKey].(bool)
	userID, identityOK := decoded.Values[userIDKey].(string)
	return markerOK && marked && identityOK && userID != ""
}

func signedFuzzCookieValue(f *testing.F, store *Store, values map[interface{}]interface{}) string {
	f.Helper()
	session := sessions.NewSession(store.sessionStore, cookieName)
	session.Values = values
	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values, store.sessionStore.Codecs...)
	if err != nil {
		f.Fatal(err)
	}
	return encoded
}

func FuzzPluginCookieMarkerFailsClosed(f *testing.F) {
	store := NewStore(pluginSessionConfig{})
	valid42 := store.MakePluginCookie(SetCurrentUserID(context.Background(), "42"))
	valid84 := store.MakePluginCookie(SetCurrentUserID(context.Background(), "84"))
	if valid42 == nil || valid84 == nil {
		f.Fatal("MakePluginCookie returned nil")
	}
	signedUnmarked := signedFuzzCookieValue(f, store, map[interface{}]interface{}{
		userIDKey: "42",
	})
	signedMissingIdentity := signedFuzzCookieValue(f, store, map[interface{}]interface{}{
		pluginRequestKey: true,
	})
	signedFalseMarker := signedFuzzCookieValue(f, store, map[interface{}]interface{}{
		userIDKey:        "42",
		pluginRequestKey: false,
	})
	for _, seed := range []string{
		"",
		valid42.Value,
		valid84.Value,
		signedUnmarked,
		signedMissingIdentity,
		signedFalseMarker,
		valid42.Value + "tampered",
		valid42.Value + "\x1f\x1f\x1f\x1f\x1f",
		valid42.Value[:len(valid42.Value)/2],
		string(make([]byte, 64<<10)),
		"plugin_request=true",
		"\x00\xffrandom",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		request := httptest.NewRequest(http.MethodPost, "/graphql", nil)
		request.AddCookie(&http.Cookie{Name: cookieName, Value: value})
		received, err := request.Cookie(cookieName)
		if err != nil {
			if store.IsPluginRequest(request) {
				t.Fatal("request without a parseable cookie was classified as a plugin request")
			}
			return
		}
		marked := store.IsPluginRequest(request)
		authenticPlugin := independentlyValidPluginCookie(store, request)
		if marked != authenticPlugin {
			t.Fatalf(
				"plugin classification=%v independent signed-payload verification=%v for received cookie length %d",
				marked,
				authenticPlugin,
				len(received.Value),
			)
		}
	})
}

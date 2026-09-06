package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/authservice"
	"github.com/stashapp/stash/pkg/sqlite"
)

const (
	dbSessionCookie = "stash_session_v2"
	dbCSRFCookie    = "stash_csrf_v2"
)

func setDBSessionCookies(w http.ResponseWriter, r *http.Request, credentials *sqlite.SessionCredentials) {
	secure := r.TLS != nil
	path := getProxyPrefix(r)
	if path == "" {
		path = "/"
	}
	maxAge := int(authservice.DefaultSessionAbsolute.Seconds())
	http.SetCookie(w, &http.Cookie{Name: dbSessionCookie, Value: credentials.ID + "." + credentials.Secret, Path: path, MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: dbCSRFCookie, Value: credentials.CSRFSecret, Path: path, MaxAge: maxAge, HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode})
}

func readDBSessionCookie(r *http.Request) (id, secret string, ok bool) {
	cookie, err := r.Cookie(dbSessionCookie)
	if err != nil {
		return "", "", false
	}
	id, secret, ok = strings.Cut(cookie.Value, ".")
	return id, secret, ok && id != "" && secret != ""
}

func clearDBSessionCookies(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil
	path := getProxyPrefix(r)
	if path == "" {
		path = "/"
	}
	for _, cookie := range []*http.Cookie{
		{Name: dbSessionCookie, Path: path, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode},
		{Name: dbCSRFCookie, Path: path, HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode},
	} {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
		http.SetCookie(w, cookie)
	}
}

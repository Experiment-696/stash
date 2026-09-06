package api

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/authservice"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stashapp/stash/pkg/utils"
	"github.com/stashapp/stash/ui"
)

const (
	returnURLParam = "returnURL"

	defaultLocale          = "en-GB"
	defaultLoginLocaleJSON = `{"login":"Login","username":"Username","password":"Password","invalid_credentials":"Invalid username or password","internal_error":"Unexpected internal error. See logs for more details"}`
)

func getLoginPage() []byte {
	data, err := fs.ReadFile(ui.LoginUIBox, "login.html")
	if err != nil {
		panic(err)
	}
	return data
}

type loginTemplateData struct {
	URL   string
	Error string
}

func serveLoginPage(w http.ResponseWriter, r *http.Request, returnURL string, loginError string) {
	loginPage := string(getLoginPage())
	prefix := getProxyPrefix(r)
	loginPage = strings.ReplaceAll(loginPage, "/%BASE_URL%", prefix)

	templ, err := template.New("Login").Parse(loginPage)
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %s", err), http.StatusInternalServerError)
		return
	}

	buffer := bytes.Buffer{}
	err = templ.Execute(&buffer, loginTemplateData{URL: returnURL, Error: loginError})
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %s", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// we shouldn't need to set plugin exceptions here
	setPageSecurityHeaders(w, r, nil)

	utils.ServeStaticContent(w, r, buffer.Bytes())
}

func handleLoginLocale(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// get the locale from the config
		lang := cfg.GetLanguage()
		if lang == "" {
			lang = defaultLocale
		}

		data, err := getLoginLocale(lang)
		if err != nil {
			logger.Debugf("Failed to load login locale file for language %s: %v", lang, err)
			// try again with the default language
			if lang != defaultLocale {
				data, err = getLoginLocale(defaultLocale)
				if err != nil {
					logger.Errorf("Failed to load login locale file for default language %s: %v", defaultLocale, err)
				}
			}

			// if there's still an error, response with an internal server error
			if err != nil {
				// Keep the credential form functional even if a release was built
				// without generated locale assets.
				data = []byte(defaultLoginLocaleJSON)
			}
		}

		// write a script to set the locale string map as a global variable
		localeScript := fmt.Sprintf("var localeStrings = %s;", data)
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(localeScript))
	}
}

func getLoginLocale(lang string) ([]byte, error) {
	data, err := fs.ReadFile(ui.LoginUIBox, "locales/"+lang+".json")
	if err != nil {
		return nil, err
	}
	return data, nil
}

func handleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		returnURL := r.URL.Query().Get(returnURLParam)
		loginMessage := ""
		if r.URL.Query().Get("sessionExpired") == "1" {
			loginMessage = "Your session expired. Sign in to return to your previous page; unsaved Cam Show rule drafts are preserved in this browser."
		}

		mgr := manager.GetInstance()
		databaseUsers := 0
		if err := mgr.Database.Ready(); err != nil {
			// Existing pre-migration installations must retain the legacy login
			// path so an administrator can reach and perform the migration.
			if config.GetInstance().HasCredentials() {
				serveLoginPage(w, r, returnURL, loginMessage)
				return
			}
			http.Error(w, "Database migration is required", http.StatusServiceUnavailable)
			return
		} else if err := txn.WithReadTxn(r.Context(), mgr.Database, func(ctx context.Context) error {
			var err error
			databaseUsers, err = mgr.Database.User.Count(ctx)
			return err
		}); err != nil {
			http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
			return
		}
		if databaseUsers == 0 && !config.GetInstance().HasCredentials() {
			if returnURL != "" {
				http.Redirect(w, r, returnURL, http.StatusFound)
			} else {
				prefix := getProxyPrefix(r)
				http.Redirect(w, r, prefix+"/", http.StatusFound)
			}
			return
		}

		serveLoginPage(w, r, returnURL, loginMessage)
	}
}

func handleLoginPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mgr := manager.GetInstance()
		databaseUsers := 0
		if err := mgr.Database.Ready(); err != nil {
			if config.GetInstance().HasCredentials() {
				err := mgr.SessionStore.Login(w, r)
				if err == nil {
					w.WriteHeader(http.StatusOK)
					return
				}
				var invalidCredentialsError *session.InvalidCredentialsError
				if errors.As(err, &invalidCredentialsError) {
					http.Error(w, "Username or password is invalid", http.StatusUnauthorized)
					return
				}
				logger.Errorf("Error logging in during database migration: %v from IP: %s", err, r.RemoteAddr)
				http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
				return
			}
			http.Error(w, "Database migration is required", http.StatusServiceUnavailable)
			return
		} else if err := txn.WithReadTxn(r.Context(), mgr.Database, func(ctx context.Context) error {
			var err error
			databaseUsers, err = mgr.Database.User.Count(ctx)
			return err
		}); err != nil {
			logger.Errorf("Error checking database login state: %v", err)
			http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
			return
		}
		if databaseUsers > 0 {
			_, credentials, err := (authservice.LoginService{Database: mgr.Database}).Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
			if errors.Is(err, sqlite.ErrInvalidCredentials) {
				http.Error(w, "Username or password is invalid", http.StatusUnauthorized)
				return
			}
			if err != nil {
				logger.Errorf("Error logging in with database account: %v from IP: %s", err, r.RemoteAddr)
				http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
				return
			}
			setDBSessionCookies(w, r, credentials)
			w.WriteHeader(http.StatusOK)
			return
		}
		err := manager.GetInstance().SessionStore.Login(w, r)
		if err != nil {
			// always log the error
			logger.Errorf("Error logging in: %v from IP: %s", err, r.RemoteAddr)
		}

		var invalidCredentialsError *session.InvalidCredentialsError

		if errors.As(err, &invalidCredentialsError) {
			http.Error(w, "Username or password is invalid", http.StatusUnauthorized)
			return
		}

		if err != nil {
			// don't expose the error to the user
			http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mgr := manager.GetInstance()
		if sessionID, sessionSecret, ok := readDBSessionCookie(r); ok {
			clearDBSessionCookies(w, r)
			retryer := txn.Retryer{Manager: mgr.Database, Retries: 5}
			err := retryer.WithTxn(r.Context(), func(ctx context.Context) error {
				sessionRecord, _, err := mgr.Database.Session.AuthenticatePrincipal(ctx, sessionID, sessionSecret, time.Now())
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return nil // Invalid/expired cookie is still cleared below.
					}
					return err
				}
				return mgr.Database.Session.Revoke(ctx, sessionRecord.UserID, sessionRecord.ID)
			})
			if err != nil {
				http.Error(w, "An unexpected error occurred. See logs", http.StatusInternalServerError)
				return
			}
		} else if err := mgr.SessionStore.Logout(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// redirect to the login page if credentials are required
		prefix := getProxyPrefix(r)
		if config.GetInstance().HasCredentials() {
			http.Redirect(w, r, prefix+loginEndpoint, http.StatusFound)
		} else {
			http.Redirect(w, r, prefix+"/", http.StatusFound)
		}
	}
}

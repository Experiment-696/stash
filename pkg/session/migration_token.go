package session

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	MigrationTokenHeader = "X-Stash-Migration-Token"
	MigrationTokenQuery  = "migration_token"
	MigrationCookie      = "stash_migration"
	MigrationCSRFHeader  = "X-Stash-Migration-CSRF"
	MigrationTokenFile   = ".migration-bootstrap-token"
	migrationTokenTTL    = 30 * time.Minute
)

type migrationRequestContextKey struct{}

type migrationCredentialState struct {
	sync.Mutex
	exchangeToken string
	sessionToken  string
	csrfToken     string
	expiresAt     time.Time
	handoffPath   string
	correlationID string
	inFlight      bool
}

var migrationCredential migrationCredentialState

func randomHexToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func secureHandoffPath(configDir string) (string, error) {
	abs, err := filepath.Abs(configDir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	if filepath.Clean(resolved) != filepath.Clean(abs) {
		return "", errors.New("migration token directory must not be a symlink")
	}
	return filepath.Join(abs, MigrationTokenFile), nil
}

// EnsureMigrationToken creates a purpose-bound, process-local exchange token
// and discloses it only through a protected handoff file.
func EnsureMigrationToken(now time.Time, configDir string) (path string, expiresAt time.Time, err error) {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()

	if now.Before(migrationCredential.expiresAt) &&
		(migrationCredential.exchangeToken != "" || migrationCredential.sessionToken != "") {
		return migrationCredential.handoffPath, migrationCredential.expiresAt, nil
	}
	cleanupMigrationCredentialLocked()

	handoffPath, err := secureHandoffPath(configDir)
	if err != nil {
		return "", time.Time{}, err
	}
	if info, statErr := os.Lstat(handoffPath); statErr == nil {
		return "", time.Time{}, fmt.Errorf("migration token handoff already exists (%s)", info.Mode().Type())
	} else if !os.IsNotExist(statErr) {
		return "", time.Time{}, statErr
	}
	token, err := randomHexToken()
	if err != nil {
		return "", time.Time{}, err
	}
	correlationID, err := randomHexToken()
	if err != nil {
		return "", time.Time{}, err
	}
	f, err := os.OpenFile(handoffPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", time.Time{}, err
	}
	writeErr := error(nil)
	if _, err = f.WriteString(token + "\n"); err != nil {
		writeErr = err
	}
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(handoffPath)
		return "", time.Time{}, writeErr
	}
	info, statErr := os.Lstat(handoffPath)
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		_ = os.Remove(handoffPath)
		return "", time.Time{}, errors.New("migration token handoff is not a regular mode-0600 file")
	}
	migrationCredential.exchangeToken = token
	migrationCredential.correlationID = correlationID[:16]
	migrationCredential.expiresAt = now.Add(migrationTokenTTL)
	migrationCredential.handoffPath = handoffPath
	return handoffPath, migrationCredential.expiresAt, nil
}

func constantTimeTokenEqual(candidate, expected string) bool {
	return expected != "" && len(candidate) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1
}

// SetMigrationRequest validates either the migration session cookie or a
// single-use exchange bearer. Exchange is atomic and returns the new session
// and CSRF values to the HTTP middleware for delivery.
func SetMigrationRequest(r *http.Request, now time.Time) (request *http.Request, sessionToken, csrfToken string, exchanged bool) {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()

	if !now.Before(migrationCredential.expiresAt) {
		cleanupMigrationCredentialLocked()
		return r, "", "", false
	}
	if cookie, err := r.Cookie(MigrationCookie); err == nil &&
		constantTimeTokenEqual(cookie.Value, migrationCredential.sessionToken) {
		ctx := context.WithValue(r.Context(), migrationRequestContextKey{}, true)
		return r.WithContext(ctx), "", "", false
	}

	candidate := r.Header.Get(MigrationTokenHeader)
	if candidate == "" {
		candidate = r.URL.Query().Get(MigrationTokenQuery)
	}
	if !constantTimeTokenEqual(candidate, migrationCredential.exchangeToken) {
		return r, "", "", false
	}
	sessionToken, err := randomHexToken()
	if err != nil {
		return r, "", "", false
	}
	csrfToken, err = randomHexToken()
	if err != nil {
		return r, "", "", false
	}
	if migrationCredential.handoffPath != "" {
		if err := os.Remove(migrationCredential.handoffPath); err != nil {
			return r, "", "", false
		}
	}
	migrationCredential.exchangeToken = ""
	migrationCredential.sessionToken = sessionToken
	migrationCredential.csrfToken = csrfToken
	migrationCredential.handoffPath = ""
	ctx := context.WithValue(r.Context(), migrationRequestContextKey{}, true)
	return r.WithContext(ctx), sessionToken, csrfToken, true
}

func SetMigrationCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{Name: MigrationCookie, Value: token, Path: "/", Expires: expiresAt,
		MaxAge: int(migrationTokenTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func ClearMigrationCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: MigrationCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func IsMigrationRequest(ctx context.Context) bool {
	value, _ := ctx.Value(migrationRequestContextKey{}).(bool)
	return value
}

func VerifyMigrationCSRF(candidate string) bool {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()
	return time.Now().Before(migrationCredential.expiresAt) && constantTimeTokenEqual(candidate, migrationCredential.csrfToken)
}

// MigrationCSRF returns the nonce only to a request already authenticated by
// the migration session. It is intended for one-time injection into the
// static migration shell, never for logs, cookies, or persistent storage.
func MigrationCSRF(ctx context.Context) string {
	if !IsMigrationRequest(ctx) {
		return ""
	}
	migrationCredential.Lock()
	defer migrationCredential.Unlock()
	if !time.Now().Before(migrationCredential.expiresAt) {
		return ""
	}
	return migrationCredential.csrfToken
}

func MigrationTokenExpiry() time.Time {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()
	return migrationCredential.expiresAt
}

func MigrationCorrelationID() string {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()
	return migrationCredential.correlationID
}

func AcquireMigrationLease() bool {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()
	if migrationCredential.sessionToken == "" || migrationCredential.inFlight ||
		!time.Now().Before(migrationCredential.expiresAt) {
		return false
	}
	migrationCredential.inFlight = true
	return true
}

func ReleaseMigrationLease() {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()
	migrationCredential.inFlight = false
}

func cleanupMigrationCredentialLocked() {
	if migrationCredential.handoffPath != "" {
		_ = os.Remove(migrationCredential.handoffPath)
	}
	migrationCredential.exchangeToken = ""
	migrationCredential.sessionToken = ""
	migrationCredential.csrfToken = ""
	migrationCredential.expiresAt = time.Time{}
	migrationCredential.handoffPath = ""
	migrationCredential.correlationID = ""
	migrationCredential.inFlight = false
}

func ConsumeMigrationToken() {
	migrationCredential.Lock()
	defer migrationCredential.Unlock()
	cleanupMigrationCredentialLocked()
}

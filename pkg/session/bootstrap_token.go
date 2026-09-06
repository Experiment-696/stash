package session

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/stashapp/stash/pkg/logger"
)

const (
	BootstrapTokenHeader = "X-Stash-Bootstrap-Token"
	BootstrapTokenQuery  = "bootstrap_token"
	BootstrapTokenCookie = "stash_bootstrap"
	bootstrapTokenTTL    = 30 * time.Minute
)

type bootstrapRequestContextKey struct{}

var bootstrapCredential = struct {
	sync.Mutex
	value     string
	expiresAt time.Time
}{}

// EnsureBootstrapToken returns the active one-time bootstrap token, creating
// without logging it. It is intended only for the zero-user
// migration/bootstrap window.
func EnsureBootstrapToken(now time.Time) (string, error) {
	bootstrapCredential.Lock()
	defer bootstrapCredential.Unlock()

	if bootstrapCredential.value != "" && now.Before(bootstrapCredential.expiresAt) {
		return bootstrapCredential.value, nil
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	bootstrapCredential.value = hex.EncodeToString(raw)
	bootstrapCredential.expiresAt = now.Add(bootstrapTokenTTL)
	logger.Infof(
		"One-time setup token created; expires %s",
		bootstrapCredential.expiresAt.Format(time.RFC3339),
	)
	return bootstrapCredential.value, nil
}

// SetBootstrapRequest marks a request only when its dedicated header matches
// the current unexpired token. Forwarding headers and source address ranges are
// deliberately ignored.
func SetBootstrapRequest(r *http.Request, now time.Time) *http.Request {
	candidate := r.Header.Get(BootstrapTokenHeader)
	if candidate == "" {
		if cookie, err := r.Cookie(BootstrapTokenCookie); err == nil {
			candidate = cookie.Value
		}
	}
	if candidate == "" {
		candidate = r.URL.Query().Get(BootstrapTokenQuery)
	}
	if candidate == "" {
		return r
	}

	bootstrapCredential.Lock()
	defer bootstrapCredential.Unlock()
	if bootstrapCredential.value == "" || !now.Before(bootstrapCredential.expiresAt) ||
		len(candidate) != len(bootstrapCredential.value) ||
		subtle.ConstantTimeCompare([]byte(candidate), []byte(bootstrapCredential.value)) != 1 {
		return r
	}

	ctx := context.WithValue(r.Context(), bootstrapRequestContextKey{}, true)
	return r.WithContext(ctx)
}

func SetBootstrapCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     BootstrapTokenCookie,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(bootstrapTokenTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func BootstrapTokenExpiry() time.Time {
	bootstrapCredential.Lock()
	defer bootstrapCredential.Unlock()
	return bootstrapCredential.expiresAt
}

func IsBootstrapRequest(ctx context.Context) bool {
	value, _ := ctx.Value(bootstrapRequestContextKey{}).(bool)
	return value
}

// ConsumeBootstrapToken closes the token window after the first Admin commits.
func ConsumeBootstrapToken() {
	bootstrapCredential.Lock()
	defer bootstrapCredential.Unlock()
	bootstrapCredential.value = ""
	bootstrapCredential.expiresAt = time.Time{}
}

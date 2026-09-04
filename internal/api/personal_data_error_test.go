package api

import (
	"errors"
	"strings"
	"testing"
)

func TestPersonalDataErrorDoesNotLeakDatabaseDetails(t *testing.T) {
	serverErr := errors.New("SELECT secret_hash FROM users: no such table: user_performer_state")
	clientErr := personalDataError("test", serverErr)
	for _, forbidden := range []string{"SELECT", "secret_hash", "users", "user_performer_state", "no such table"} {
		if strings.Contains(clientErr.Error(), forbidden) {
			t.Fatalf("client error leaked %q: %q", forbidden, clientErr)
		}
	}
	if clientErr.Error() != "personal data is temporarily unavailable" {
		t.Fatalf("unexpected client error: %q", clientErr)
	}
}

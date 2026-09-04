package api

import (
	"encoding/json"
	"testing"
)

func TestDefaultLoginLocaleFallbackIsValid(t *testing.T) {
	var locale map[string]string
	if err := json.Unmarshal([]byte(defaultLoginLocaleJSON), &locale); err != nil {
		t.Fatalf("default login locale is not valid JSON: %v", err)
	}

	for _, key := range []string{"login", "username", "password", "invalid_credentials", "internal_error"} {
		if locale[key] == "" {
			t.Fatalf("default login locale is missing %q", key)
		}
	}
}

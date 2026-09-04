package sqlite

import (
	"context"
	"errors"
	"testing"
)

func TestBeginFailsSafelyWhenDatabaseIsUnopened(t *testing.T) {
	database := NewDatabase()
	for _, writable := range []bool{false, true} {
		_, err := database.Begin(context.Background(), writable)
		var unavailable *DatabaseUnavailableError
		if !errors.As(err, &unavailable) || !errors.Is(err, ErrDatabaseNotInitialized) {
			t.Fatalf("writable=%v error=%v, want typed unavailable wrapping not-initialized", writable, err)
		}
	}
}

func TestWithDatabaseFailsSafelyWhenDatabaseIsUnopened(t *testing.T) {
	database := NewDatabase()
	_, err := database.WithDatabase(context.Background())
	var unavailable *DatabaseUnavailableError
	if !errors.As(err, &unavailable) || !errors.Is(err, ErrDatabaseNotInitialized) {
		t.Fatalf("error=%v, want typed unavailable wrapping not-initialized", err)
	}
}

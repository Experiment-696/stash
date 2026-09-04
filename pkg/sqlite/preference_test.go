package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
)

func TestPreferenceStoreTwoUserIsolationUpdateClearAndCascade(t *testing.T) {
	config.InitializeEmpty()
	database := NewDatabase()
	if err := database.Open(filepath.Join(t.TempDir(), "preferences.sqlite")); err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	tx, err := database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := database.User.Create(tx, "preference-one", "password-one", authz.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.User.Create(tx, "preference-two", "password-two", authz.RoleModerator)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Preference.Set(tx, first.ID, PreferenceHomepageRoute, "/scenes"); err != nil {
		t.Fatal(err)
	}
	if err := database.Preference.Set(tx, second.ID, PreferenceHomepageRoute, "/performers"); err != nil {
		t.Fatal(err)
	}
	if err := database.Preference.Set(tx, first.ID, PreferenceThemeID, "theme-dark"); err != nil {
		t.Fatal(err)
	}
	if err := database.Commit(tx); err != nil {
		t.Fatal(err)
	}

	read, err := database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	firstHome, err := database.Preference.Get(read, first.ID, PreferenceHomepageRoute)
	if err != nil {
		t.Fatal(err)
	}
	secondHome, err := database.Preference.Get(read, second.ID, PreferenceHomepageRoute)
	if err != nil {
		t.Fatal(err)
	}
	secondTheme, err := database.Preference.Get(read, second.ID, PreferenceThemeID)
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Rollback(read)
	if firstHome == nil || *firstHome != "/scenes" ||
		secondHome == nil || *secondHome != "/performers" ||
		secondTheme != nil {
		t.Fatalf("isolated preferences first=%v second=%v secondTheme=%v", firstHome, secondHome, secondTheme)
	}

	tx, err = database.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Preference.Set(tx, first.ID, PreferenceHomepageRoute, "/images"); err != nil {
		t.Fatal(err)
	}
	if err := database.Preference.Clear(tx, first.ID, PreferenceThemeID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbWrapper.Exec(tx, `DELETE FROM users WHERE id = ?`, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Commit(tx); err != nil {
		t.Fatal(err)
	}

	read, err = database.Begin(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(read)
	var count int
	if err := dbWrapper.Get(read, &count, `SELECT count(*) FROM user_preferences WHERE user_id = ?`, first.ID); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("deleted user retained %d preference rows", count)
	}
	secondHome, err = database.Preference.Get(read, second.ID, PreferenceHomepageRoute)
	if err != nil || secondHome == nil || *secondHome != "/performers" {
		t.Fatalf("other user's preference changed: value=%v err=%v", secondHome, err)
	}
}

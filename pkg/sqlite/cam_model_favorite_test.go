package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
)

func TestCamModelFavoriteUserIsolationOrderingAndRestart(t *testing.T) {
	config.InitializeEmpty()
	path := filepath.Join(t.TempDir(), "favorites.sqlite")
	db := NewDatabase()
	if err := db.Open(path); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := db.User.Create(tx, "favorite-one", "password-one", authz.RoleUser)
	second, _ := db.User.Create(tx, "favorite-two", "password-two", authz.RoleUser)
	zulu, err := db.CamShow.CreateModel(tx, "Zulu", nil)
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := db.CamShow.CreateModel(tx, "alpha", nil)
	if err != nil {
		t.Fatal(err)
	}
	rating := 88
	if err := db.CamShow.SetUserState(tx, first.ID, zulu.ID, true, &rating); err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.SetUserState(tx, first.ID, alpha.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CamShow.SetUserState(tx, first.ID, alpha.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	favorites, err := db.CamShow.ListModelProfilesForUser(tx, first.ID, true)
	if err != nil || len(favorites) != 2 || favorites[0].Model.ID != alpha.ID || favorites[1].Model.ID != zulu.ID {
		t.Fatalf("favorites=%+v err=%v", favorites, err)
	}
	other, err := db.CamShow.ListModelProfilesForUser(tx, second.ID, true)
	if err != nil || len(other) != 0 {
		t.Fatalf("other=%+v err=%v", other, err)
	}
	if err := db.Commit(tx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := NewDatabase()
	if err := restarted.Open(path); err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	read, err := restarted.WithDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.CamShow.FindModelProfileForUser(read, zulu.ID, first.ID)
	if err != nil || got == nil || got.UserState == nil || !got.UserState.Favorite || got.UserState.Rating == nil || *got.UserState.Rating != rating {
		t.Fatalf("state=%+v err=%v", got, err)
	}
}

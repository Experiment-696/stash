package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type CamSite struct {
	ID          int64     `db:"id"`
	Name        string    `db:"name"`
	BaseURL     *string   `db:"base_url"`
	ExternalKey *string   `db:"external_key"`
	Enabled     bool      `db:"enabled"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
type CamModel struct {
	ID          int64     `db:"id"`
	DisplayName string    `db:"display_name"`
	Image       *string   `db:"image"`
	Notes       *string   `db:"notes"`
	PerformerID *int64    `db:"performer_id"`
	Status      string    `db:"status"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
type CamShow struct {
	ID         int64     `db:"id"`
	SceneID    int64     `db:"scene_id"`
	Category   string    `db:"category"`
	SiteID     *int64    `db:"site_id"`
	ExternalID *string   `db:"external_id"`
	SyncState  string    `db:"sync_state"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type CamShowLibraryTag struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

type CamShowLibraryItem struct {
	ID       int64               `db:"id"`
	SceneID  int64               `db:"scene_id"`
	Title    string              `db:"title"`
	Category string              `db:"category"`
	Tags     []CamShowLibraryTag `db:"-"`
}

type CamShowStore struct{}

type CamModelAccount struct {
	ID               int64      `db:"id"`
	ModelID          int64      `db:"model_id"`
	SiteID           int64      `db:"site_id"`
	Handle           string     `db:"handle"`
	NormalizedHandle string     `db:"normalized_handle"`
	Status           string     `db:"status"`
	ValidFrom        *time.Time `db:"valid_from"`
	ValidTo          *time.Time `db:"valid_to"`
}

type CamModelAlias struct {
	ID              int64      `db:"id"`
	ModelID         int64      `db:"model_id"`
	AccountID       *int64     `db:"account_id"`
	SiteID          *int64     `db:"site_id"`
	Alias           string     `db:"alias"`
	NormalizedAlias string     `db:"normalized_alias"`
	ValidTo         *time.Time `db:"valid_to"`
	IsCurrent       bool       `db:"is_current"`
}

type CamModelUserState struct {
	UserID    int64     `db:"user_id"`
	ModelID   int64     `db:"model_id"`
	Favorite  bool      `db:"favorite"`
	Rating    *int      `db:"rating"`
	UpdatedAt time.Time `db:"updated_at"`
}

type CamIdentityExport struct {
	Accounts []CamModelAccount   `json:"accounts"`
	Aliases  []CamModelAlias     `json:"aliases"`
	States   []CamModelUserState `json:"user_states"`
}

func normalizeCamIdentity(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}

func (s *CamShowStore) CreateSite(ctx context.Context, name string, baseURL, externalKey *string) (*CamSite, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("site name is required")
	}
	now := time.Now().UTC()
	result, err := dbWrapper.Exec(ctx, `INSERT INTO cam_sites(name,base_url,external_key,enabled,created_at,updated_at) VALUES(?,?,?,1,?,?)`, name, baseURL, externalKey, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.FindSite(ctx, id)
}
func (s *CamShowStore) FindSite(ctx context.Context, id int64) (*CamSite, error) {
	var v CamSite
	err := dbWrapper.Get(ctx, &v, `SELECT id,name,base_url,external_key,enabled,created_at,updated_at FROM cam_sites WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
func (s *CamShowStore) CreateModel(ctx context.Context, name string, performerID *int64) (*CamModel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("model display name is required")
	}
	now := time.Now().UTC()
	r, err := dbWrapper.Exec(ctx, `INSERT INTO cam_models(display_name,status,performer_id,created_at,updated_at) VALUES(?,'ACTIVE',?,?,?)`, name, performerID, now, now)
	if err != nil {
		return nil, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.FindModel(ctx, id)
}
func (s *CamShowStore) FindModel(ctx context.Context, id int64) (*CamModel, error) {
	var v CamModel
	err := dbWrapper.Get(ctx, &v, `SELECT id,display_name,image,notes,performer_id,status,created_at,updated_at FROM cam_models WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
func (s *CamShowStore) CreateShow(ctx context.Context, sceneID int64, category string, siteID *int64) (*CamShow, error) {
	category = strings.TrimSpace(category)
	if sceneID <= 0 || category == "" {
		return nil, errors.New("scene and category are required")
	}
	now := time.Now().UTC()
	_, err := dbWrapper.Exec(ctx, `INSERT INTO cam_shows(scene_id,category,site_id,sync_state,created_at,updated_at) VALUES(?,?,?,'LOCAL',?,?)`, sceneID, category, siteID, now, now)
	if err != nil {
		return nil, err
	}
	return s.FindShowByScene(ctx, sceneID)
}
func (s *CamShowStore) FindShowByScene(ctx context.Context, sceneID int64) (*CamShow, error) {
	var v CamShow
	err := dbWrapper.Get(ctx, &v, `SELECT id,scene_id,category,site_id,external_id,sync_state,created_at,updated_at FROM cam_shows WHERE scene_id=?`, sceneID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
func (s *CamShowStore) LinkModel(ctx context.Context, showID, modelID int64, billingOrder int, role *string) error {
	_, err := dbWrapper.Exec(ctx, `INSERT INTO cam_show_models(show_id,model_id,billing_order,participation_role) VALUES(?,?,?,?)`, showID, modelID, billingOrder, role)
	return err
}

func (s *CamShowStore) CreateAccount(ctx context.Context, modelID, siteID int64, handle string, validFrom *time.Time) (*CamModelAccount, error) {
	handle = strings.TrimSpace(handle)
	normalized := normalizeCamIdentity(handle)
	if modelID <= 0 || siteID <= 0 || normalized == "" {
		return nil, errors.New("model, site, and handle are required")
	}
	now := time.Now().UTC()
	result, err := dbWrapper.Exec(ctx, `INSERT INTO cam_model_accounts
		(model_id,site_id,handle,normalized_handle,status,valid_from,source,created_at,updated_at)
		VALUES(?,?,?,?,'ACTIVE',?,'MANUAL',?,?)`, modelID, siteID, handle, normalized, validFrom, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.FindAccount(ctx, id)
}

func (s *CamShowStore) FindAccount(ctx context.Context, id int64) (*CamModelAccount, error) {
	var value CamModelAccount
	err := dbWrapper.Get(ctx, &value, `SELECT id,model_id,site_id,handle,normalized_handle,status,valid_from,valid_to FROM cam_model_accounts WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func (s *CamShowStore) CloseAccount(ctx context.Context, id int64, validTo time.Time) error {
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_model_accounts SET status='INACTIVE',valid_to=?,updated_at=? WHERE id=? AND valid_to IS NULL`, validTo.UTC(), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *CamShowStore) CreateAlias(ctx context.Context, modelID int64, accountID, siteID *int64, alias string) (*CamModelAlias, error) {
	alias = strings.TrimSpace(alias)
	normalized := normalizeCamIdentity(alias)
	if modelID <= 0 || normalized == "" {
		return nil, errors.New("model and alias are required")
	}
	now := time.Now().UTC()
	result, err := dbWrapper.Exec(ctx, `INSERT INTO cam_model_aliases
		(model_id,account_id,site_id,alias,normalized_alias,is_current,source,created_at,updated_at)
		VALUES(?,?,?,?,?,1,'MANUAL',?,?)`, modelID, accountID, siteID, alias, normalized, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.FindAlias(ctx, id)
}

func (s *CamShowStore) FindAlias(ctx context.Context, id int64) (*CamModelAlias, error) {
	var value CamModelAlias
	err := dbWrapper.Get(ctx, &value, `SELECT id,model_id,account_id,site_id,alias,normalized_alias,valid_to,is_current FROM cam_model_aliases WHERE id=?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func (s *CamShowStore) RetireAlias(ctx context.Context, id int64, validTo time.Time) error {
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_model_aliases SET is_current=0,valid_to=?,updated_at=? WHERE id=? AND is_current=1`, validTo.UTC(), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *CamShowStore) SetUserState(ctx context.Context, userID, modelID int64, favorite bool, rating *int) error {
	if userID <= 0 || modelID <= 0 || (rating != nil && (*rating < 1 || *rating > 100)) {
		return errors.New("invalid model user state")
	}
	_, err := dbWrapper.Exec(ctx, `INSERT INTO cam_model_user_state(user_id,model_id,favorite,rating,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(user_id,model_id) DO UPDATE SET favorite=excluded.favorite,rating=excluded.rating,updated_at=excluded.updated_at`, userID, modelID, favorite, rating, time.Now().UTC())
	return err
}

func (s *CamShowStore) GetUserState(ctx context.Context, userID, modelID int64) (*CamModelUserState, error) {
	var value CamModelUserState
	err := dbWrapper.Get(ctx, &value, `SELECT user_id,model_id,favorite,rating,updated_at FROM cam_model_user_state WHERE user_id=? AND model_id=?`, userID, modelID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func (s *CamShowStore) ExportIdentityState(ctx context.Context) (*CamIdentityExport, error) {
	ret := &CamIdentityExport{}
	if err := dbWrapper.Select(ctx, &ret.Accounts, `SELECT id,model_id,site_id,handle,normalized_handle,status,valid_from,valid_to FROM cam_model_accounts ORDER BY id`); err != nil {
		return nil, err
	}
	if err := dbWrapper.Select(ctx, &ret.Aliases, `SELECT id,model_id,account_id,site_id,alias,normalized_alias,valid_to,is_current FROM cam_model_aliases ORDER BY id`); err != nil {
		return nil, err
	}
	if err := dbWrapper.Select(ctx, &ret.States, `SELECT user_id,model_id,favorite,rating,updated_at FROM cam_model_user_state ORDER BY user_id,model_id`); err != nil {
		return nil, err
	}
	return ret, nil
}

func (s *CamShowStore) ImportIdentityState(ctx context.Context, input CamIdentityExport) (err error) {
	if _, err = dbWrapper.Exec(ctx, "SAVEPOINT cam_identity_import"); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if _, rollbackErr := dbWrapper.Exec(ctx, "ROLLBACK TO cam_identity_import"); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("rolling back identity import: %w", rollbackErr))
			}
		}
		if _, releaseErr := dbWrapper.Exec(ctx, "RELEASE cam_identity_import"); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("releasing identity import savepoint: %w", releaseErr))
		}
	}()
	for _, value := range input.Accounts {
		if value.ID <= 0 || value.ModelID <= 0 || value.SiteID <= 0 || normalizeCamIdentity(value.Handle) != value.NormalizedHandle {
			return fmt.Errorf("invalid account %d", value.ID)
		}
		_, err = dbWrapper.Exec(ctx, `INSERT OR IGNORE INTO cam_model_accounts(id,model_id,site_id,handle,normalized_handle,status,valid_from,valid_to,source,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,'IMPORT',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, value.ID, value.ModelID, value.SiteID, value.Handle, value.NormalizedHandle, value.Status, value.ValidFrom, value.ValidTo)
		if err != nil {
			return err
		}
		got, findErr := s.FindAccount(ctx, value.ID)
		if findErr != nil {
			return findErr
		}
		if got == nil || !sameAccount(*got, value) {
			return fmt.Errorf("account %d conflicts with existing data", value.ID)
		}
	}
	for _, value := range input.Aliases {
		if value.ID <= 0 || value.ModelID <= 0 || normalizeCamIdentity(value.Alias) != value.NormalizedAlias {
			return fmt.Errorf("invalid alias %d", value.ID)
		}
		if value.AccountID != nil {
			account, accountErr := s.FindAccount(ctx, *value.AccountID)
			if accountErr != nil {
				return accountErr
			}
			if account == nil || account.ModelID != value.ModelID || (value.SiteID != nil && account.SiteID != *value.SiteID) {
				return fmt.Errorf("alias %d account/model/site mismatch", value.ID)
			}
		}
		_, err = dbWrapper.Exec(ctx, `INSERT OR IGNORE INTO cam_model_aliases(id,model_id,account_id,site_id,alias,normalized_alias,valid_to,is_current,source,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?, 'IMPORT',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, value.ID, value.ModelID, value.AccountID, value.SiteID, value.Alias, value.NormalizedAlias, value.ValidTo, value.IsCurrent)
		if err != nil {
			return err
		}
		got, findErr := s.FindAlias(ctx, value.ID)
		if findErr != nil {
			return findErr
		}
		if got == nil || !sameAlias(*got, value) {
			return fmt.Errorf("alias %d conflicts with existing data", value.ID)
		}
	}
	for _, value := range input.States {
		if value.UserID <= 0 || value.ModelID <= 0 || (value.Rating != nil && (*value.Rating < 1 || *value.Rating > 100)) {
			return fmt.Errorf("invalid user state %d/%d", value.UserID, value.ModelID)
		}
		_, err = dbWrapper.Exec(ctx, `INSERT OR IGNORE INTO cam_model_user_state(user_id,model_id,favorite,rating,updated_at) VALUES(?,?,?,?,?)`, value.UserID, value.ModelID, value.Favorite, value.Rating, value.UpdatedAt)
		if err != nil {
			return err
		}
		got, findErr := s.GetUserState(ctx, value.UserID, value.ModelID)
		if findErr != nil {
			return findErr
		}
		if got == nil || got.Favorite != value.Favorite || !sameIntPointer(got.Rating, value.Rating) || !got.UpdatedAt.Equal(value.UpdatedAt) {
			return fmt.Errorf("user state %d/%d conflicts with existing data", value.UserID, value.ModelID)
		}
	}
	return nil
}

func sameAccount(a, b CamModelAccount) bool {
	return a.ID == b.ID && a.ModelID == b.ModelID && a.SiteID == b.SiteID && a.Handle == b.Handle && a.NormalizedHandle == b.NormalizedHandle && a.Status == b.Status && sameTimePointer(a.ValidFrom, b.ValidFrom) && sameTimePointer(a.ValidTo, b.ValidTo)
}
func sameAlias(a, b CamModelAlias) bool {
	return a.ID == b.ID && a.ModelID == b.ModelID && sameInt64Pointer(a.AccountID, b.AccountID) && sameInt64Pointer(a.SiteID, b.SiteID) && a.Alias == b.Alias && a.NormalizedAlias == b.NormalizedAlias && sameTimePointer(a.ValidTo, b.ValidTo) && a.IsCurrent == b.IsCurrent
}
func sameTimePointer(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
func sameInt64Pointer(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func sameIntPointer(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *CamShowStore) ListSites(ctx context.Context) ([]CamSite, error) {
	var values []CamSite
	err := dbWrapper.Select(ctx, &values, `SELECT id,name,base_url,external_key,enabled,created_at,updated_at FROM cam_sites ORDER BY name COLLATE NOCASE,id`)
	return values, err
}
func (s *CamShowStore) ListModels(ctx context.Context) ([]CamModel, error) {
	var values []CamModel
	err := dbWrapper.Select(ctx, &values, `SELECT id,display_name,image,notes,performer_id,status,created_at,updated_at FROM cam_models ORDER BY display_name COLLATE NOCASE,id`)
	return values, err
}
func (s *CamShowStore) ListShows(ctx context.Context) ([]CamShow, error) {
	var values []CamShow
	err := dbWrapper.Select(ctx, &values, `SELECT id,scene_id,category,site_id,external_id,sync_state,created_at,updated_at FROM cam_shows ORDER BY id`)
	return values, err
}
func (s *CamShowStore) ListShowLibrary(ctx context.Context) ([]CamShowLibraryItem, error) {
	var values []CamShowLibraryItem
	err := dbWrapper.Select(ctx, &values, `SELECT cs.id,cs.scene_id,COALESCE(NULLIF(s.title,''),'Scene ' || cs.scene_id) AS title,cs.category FROM cam_shows cs JOIN scenes s ON s.id=cs.scene_id ORDER BY cs.id DESC`)
	if err != nil {
		return nil, err
	}
	for i := range values {
		if err := dbWrapper.Select(ctx, &values[i].Tags, `SELECT t.id,t.name FROM tags t JOIN scenes_tags st ON st.tag_id=t.id WHERE st.scene_id=? ORDER BY t.name COLLATE NOCASE,t.id`, values[i].SceneID); err != nil {
			return nil, err
		}
	}
	return values, nil
}
func (s *CamShowStore) ListAccounts(ctx context.Context, modelID int64) ([]CamModelAccount, error) {
	var values []CamModelAccount
	err := dbWrapper.Select(ctx, &values, `SELECT id,model_id,site_id,handle,normalized_handle,status,valid_from,valid_to FROM cam_model_accounts WHERE model_id=? ORDER BY id`, modelID)
	return values, err
}
func (s *CamShowStore) ListAliases(ctx context.Context, modelID int64) ([]CamModelAlias, error) {
	var values []CamModelAlias
	err := dbWrapper.Select(ctx, &values, `SELECT id,model_id,account_id,site_id,alias,normalized_alias,valid_to,is_current FROM cam_model_aliases WHERE model_id=? ORDER BY id`, modelID)
	return values, err
}
func (s *CamShowStore) SetSiteEnabled(ctx context.Context, id int64, enabled bool) error {
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_sites SET enabled=?,updated_at=? WHERE id=?`, enabled, time.Now().UTC(), id)
	return requireOneRow(result, err)
}
func (s *CamShowStore) SetModelStatus(ctx context.Context, id int64, status string) error {
	if status != "ACTIVE" && status != "INACTIVE" && status != "UNKNOWN" {
		return errors.New("invalid model status")
	}
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_models SET status=?,updated_at=? WHERE id=?`, status, time.Now().UTC(), id)
	return requireOneRow(result, err)
}
func (s *CamShowStore) UpdateShowCategory(ctx context.Context, id int64, category string) error {
	category = strings.TrimSpace(category)
	if category == "" {
		return errors.New("category is required")
	}
	result, err := dbWrapper.Exec(ctx, `UPDATE cam_shows SET category=?,updated_at=? WHERE id=?`, category, time.Now().UTC(), id)
	return requireOneRow(result, err)
}
func requireOneRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

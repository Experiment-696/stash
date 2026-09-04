package sqlite

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

type CamModelSocialProfile struct {
	ID         int64      `db:"id"`
	ModelID    int64      `db:"model_id"`
	Platform   string     `db:"platform"`
	Icon       *string    `db:"icon"`
	Handle     *string    `db:"handle"`
	URL        string     `db:"url"`
	Status     string     `db:"status"`
	ValidFrom  *time.Time `db:"valid_from"`
	ValidTo    *time.Time `db:"valid_to"`
	Source     string     `db:"source"`
	Confidence *float64   `db:"confidence"`
	Provenance *string    `db:"provenance"`
}
type CamModelSocialProfileInput struct {
	ModelID      int64
	Platform     string
	Icon, Handle *string
	URL          string
	ValidFrom    *time.Time
	Source       string
	Confidence   *float64
	Provenance   *string
}

func validCamHTTPURL(v string) bool {
	u, e := url.ParseRequestURI(v)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
func (s *CamShowStore) ListModelSocialProfiles(ctx context.Context, modelID int64) ([]CamModelSocialProfile, error) {
	var v []CamModelSocialProfile
	e := dbWrapper.Select(ctx, &v, `SELECT id,model_id,platform,icon,handle,url,status,valid_from,valid_to,source,confidence,provenance FROM cam_model_social_profiles WHERE model_id=? ORDER BY valid_to IS NULL DESC,platform,url,id`, modelID)
	return v, e
}
func (s *CamShowStore) AddModelSocialProfile(ctx context.Context, in CamModelSocialProfileInput) (*CamModelSocialProfile, error) {
	in.Platform, in.URL, in.Source = strings.TrimSpace(in.Platform), strings.TrimSpace(in.URL), strings.TrimSpace(in.Source)
	if in.ModelID <= 0 || in.Platform == "" || in.Source == "" || !validCamHTTPURL(in.URL) {
		return nil, errors.New("model, platform, source, and http(s) profile URL are required")
	}
	if in.Confidence != nil && (*in.Confidence < 0 || *in.Confidence > 1) {
		return nil, errors.New("confidence must be between zero and one")
	}
	now := time.Now().UTC()
	r, e := dbWrapper.Exec(ctx, `INSERT INTO cam_model_social_profiles(model_id,platform,icon,handle,url,status,valid_from,source,confidence,provenance,created_at,updated_at) VALUES(?,?,?,?,?,'ACTIVE',?,?,?,?,?,?)`, in.ModelID, in.Platform, in.Icon, in.Handle, in.URL, in.ValidFrom, in.Source, in.Confidence, in.Provenance, now, now)
	if e != nil {
		return nil, e
	}
	id, e := r.LastInsertId()
	if e != nil {
		return nil, e
	}
	return s.FindModelSocialProfile(ctx, id)
}
func (s *CamShowStore) FindModelSocialProfile(ctx context.Context, id int64) (*CamModelSocialProfile, error) {
	var v CamModelSocialProfile
	e := dbWrapper.Get(ctx, &v, `SELECT id,model_id,platform,icon,handle,url,status,valid_from,valid_to,source,confidence,provenance FROM cam_model_social_profiles WHERE id=?`, id)
	return &v, e
}
func (s *CamShowStore) RetireModelSocialProfile(ctx context.Context, id int64, validTo time.Time) (*CamModelSocialProfile, error) {
	if id <= 0 || validTo.IsZero() {
		return nil, errors.New("profile and retirement time are required")
	}
	r, e := dbWrapper.Exec(ctx, `UPDATE cam_model_social_profiles SET status='INACTIVE',valid_to=?,updated_at=? WHERE id=? AND status='ACTIVE'`, validTo.UTC(), time.Now().UTC(), id)
	if e := requireOneRow(r, e); e != nil {
		return nil, e
	}
	return s.FindModelSocialProfile(ctx, id)
}

type CamShowCoreUpdateInput struct {
	ID                                  int64
	Title, ShowType                     string
	ShowDate, CapturedAt                *time.Time
	CapturedTimezone, CapturedPrecision *string
	DurationOverrideSeconds             *float64
	DurationOverrideReason, Details     *string
}

func (s *CamShowStore) UpdateShowCore(ctx context.Context, in CamShowCoreUpdateInput) (*CamShowDomainItem, error) {
	in.Title = strings.TrimSpace(in.Title)
	switch in.ShowType {
	case "LIVE_PUBLIC", "LIVE_GROUP_TICKET_MULTIUSER", "LIVE_PRIVATE", "LIVE_EXCLUSIVE_PRIVATE", "CUSTOM_VIDEO", "PRIVATE_CALL":
	default:
		return nil, errors.New("valid Show type is required")
	}
	if in.ID <= 0 || in.Title == "" {
		return nil, errors.New("Show and title are required")
	}
	if in.DurationOverrideSeconds != nil && (in.DurationOverrideReason == nil || strings.TrimSpace(*in.DurationOverrideReason) == "") {
		return nil, errors.New("duration override reason is required")
	}
	if in.CapturedTimezone != nil && strings.TrimSpace(*in.CapturedTimezone) != "" {
		if _, e := time.LoadLocation(*in.CapturedTimezone); e != nil {
			return nil, errors.New("captured timezone is invalid")
		}
	}
	if in.CapturedPrecision != nil {
		switch *in.CapturedPrecision {
		case "DATE", "HOUR", "MINUTE", "SECOND":
		default:
			return nil, errors.New("captured precision is invalid")
		}
	}
	r, e := dbWrapper.Exec(ctx, "UPDATE cam_shows SET title_override=?,show_type=?,show_date=?,captured_at=?,captured_timezone=?,captured_precision=?,duration_override_seconds=?,duration_override_reason=?,notes=?,updated_at=? WHERE id=?", in.Title, in.ShowType, in.ShowDate, in.CapturedAt, in.CapturedTimezone, in.CapturedPrecision, in.DurationOverrideSeconds, in.DurationOverrideReason, in.Details, time.Now().UTC(), in.ID)
	if e := requireOneRow(r, e); e != nil {
		return nil, e
	}
	v, e := s.ListShowDomain(ctx)
	if e != nil {
		return nil, e
	}
	for i := range v {
		if v[i].ID == in.ID {
			return &v[i], nil
		}
	}
	return nil, errors.New("updated Show not found")
}

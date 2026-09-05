package camgirlfinder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/stashapp/stash/pkg/cammodel"
)

const ProviderKey = "camgirlfinder"

var platforms = map[string]string{"atv": "AmateurTV", "bc": "BongaCams", "c4": "Cam4", "cb": "Chaturbate", "cs": "CamSoda", "ctv": "CherryTV", "f4f": "Flirt4Free", "im": "ImLive", "lj": "LiveJasmin", "mfc": "MyFreeCams", "sc": "StripChat", "sm": "Streamate", "sr": "StreamRay", "stv": "ShowUpTV", "xl": "XloveCam"}

var (
	ErrDisabled                  = errors.New("camgirlfinder provider is disabled")
	ErrInvalidConfiguration      = errors.New("invalid camgirlfinder configuration")
	ErrUnsupportedAuthentication = errors.New("camgirlfinder authentication is not documented")
	ErrRateLimited               = errors.New("camgirlfinder request rate limited")
	ErrMalformedResponse         = errors.New("malformed camgirlfinder response")
)

type Config struct {
	Enabled           bool
	BaseURL           string
	UserAgent         string
	Credential        string
	RequestsPerSecond float64
	Timeout           time.Duration
	MaxResults        int
}

func (c Config) Validate() error {
	if !c.Enabled {
		return ErrDisabled
	}
	if strings.TrimSpace(c.Credential) != "" {
		return ErrUnsupportedAuthentication
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("%w: base URL must be an HTTPS origin", ErrInvalidConfiguration)
	}
	if strings.TrimSpace(c.UserAgent) == "" {
		return fmt.Errorf("%w: meaningful user agent is required", ErrInvalidConfiguration)
	}
	if c.RequestsPerSecond <= 0 || c.RequestsPerSecond > 10 {
		return fmt.Errorf("%w: requests per second must be within (0,10]", ErrInvalidConfiguration)
	}
	if c.Timeout <= 0 || c.Timeout > 2*time.Minute {
		return fmt.Errorf("%w: timeout must be within (0,2m]", ErrInvalidConfiguration)
	}
	if c.MaxResults <= 0 || c.MaxResults > 100 {
		return fmt.Errorf("%w: max results must be within [1,100]", ErrInvalidConfiguration)
	}
	return nil
}

type HTTPStatusError struct{ StatusCode int }

func (e HTTPStatusError) Error() string {
	return fmt.Sprintf("camgirlfinder returned HTTP %d", e.StatusCode)
}

type Client struct {
	config      Config
	http        *http.Client
	mu          sync.Mutex
	nextRequest time.Time
}

func New(config Config, transport http.RoundTripper) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Client{config: config, http: &http.Client{Transport: transport, Timeout: config.Timeout}}, nil
}

func (c *Client) Key() string { return ProviderKey }

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if now.Before(c.nextRequest) {
		wait = c.nextRequest.Sub(now)
	}
	base := now
	if c.nextRequest.After(base) {
		base = c.nextRequest
	}
	c.nextRequest = base.Add(time.Duration(float64(time.Second) / c.config.RequestsPerSecond))
	c.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type modelURLs struct {
	Profile         string `json:"profile"`
	ExternalProfile string `json:"externalProfile"`
}
type modelResult struct {
	Name      string          `json:"name"`
	Platform  string          `json:"platform"`
	Gender    string          `json:"gender"`
	Distance  float64         `json:"distance"`
	Faces     int             `json:"faces"`
	FirstSeen time.Time       `json:"firstSeen"`
	LastSeen  time.Time       `json:"lastSeen"`
	URLs      modelURLs       `json:"urls"`
	Persons   json.RawMessage `json:"persons"`
	Schedule  json.RawMessage `json:"schedule"`
}
type modelPerson struct {
	Person int64 `json:"person"`
	URLs   struct {
		FaceImage string `json:"faceImage"`
	} `json:"urls"`
}
type evidencePayload struct {
	Platform        string    `json:"platform"`
	Name            string    `json:"name"`
	Gender          string    `json:"gender"`
	Distance        float64   `json:"distance"`
	Faces           int       `json:"faces"`
	FirstSeen       time.Time `json:"firstSeen"`
	LastSeen        time.Time `json:"lastSeen"`
	Profile         string    `json:"profile"`
	ExternalProfile string    `json:"externalProfile"`
}

func (c *Client) Discover(ctx context.Context, query string) ([]cammodel.ProfileObservation, error) {
	query = strings.TrimSpace(query)
	if len(query) < 3 || len(query) > 50 {
		return nil, fmt.Errorf("%w: model query must be 3-50 characters", ErrInvalidConfiguration)
	}
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	endpoint, _ := url.Parse(strings.TrimRight(c.config.BaseURL, "/") + "/models/search")
	values := endpoint.Query()
	values.Set("model", query)
	endpoint.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set("Accept", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, ErrRateLimited
	}
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, HTTPStatusError{StatusCode: response.StatusCode}
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	decoder.DisallowUnknownFields()
	var results []modelResult
	if err := decoder.Decode(&results); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}
	if len(results) > 100 {
		return nil, fmt.Errorf("%w: provider exceeded documented 100-result bound", ErrMalformedResponse)
	}
	if len(results) > c.config.MaxResults {
		results = results[:c.config.MaxResults]
	}
	observations := make([]cammodel.ProfileObservation, 0, len(results))
	for _, result := range results {
		if _, ok := platforms[result.Platform]; !ok {
			return nil, fmt.Errorf("%w: undocumented platform code %q", ErrMalformedResponse, result.Platform)
		}
		if result.Name == "" || result.Platform == "" || result.FirstSeen.IsZero() || result.LastSeen.IsZero() || result.URLs.Profile == "" || result.URLs.ExternalProfile == "" {
			return nil, fmt.Errorf("%w: required account field missing", ErrMalformedResponse)
		}
		payload, err := json.Marshal(evidencePayload{Platform: result.Platform, Name: result.Name, Gender: result.Gender, Distance: result.Distance, Faces: result.Faces, FirstSeen: result.FirstSeen, LastSeen: result.LastSeen, Profile: result.URLs.Profile, ExternalProfile: result.URLs.ExternalProfile})
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(strings.Join([]string{result.Platform, result.Name, result.FirstSeen.UTC().Format(time.RFC3339Nano), result.LastSeen.UTC().Format(time.RFC3339Nano)}, "\x00")))
		source := result.URLs.ExternalProfile
		var people []modelPerson
		if len(result.Persons) > 0 && string(result.Persons) != "null" {
			if err := json.Unmarshal(result.Persons, &people); err != nil {
				return nil, fmt.Errorf("%w: invalid persons list", ErrMalformedResponse)
			}
		}
		var imageURL *string
		for _, person := range people {
			if value := strings.TrimSpace(person.URLs.FaceImage); value != "" {
				imageURL = &value
				break
			}
		}
		observations = append(observations, cammodel.ProfileObservation{Provider: ProviderKey, EvidenceKey: hex.EncodeToString(sum[:]), Platform: result.Platform, Username: result.Name, SourceURL: &source, ImageURL: imageURL, ObservedAt: result.LastSeen.UTC(), PayloadJSON: string(payload)})
	}
	return observations, nil
}

// SyncService only previews discovery or writes PENDING provenance through the
// deliberately narrow ingestion boundary. It has no account/identity mutation.
type SyncService struct {
	Provider  cammodel.DiscoveryProvider
	Ingestion cammodel.IngestionService
}

func (s SyncService) DryRun(ctx context.Context, query string) ([]cammodel.ProfileObservation, error) {
	return s.Provider.Discover(ctx, query)
}
func (s SyncService) IngestPending(ctx context.Context, modelID int64, query string) ([]cammodel.ObservationIngestResult, error) {
	observations, err := s.Provider.Discover(ctx, query)
	if err != nil {
		return nil, err
	}
	results := make([]cammodel.ObservationIngestResult, 0, len(observations))
	for _, observation := range observations {
		result, err := s.Ingestion.IngestProfileObservation(ctx, modelID, nil, observation)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

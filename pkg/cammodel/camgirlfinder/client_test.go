package camgirlfinder

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stashapp/stash/pkg/cammodel"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testConfig(baseURL string) Config {
	return Config{Enabled: true, BaseURL: baseURL, UserAgent: "stash-cs04a-test/1.0", RequestsPerSecond: 10, Timeout: time.Second, MaxResults: 100}
}
func TestConfigValidationIsDisabledAndCredentialFailClosed(t *testing.T) {
	c := testConfig("https://api.camgirlfinder.net")
	c.Enabled = false
	if !errors.Is(c.Validate(), ErrDisabled) {
		t.Fatal("disabled accepted")
	}
	c.Enabled = true
	c.Credential = "secret"
	if !errors.Is(c.Validate(), ErrUnsupportedAuthentication) {
		t.Fatal("credential accepted")
	}
	c.Credential = ""
	c.UserAgent = ""
	if !errors.Is(c.Validate(), ErrInvalidConfiguration) {
		t.Fatal("empty user agent accepted")
	}
	c.UserAgent = "x"
	c.BaseURL = "http://api.camgirlfinder.net"
	if !errors.Is(c.Validate(), ErrInvalidConfiguration) {
		t.Fatal("HTTP origin accepted")
	}
}
func TestDiscoverContractAllPlatformsBoundedAndRedacted(t *testing.T) {
	codes := make([]string, 0, len(platforms))
	for code := range platforms {
		codes = append(codes, code)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models/search" || r.URL.Query().Get("model") != "alice" {
			t.Errorf("request %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("User-Agent") != "stash-cs04a-test/1.0" {
			t.Errorf("user-agent=%q", r.Header.Get("User-Agent"))
		}
		values := make([]map[string]any, 0, len(codes))
		for _, code := range codes {
			values = append(values, map[string]any{"name": "alice_" + code, "platform": code, "gender": "f", "distance": 0.25, "faces": 2, "firstSeen": "2025-01-01T00:00:00Z", "lastSeen": "2026-01-01T00:00:00Z", "persons": []any{map[string]any{"person": 99, "urls": map[string]string{"faceImage": "https://images.invalid/face"}}}, "schedule": [][]float64{{0.5}}, "urls": map[string]string{"profile": "https://camgirlfinder.net/models/" + code + "/alice", "externalProfile": "https://api.camgirlfinder.net/out/" + code + "/alice"}})
		}
		json.NewEncoder(w).Encode(values)
	}))
	defer server.Close()
	client, err := New(testConfig(server.URL), server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Discover(context.Background(), "alice")
	if err != nil || len(got) != len(platforms) {
		t.Fatalf("observations=%d err=%v", len(got), err)
	}
	again, err := client.Discover(context.Background(), "alice")
	if err != nil || len(again) != len(got) || again[0].EvidenceKey != got[0].EvidenceKey || !again[0].ObservedAt.Equal(got[0].ObservedAt) {
		t.Fatalf("replay changed evidence: first=%+v again=%+v err=%v", got[0], again[0], err)
	}
	for _, o := range got {
		if o.Provider != ProviderKey || o.ProviderRecordID != nil || o.SourceURL == nil || o.EvidenceKey == "" {
			t.Fatalf("observation=%+v", o)
		}
		if strings.Contains(o.PayloadJSON, "person") || strings.Contains(o.PayloadJSON, "faceImage") || strings.Contains(o.PayloadJSON, "schedule") {
			t.Fatalf("unbounded payload=%s", o.PayloadJSON)
		}
	}
}
func TestDiscoverTypedErrorsAndCancellation(t *testing.T) {
	status := http.StatusTooManyRequests
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusOK {
			w.Write([]byte("[{}]"))
		}
	}))
	defer server.Close()
	cfg := testConfig(server.URL)
	cfg.RequestsPerSecond = 0.1
	client, err := New(cfg, server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Discover(context.Background(), "alice"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.Discover(ctx, "alice"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	status = http.StatusOK
	cfg.RequestsPerSecond = 10
	client, _ = New(cfg, server.Client().Transport)
	if _, err = client.Discover(context.Background(), "alice"); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("malformed error=%v", err)
	}
}

type providerFixture struct{ observations []cammodel.ProfileObservation }

func (p providerFixture) Key() string { return ProviderKey }
func (p providerFixture) Discover(context.Context, string) ([]cammodel.ProfileObservation, error) {
	return p.observations, nil
}

type storeFixture struct{ calls int }

func (s *storeFixture) IngestPendingProfileObservation(context.Context, int64, *int64, cammodel.ProfileObservation) (cammodel.ObservationIngestResult, error) {
	s.calls++
	return cammodel.ObservationIngestResult{EvidenceID: int64(s.calls), Status: cammodel.ObservationInserted}, nil
}
func TestSyncBoundaryIsDryRunOrPendingEvidenceOnly(t *testing.T) {
	o := cammodel.ProfileObservation{Provider: ProviderKey, EvidenceKey: "e", ObservedAt: time.Now(), PayloadJSON: "{}"}
	store := &storeFixture{}
	service := SyncService{Provider: providerFixture{[]cammodel.ProfileObservation{o}}, Ingestion: cammodel.NewIngestionService(store)}
	if got, err := service.DryRun(context.Background(), "alice"); err != nil || len(got) != 1 || store.calls != 0 {
		t.Fatalf("dry run=%v calls=%d err=%v", got, store.calls, err)
	}
	if got, err := service.IngestPending(context.Background(), 7, "alice"); err != nil || len(got) != 1 || store.calls != 1 {
		t.Fatalf("ingest=%v calls=%d err=%v", got, store.calls, err)
	}
	typ := reflect.TypeOf(service)
	if typ.NumMethod() != 2 {
		t.Fatalf("methods=%d", typ.NumMethod())
	}
	for _, name := range []string{"CreateAccount", "MergeIdentity", "StartRecording", "StopRecording", "PollPresence"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Fatalf("forbidden %s", name)
		}
	}
}

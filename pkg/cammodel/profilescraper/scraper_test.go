package profilescraper

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestScrapePublicProfileMetadata(t *testing.T) {
	html := `<!doctype html><html><head>
<meta property="og:image" content="/profile.webp">
<script type="application/ld+json">{"@type":"Person","age":27,"homeLocation":{"name":"Prague, Czechia"},"sameAs":["https://x.com/example_model","https://instagram.com/example.model"]}</script>
</head><body><a rel="me" href="https://t.me/examplemodel">Telegram</a></body></html>`
	scraper := New(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(html)), Request: request}, nil
	}))
	value, err := scraper.Scrape(context.Background(), "https://example.com/model")
	if err != nil {
		t.Fatal(err)
	}
	if value.ImageURL == nil || *value.ImageURL != "https://example.com/profile.webp" || value.Location == nil || *value.Location != "Prague, Czechia" || value.Age == nil || *value.Age != 27 {
		t.Fatalf("unexpected metadata: %#v", value)
	}
	if len(value.Socials) != 3 {
		t.Fatalf("expected 3 social links, got %#v", value.Socials)
	}
}

func TestExtractRejectsUnderageAgeValue(t *testing.T) {
	html := `<html><head><meta name="age" content="17"></head><body>Age: 17</body></html>`
	scraper := New(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(html)), Request: request}, nil
	}))
	value, err := scraper.Scrape(context.Background(), "https://example.com/model")
	if err != nil {
		t.Fatal(err)
	}
	if value.Age != nil {
		t.Fatalf("underage value must never be imported: %#v", value.Age)
	}
}

package profilescraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxBody = 4 << 20

var agePattern = regexp.MustCompile(`(?i)\bage\s*[:\-]\s*(\d{2})\b`)

type Social struct {
	Platform string
	Handle   *string
	URL      string
}

type Metadata struct {
	ImageURL *string
	Location *string
	Age      *int
	Socials  []Social
}

type Scraper struct {
	client *http.Client
}

func New(transport http.RoundTripper) *Scraper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	s := &Scraper{}
	s.client = &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many profile redirects")
			}
			return validatePublicURL(req.Context(), req.URL)
		},
	}
	return s
}

func validatePublicURL(ctx context.Context, value *url.URL) error {
	if value == nil || value.Scheme != "https" || value.User != nil || value.Hostname() == "" {
		return errors.New("profile URL must be public HTTPS")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, value.Hostname())
	if err != nil || len(addresses) == 0 {
		return errors.New("profile host cannot be resolved")
	}
	for _, address := range addresses {
		ip := address.IP
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return errors.New("profile host is not public")
		}
	}
	return nil
}

func (s *Scraper) Scrape(ctx context.Context, rawURL string) (Metadata, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || validatePublicURL(ctx, u) != nil {
		return Metadata{}, errors.New("profile URL must resolve to a public HTTPS host")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Metadata{}, err
	}
	req.Header.Set("User-Agent", "Stash Cam Model Profile Scraper/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := s.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Metadata{}, fmt.Errorf("profile returned HTTP %d", response.StatusCode)
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.Contains(mediaType, "html") {
		return Metadata{}, errors.New("profile did not return HTML")
	}
	doc, err := html.Parse(io.LimitReader(response.Body, maxBody))
	if err != nil {
		return Metadata{}, err
	}
	return extract(doc, response.Request.URL), nil
}

func extract(root *html.Node, base *url.URL) Metadata {
	var result Metadata
	seen := map[string]struct{}{}
	var text strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			text.WriteString(" ")
			text.WriteString(node.Data)
		}
		if node.Type == html.ElementNode {
			attrs := attributes(node)
			if node.Data == "meta" {
				key := strings.ToLower(first(attrs["property"], attrs["name"], attrs["itemprop"]))
				value := strings.TrimSpace(attrs["content"])
				switch key {
				case "og:image", "twitter:image", "image":
					setURL(&result.ImageURL, value, base)
				case "profile:location", "location", "home_location":
					setString(&result.Location, value)
				case "profile:age", "age":
					setAge(&result.Age, value)
				}
			}
			if node.Data == "a" {
				relations := strings.Fields(strings.ToLower(attrs["rel"]))
				identityLink := false
				for _, relation := range relations {
					identityLink = identityLink || relation == "me"
				}
				if resolved := resolveHTTPURL(attrs["href"], base); identityLink && resolved != "" {
					if social, ok := socialFromURL(resolved); ok {
						if _, exists := seen[social.URL]; !exists {
							seen[social.URL] = struct{}{}
							result.Socials = append(result.Socials, social)
						}
					}
				}
			}
			if node.Data == "script" && strings.EqualFold(attrs["type"], "application/ld+json") && node.FirstChild != nil {
				var value any
				if json.Unmarshal([]byte(node.FirstChild.Data), &value) == nil {
					readJSONLD(value, &result, base, seen)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if result.Age == nil {
		if match := agePattern.FindStringSubmatch(text.String()); len(match) == 2 {
			setAge(&result.Age, match[1])
		}
	}
	return result
}

func attributes(node *html.Node) map[string]string {
	ret := map[string]string{}
	for _, attr := range node.Attr {
		ret[strings.ToLower(attr.Key)] = attr.Val
	}
	return ret
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func setString(target **string, value string) {
	value = strings.TrimSpace(value)
	if *target == nil && value != "" && len(value) <= 200 {
		*target = &value
	}
}

func setAge(target **int, value string) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if *target == nil && err == nil && parsed >= 18 && parsed <= 120 {
		*target = &parsed
	}
}

func setURL(target **string, value string, base *url.URL) {
	if *target == nil {
		if resolved := resolveHTTPURL(value, base); resolved != "" {
			*target = &resolved
		}
	}
}

func resolveHTTPURL(value string, base *url.URL) string {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.User != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

func socialFromURL(value string) (Social, bool) {
	u, err := url.Parse(value)
	if err != nil {
		return Social{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	platforms := map[string]string{
		"twitter.com": "X / Twitter", "x.com": "X / Twitter", "instagram.com": "Instagram",
		"tiktok.com": "TikTok", "youtube.com": "YouTube", "youtu.be": "YouTube",
		"reddit.com": "Reddit", "telegram.me": "Telegram", "t.me": "Telegram",
		"onlyfans.com": "OnlyFans", "fansly.com": "Fansly", "manyvids.com": "ManyVids",
		"linktr.ee": "Linktree", "allmylinks.com": "AllMyLinks",
	}
	platform, ok := platforms[host]
	if !ok {
		return Social{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	var handle *string
	if len(parts) > 0 && parts[0] != "" {
		value := strings.TrimPrefix(parts[0], "@")
		handle = &value
	}
	return Social{Platform: platform, Handle: handle, URL: value}, true
}

func readJSONLD(value any, result *Metadata, base *url.URL, seen map[string]struct{}) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			readJSONLD(item, result, base, seen)
		}
	case map[string]any:
		if graph, ok := typed["@graph"]; ok {
			readJSONLD(graph, result, base, seen)
		}
		if image, ok := typed["image"].(string); ok {
			setURL(&result.ImageURL, image, base)
		}
		if age, ok := typed["age"].(float64); ok {
			setAge(&result.Age, strconv.Itoa(int(age)))
		}
		for _, key := range []string{"homeLocation", "nationality", "address"} {
			if location := jsonLDText(typed[key]); location != "" {
				setString(&result.Location, location)
			}
		}
		if sameAs, ok := typed["sameAs"]; ok {
			values, _ := sameAs.([]any)
			for _, item := range values {
				if raw, ok := item.(string); ok {
					if resolved := resolveHTTPURL(raw, base); resolved != "" {
						if social, ok := socialFromURL(resolved); ok {
							if _, exists := seen[social.URL]; !exists {
								seen[social.URL] = struct{}{}
								result.Socials = append(result.Socials, social)
							}
						}
					}
				}
			}
		}
	}
}

func jsonLDText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, key := range []string{"name", "addressLocality", "addressCountry"} {
			if value, ok := typed[key].(string); ok && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

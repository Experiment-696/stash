package main

import "testing"

func TestValidateLocalBrowserURL(t *testing.T) {
	for _, value := range []string{"http://localhost:9999", "http://127.0.0.1:9999/path", "https://[::1]:9999"} {
		if err := validateLocalBrowserURL(value); err != nil {
			t.Errorf("rejected local URL %q: %v", value, err)
		}
	}
	for _, value := range []string{"https://example.com", "file:///tmp/page.html", "http://192.168.1.2"} {
		if err := validateLocalBrowserURL(value); err == nil {
			t.Errorf("accepted non-local URL %q", value)
		}
	}
}

package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeStaticContentNoStore(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	ServeStaticContentNoStore(w, r, []byte("shell"))

	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q want no-store", got)
	}
	if got := w.Header().Get("ETag"); got != "" {
		t.Fatalf("ETag=%q want empty", got)
	}
	if w.Code != http.StatusOK || w.Body.String() != "shell" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestServeStaticContentNoStoreIgnoresConditionalETag(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("If-None-Match", GenerateETag([]byte("shell")))
	w := httptest.NewRecorder()

	ServeStaticContentNoStore(w, r, []byte("shell"))

	if w.Code != http.StatusOK || w.Body.String() != "shell" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

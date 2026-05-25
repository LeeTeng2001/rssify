package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeeTeng2001/rssify/internal/cache"
)

func assertStatus(t *testing.T, h http.Handler, method, path string, wantCode int, wantContentType string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantCode {
		t.Errorf("%s %s: status = %d, want %d", method, path, rec.Code, wantCode)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != wantContentType {
		t.Errorf("%s %s: Content-Type = %q, want %q", method, path, ct, wantContentType)
	}
}

func TestFeedHitMissAndUnknown(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	if err := c.Put("hn", []byte("<rss/>")); err != nil {
		t.Fatalf("cache.Put() error = %v", err)
	}

	h := New(c, []string{"hn", "empty"})

	assertStatus(t, h, http.MethodGet, "/feed/hn.xml", http.StatusOK, "application/rss+xml; charset=utf-8")
	assertStatus(t, h, http.MethodGet, "/feed/empty.xml", http.StatusServiceUnavailable, "text/plain; charset=utf-8")
	assertStatus(t, h, http.MethodGet, "/feed/nope.xml", http.StatusNotFound, "text/plain; charset=utf-8")
}

func TestMethodNotAllowed(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	h := New(c, []string{"hn"})

	assertStatus(t, h, http.MethodPost, "/feed/hn.xml", http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
}

func TestHEADAllowed(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	if err := c.Put("hn", []byte("<rss/>")); err != nil {
		t.Fatalf("cache.Put() error = %v", err)
	}

	h := New(c, []string{"hn"})

	assertStatus(t, h, http.MethodHead, "/feed/hn.xml", http.StatusOK, "application/rss+xml; charset=utf-8")
}

func TestFeedSlashOnly(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	h := New(c, []string{"hn"})

	assertStatus(t, h, http.MethodGet, "/feed/", http.StatusNotFound, "text/plain; charset=utf-8")
}

func TestFeedMissingXMLExtension(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	h := New(c, []string{"hn"})

	assertStatus(t, h, http.MethodGet, "/feed/hn", http.StatusNotFound, "text/plain; charset=utf-8")
}
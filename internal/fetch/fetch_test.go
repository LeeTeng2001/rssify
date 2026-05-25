package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientGetReturnsBodyFinalURLAndUserAgent(t *testing.T) {
	t.Parallel()

	var userAgent string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(ts.Close)

	client := NewClient("rssify-test", time.Second)
	body, finalURL, err := client.Get(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("Get() body = %q, want %q", body, "hello")
	}
	if finalURL.String() != ts.URL {
		t.Fatalf("Get() finalURL = %q, want %q", finalURL.String(), ts.URL)
	}
	if userAgent != "rssify-test" {
		t.Fatalf("User-Agent = %q, want %q", userAgent, "rssify-test")
	}
}

func TestClientGetTreatsServerErrorAsRetryable(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	t.Cleanup(ts.Close)

	client := NewClient("rssify-test", time.Second)
	_, _, err := client.Get(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("Get() error = nil, want error")
	}
	if !IsRetryable(err) {
		t.Fatal("IsRetryable() = false, want true")
	}
}

func TestClientGetTreatsNotFoundAsNonRetryable(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(ts.Close)

	client := NewClient("rssify-test", time.Second)
	_, _, err := client.Get(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("Get() error = nil, want error")
	}
	if IsRetryable(err) {
		t.Fatal("IsRetryable() = true, want false")
	}
}

func TestIsRetryableFalseForUnknownError(t *testing.T) {
	t.Parallel()

	if IsRetryable(errors.New("x")) {
		t.Fatal("IsRetryable() = true, want false")
	}
}

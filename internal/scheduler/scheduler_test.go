package scheduler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LeeTeng2001/rssify/internal/cache"
	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/LeeTeng2001/rssify/internal/extract"
	"github.com/LeeTeng2001/rssify/internal/fetch"
)

func TestScrapeKeepsOldCacheOnFetchFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	if err := c.Put("hn", []byte("old")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	fetchFunc := FetchFunc(func(_ context.Context, _ string) ([]byte, *url.URL, error) {
		return nil, nil, errors.New("down")
	})

	feed := config.FeedConfig{
		ID:          "hn",
		URL:         "https://example.com",
		Title:       "Hacker News",
		Description: "HN",
		Link:        "https://example.com",
		Interval:    5 * time.Minute,
	}

	s := New([]config.FeedConfig{feed}, c, fetchFunc, Options{
		MaxAttempts:  1,
		RetryBackoff: 1 * time.Millisecond,
	})

	s.scrape(context.Background(), s.feeds[0])

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if string(got) != "old" {
		t.Fatalf("Get() = %q, want %q", got, "old")
	}
}

func TestScrapeWritesRSSOnSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}

	itemSel, err := config.CompileSelector(".item")
	if err != nil {
		t.Fatalf("CompileSelector(.item) error = %v", err)
	}
	titleSel, err := config.CompileSelector(".title")
	if err != nil {
		t.Fatalf("CompileSelector(.title) error = %v", err)
	}
	linkSel, err := config.CompileSelector("a")
	if err != nil {
		t.Fatalf("CompileSelector(a) error = %v", err)
	}

	feed := config.FeedConfig{
		ID:          "hn",
		URL:         "https://e.test/list",
		Title:       "Hacker News",
		Description: "HN Desc",
		Link:        "https://e.test",
		Interval:    5 * time.Minute,
		Rule: config.CompiledRule{
			Item:  itemSel,
			Title: config.CompiledField{Selector: titleSel},
			Link:  config.CompiledField{Selector: linkSel, Attr: "href", Absolute: true},
		},
	}

	htmlBody := `<div class="item"><a class="title" href="/1">One</a></div>`
	baseURL, _ := url.Parse("https://e.test/list")

	fetchFunc := FetchFunc(func(_ context.Context, _ string) ([]byte, *url.URL, error) {
		return []byte(htmlBody), baseURL, nil
	})

	s := New([]config.FeedConfig{feed}, c, fetchFunc, Options{
		MaxAttempts:  1,
		RetryBackoff: 1 * time.Millisecond,
		SelfURL: func(feedID string) string {
			return "https://rss.test/feed/" + feedID + ".xml"
		},
	})

	s.scrape(context.Background(), s.feeds[0])

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	gotStr := string(got)
	if !strings.Contains(gotStr, "<rss") {
		t.Fatalf("RSS output does not contain <rss, got: %s", gotStr)
	}
	if !strings.Contains(gotStr, "One") {
		t.Fatalf("RSS output does not contain 'One', got: %s", gotStr)
	}
}

func TestScrapeRetriesOnRetryableError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}

	attempts := 0
	fetchFunc := FetchFunc(func(_ context.Context, rawURL string) ([]byte, *url.URL, error) {
		attempts++
		if attempts < 3 {
			return nil, nil, fetch.HTTPError{StatusCode: http.StatusBadGateway, URL: rawURL}
		}
		baseURL, _ := url.Parse("https://e.test/list")
		return []byte(`<div class="item"><a class="title" href="/1">One</a></div>`), baseURL, nil
	})

	itemSel, _ := config.CompileSelector(".item")
	titleSel, _ := config.CompileSelector(".title")
	linkSel, _ := config.CompileSelector("a")

	feed := config.FeedConfig{
		ID:          "hn",
		URL:         "https://e.test/list",
		Title:       "Hacker News",
		Description: "HN Desc",
		Link:        "https://e.test",
		Interval:    5 * time.Minute,
		Rule: config.CompiledRule{
			Item:  itemSel,
			Title: config.CompiledField{Selector: titleSel},
			Link:  config.CompiledField{Selector: linkSel, Attr: "href", Absolute: true},
		},
	}

	s := New([]config.FeedConfig{feed}, c, fetchFunc, Options{
		MaxAttempts:  3,
		RetryBackoff: 1 * time.Millisecond,
	})

	s.scrape(context.Background(), s.feeds[0])

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if !strings.Contains(string(got), "One") {
		t.Fatalf("expected RSS with 'One' after retry, got: %s", string(got))
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestScrapeNoRetryOnNonRetryableError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}

	if err := c.Put("hn", []byte("old")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	attempts := 0
	fetchFunc := FetchFunc(func(_ context.Context, _ string) ([]byte, *url.URL, error) {
		attempts++
		return nil, nil, errors.New("non-retryable")
	})

	feed := config.FeedConfig{
		ID:          "hn",
		URL:         "https://e.test/list",
		Title:       "Hacker News",
		Description: "HN Desc",
		Link:        "https://e.test",
		Interval:    5 * time.Minute,
	}

	s := New([]config.FeedConfig{feed}, c, fetchFunc, Options{
		MaxAttempts:  3,
		RetryBackoff: 1 * time.Millisecond,
	})

	s.scrape(context.Background(), s.feeds[0])

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if string(got) != "old" {
		t.Fatalf("Get() = %q, want %q (cache preserved)", got, "old")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestScrapeKeepsCacheOnExtractError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	if err := c.Put("hn", []byte("old")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	fetchFunc := FetchFunc(func(_ context.Context, _ string) ([]byte, *url.URL, error) {
		baseURL, _ := url.Parse("https://e.test/list")
		return []byte("not valid html <><>"), baseURL, nil
	})

	feed := config.FeedConfig{
		ID:          "hn",
		URL:         "https://e.test/list",
		Title:       "Hacker News",
		Description: "HN Desc",
		Link:        "https://e.test",
		Interval:    5 * time.Minute,
		Rule: config.CompiledRule{
			Item:  itemSel,
			Title: config.CompiledField{Selector: titleSel},
			Link:  config.CompiledField{Selector: linkSel, Attr: "href", Absolute: true},
		},
	}

	s := New([]config.FeedConfig{feed}, c, fetchFunc, Options{
		MaxAttempts:  1,
		RetryBackoff: 1 * time.Millisecond,
	})

	s.scrape(context.Background(), s.feeds[0])

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if string(got) != "old" {
		t.Fatalf("Get() = %q, want %q (cache preserved on extract error)", got, "old")
	}
}

func TestScrapeKeepsCacheOnZeroItems(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}
	if err := c.Put("hn", []byte("old")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	fetchFunc := FetchFunc(func(_ context.Context, _ string) ([]byte, *url.URL, error) {
		baseURL, _ := url.Parse("https://e.test/list")
		return []byte(`<div>no items here</div>`), baseURL, nil
	})

	feed := config.FeedConfig{
		ID:          "hn",
		URL:         "https://e.test/list",
		Title:       "Hacker News",
		Description: "HN Desc",
		Link:        "https://e.test",
		Interval:    5 * time.Minute,
		Rule: config.CompiledRule{
			Item:  itemSel,
			Title: config.CompiledField{Selector: titleSel},
			Link:  config.CompiledField{Selector: linkSel, Attr: "href", Absolute: true},
		},
	}

	s := New([]config.FeedConfig{feed}, c, fetchFunc, Options{
		MaxAttempts:  1,
		RetryBackoff: 1 * time.Millisecond,
	})

	s.scrape(context.Background(), s.feeds[0])

	got, ok := c.Get("hn")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if string(got) != "old" {
		t.Fatalf("Get() = %q, want %q (cache preserved on zero items)", got, "old")
	}
}

func TestScrapeRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache.New() error = %v", err)
	}

	fetchFunc := FetchFunc(func(ctx context.Context, _ string) ([]byte, *url.URL, error) {
		return nil, nil, ctx.Err()
	})

	feed := config.FeedConfig{
		ID:          "hn",
		URL:         "https://e.test/list",
		Title:       "Hacker News",
		Description: "HN Desc",
		Link:        "https://e.test",
		Interval:    5 * time.Minute,
	}

	s := New([]config.FeedConfig{feed}, c, fetchFunc, Options{
		MaxAttempts:  1,
		RetryBackoff: 1 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.scrape(ctx, s.feeds[0])

	if _, ok := c.Get("hn"); ok {
		t.Fatal("Get() ok = true, want false (no data should be written)")
	}
}

var (
	itemSel, _  = config.CompileSelector(".item")
	titleSel, _ = config.CompileSelector(".title")
	linkSel, _  = config.CompileSelector("a")
)

func init() {
	var _ extract.Item
}

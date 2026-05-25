package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rssify.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadValidConfigAppliesDefaultsAndCompilesSelectors(t *testing.T) {
	path := writeConfig(t, `
[[feed]]
id = "hn"
url = "https://news.ycombinator.com/"
title = "Hacker News"
interval = "5m"

[feed.rule]
item = ".athing"

[feed.rule.title]
selector = ".titleline > a"

[feed.rule.link]
selector = ".titleline > a"
attr = "href"
absolute = true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Server.Listen != ":8080" {
		t.Fatalf("Listen = %q, want :8080", cfg.Server.Listen)
	}
	if cfg.Server.CacheDir != "./cache" {
		t.Fatalf("CacheDir = %q, want ./cache", cfg.Server.CacheDir)
	}
	if cfg.Server.UserAgent != "rssify/dev" {
		t.Fatalf("UserAgent = %q, want rssify/dev", cfg.Server.UserAgent)
	}
	if cfg.Server.FetchTimeout != 15*time.Second {
		t.Fatalf("FetchTimeout = %v, want 15s", cfg.Server.FetchTimeout)
	}
	if cfg.Scrape.MaxAttempts != 3 {
		t.Fatalf("MaxAttempts = %d, want 3", cfg.Scrape.MaxAttempts)
	}
	if cfg.Scrape.RetryBackoff != 30*time.Second {
		t.Fatalf("RetryBackoff = %v, want 30s", cfg.Scrape.RetryBackoff)
	}

	feed := cfg.Feeds[0]
	if feed.Description != feed.Title {
		t.Fatalf("Description = %q, want title %q", feed.Description, feed.Title)
	}
	if feed.Link != feed.URL {
		t.Fatalf("Link = %q, want URL %q", feed.Link, feed.URL)
	}
	if feed.Rule.Item == nil || feed.Rule.Title.Selector == nil || feed.Rule.Link.Selector == nil {
		t.Fatalf("expected selectors to be compiled: %#v", feed.Rule)
	}
}

func TestLoadRejectsFormatOnNonPubDateFieldWithoutSelector(t *testing.T) {
	path := writeConfig(t, `
[[feed]]
id = "hn"
url = "https://news.ycombinator.com/"
title = "Hacker News"
interval = "5m"

[feed.rule]
item = ".athing"

[feed.rule.title]
selector = ".titleline > a"

[feed.rule.link]
selector = ".titleline > a"
attr = "href"

[feed.rule.description]
format = "2006-01-02"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "description format") {
		t.Fatalf("error %q missing description format", err)
	}
}

func TestLoadExpandsAIEnvVar(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	path := writeConfig(t, `
[ai]
api_key = "$OPENAI_API_KEY"

[[feed]]
id = "hn"
url = "https://news.ycombinator.com/"
title = "Hacker News"
interval = "5m"

[feed.rule]
item = ".athing"

[feed.rule.title]
selector = ".titleline > a"

[feed.rule.link]
selector = ".titleline > a"
attr = "href"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.AI.APIKey != "secret" {
		t.Fatalf("APIKey = %q, want secret", cfg.AI.APIKey)
	}
}

func TestLoadReportsMultipleValidationErrors(t *testing.T) {
	path := writeConfig(t, `
[scrape]
max_attempts = 0
retry_backoff = "500ms"

[[feed]]
id = "Bad_ID"
url = "/relative"
title = ""
interval = "10s"

[feed.rule]
item = "["

[feed.rule.link]
selector = "a"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	errText := err.Error()
	for _, want := range []string{"max_attempts", "retry_backoff", "feed Bad_ID", "title", "interval", "selector"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error %q missing %q", errText, want)
		}
	}
}

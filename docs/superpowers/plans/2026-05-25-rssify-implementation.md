# rssify Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a low-footprint Go RSS generator that scrapes configured HTML pages on a schedule, serves cached RSS feeds, and provides a CLI probe mode with optional AI-assisted rule authoring.

**Architecture:** One Go binary with `serve`, `probe`, and `version` subcommands using `urfave/cli/v3`. Runtime server path is config -> scheduler -> fetch -> extract -> render -> cache -> HTTP; authoring path is probe -> fetch -> extract, optionally probe -> AI -> extract refinement loop. Pure packages (`extract`, `render`) are isolated from I/O for deterministic tests.

**Tech Stack:** Go 1.23+, `github.com/urfave/cli/v3`, `github.com/BurntSushi/toml`, `github.com/PuerkitoBio/goquery`, `github.com/antchfx/htmlquery`, `github.com/lmittmann/tint`, `github.com/openai/openai-go`, stdlib `log/slog`, `net/http`, `encoding/xml`.

---

## File Structure

- Create `go.mod`: module path `github.com/LeeTeng2001/rssify` (normalized from the user's URL; Go module paths do not include `https://`).
- Create `main.go`: root CLI construction with `urfave/cli/v3`; global config/log flags; subcommand registration.
- Create `cmd/serve/serve.go`: `serve` command action, config load, logging setup, cache startup, scheduler startup, HTTP server lifecycle, graceful shutdown.
- Create `cmd/probe/probe.go`: `probe` command action, verify mode, AI suggest loop, item table / JSON output.
- Create `cmd/version/version.go`: version command and version variables.
- Create `internal/logging/logging.go`: `slog` setup using tint by default, JSON optional.
- Create `internal/config/config.go`: TOML structs, public config types, load/default/env expansion/validation.
- Create `internal/config/selectors.go`: selector compilation types used by `extract`.
- Create `internal/fetch/fetch.go`: context-aware HTTP GET wrapper and retryable-error classification.
- Create `internal/extract/extract.go`: HTML -> `[]Item` extraction using compiled selectors.
- Create `internal/render/render.go`: RSS 2.0 XML rendering.
- Create `internal/cache/cache.go`: concurrent in-memory XML cache plus atomic disk writes.
- Create `internal/scheduler/scheduler.go`: one goroutine per feed, jitter, retry loop, non-destructive failures.
- Create `internal/server/server.go`: `/feed/<id>.xml` handler.
- Create `internal/ai/ai.go`: thin wrapper around `openai-go` chat completions.
- Create `rssify.toml.example`: runnable example config.
- Create `README.md`: install, configure, serve, probe, AI setup.
- Create focused `_test.go` files beside each package.

---

### Task 1: Go Module, CLI Skeleton, Logging

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `cmd/version/version.go`
- Create: `internal/logging/logging.go`
- Test: `internal/logging/logging_test.go`

- [ ] **Step 1: Initialize module and dependencies**

Run:

```bash
go mod init github.com/LeeTeng2001/rssify
go get github.com/urfave/cli/v3 github.com/lmittmann/tint
```

Expected: `go.mod` exists and contains module path `github.com/LeeTeng2001/rssify`.

- [ ] **Step 2: Write logging tests**

Create `internal/logging/logging_test.go`:

```go
package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewTintLoggerWritesMessage(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New("debug", "tint", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	logger.Info("hello", slog.String("feed", "hn"))
	out := buf.String()
	if !strings.Contains(out, "hello") || !strings.Contains(out, "feed") || !strings.Contains(out, "hn") {
		t.Fatalf("tint output missing expected content: %q", out)
	}
}

func TestNewJSONLoggerWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	logger, err := New("info", "json", &buf)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	logger.Info("hello")
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Fatalf("json output missing message: %q", out)
	}
}

func TestNewRejectsInvalidLevelAndFormat(t *testing.T) {
	if _, err := New("verbose", "tint", &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid level error")
	}
	if _, err := New("info", "xml", &bytes.Buffer{}); err == nil {
		t.Fatal("expected invalid format error")
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
go test ./internal/logging
```

Expected: FAIL because `New` is undefined.

- [ ] **Step 4: Implement logging**

Create `internal/logging/logging.go`:

```go
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/lmittmann/tint"
)

func New(levelName, format string, w io.Writer) (*slog.Logger, error) {
	level, err := parseLevel(levelName)
	if err != nil {
		return nil, err
	}

	switch strings.ToLower(format) {
	case "tint":
		return slog.New(tint.NewHandler(w, &tint.Options{
			Level:      level,
			TimeFormat: time.Kitchen,
		})), nil
	case "json":
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})), nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}
}

func parseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level %q", name)
	}
}
```

- [ ] **Step 5: Add CLI skeleton**

Create `cmd/version/version.go`:

```go
package version

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"
)

var Version = "dev"

func Command(out io.Writer) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print version",
		Action: func(ctx context.Context, c *cli.Command) error {
			_, err := fmt.Fprintln(out, Version)
			return err
		},
	}
}
```

Create `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/LeeTeng2001/rssify/cmd/version"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "rssify",
		Usage: "turn configured HTML pages into RSS feeds",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Value: "rssify.toml", Usage: "path to config file"},
			&cli.StringFlag{Name: "log-level", Value: "info", Usage: "debug, info, warn, or error"},
			&cli.StringFlag{Name: "log-format", Value: "tint", Usage: "tint or json"},
		},
		Commands: []*cli.Command{
			version.Command(os.Stdout),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Verify and commit**

Run:

```bash
go test ./...
go run . version
```

Expected: tests PASS; `go run . version` prints `dev`.

Commit:

```bash
git add go.mod go.sum main.go cmd/version/version.go internal/logging/logging.go internal/logging/logging_test.go
git commit -m "chore: scaffold cli and logging"
```

---

### Task 2: Config Loading, Validation, Selector Compilation

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/selectors.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Add parser and selector dependencies**

Run:

```bash
go get github.com/BurntSushi/toml github.com/PuerkitoBio/goquery github.com/antchfx/htmlquery github.com/antchfx/xpath golang.org/x/net/html
```

Expected: dependencies added to `go.mod` and `go.sum`.

- [ ] **Step 2: Write config tests**

Create `internal/config/config_test.go`:

```go
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
	dir := t.TempDir()
	path := filepath.Join(dir, "rssify.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfigAppliesDefaultsAndCompilesSelectors(t *testing.T) {
	path := writeConfig(t, `
[[feed]]
id = "hn"
url = "https://news.ycombinator.com/"
title = "HN"
interval = "10m"

[feed.rule]
item = "tr.athing"
[feed.rule.title]
selector = "span.titleline > a"
[feed.rule.link]
selector = "span.titleline > a"
attr = "href"
absolute = true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Listen != ":8080" {
		t.Fatalf("default listen = %q", cfg.Server.Listen)
	}
	if cfg.Scrape.MaxAttempts != 3 || cfg.Scrape.RetryBackoff != 30*time.Second {
		t.Fatalf("unexpected scrape defaults: %+v", cfg.Scrape)
	}
	if len(cfg.Feeds) != 1 || cfg.Feeds[0].Description != "HN" || cfg.Feeds[0].Link != "https://news.ycombinator.com/" {
		t.Fatalf("unexpected feed defaults: %+v", cfg.Feeds)
	}
	if cfg.Feeds[0].Rule.Item == nil || cfg.Feeds[0].Rule.Title.Selector == nil || cfg.Feeds[0].Rule.Link.Selector == nil {
		t.Fatal("selectors were not compiled")
	}
}

func TestLoadExpandsAIEnvVar(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "secret")
	path := writeConfig(t, `
[ai]
api_key = "$OPENAI_API_KEY"
model = "gpt-4o-mini"

[[feed]]
id = "x"
url = "https://example.com/"
title = "X"
interval = "1m"
[feed.rule]
item = ".item"
[feed.rule.title]
selector = ".title"
[feed.rule.link]
selector = "a"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.APIKey != "secret" {
		t.Fatalf("APIKey = %q", cfg.AI.APIKey)
	}
}

func TestLoadReportsMultipleValidationErrors(t *testing.T) {
	path := writeConfig(t, `
[scrape]
max_attempts = 0
retry_backoff = "500ms"

[[feed]]
id = "Bad_ID"
url = "not a url"
title = ""
interval = "10s"
[feed.rule]
item = "["
[feed.rule.title]
selector = ""
[feed.rule.link]
selector = "a"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, want := range []string{"max_attempts", "retry_backoff", "feed Bad_ID", "title", "interval", "selector"} {
		if !contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func contains(s, substr string) bool { return strings.Contains(s, substr) }
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
go test ./internal/config
```

Expected: FAIL because `Load` and types are undefined.

- [ ] **Step 4: Implement selector compilation**

Create `internal/config/selectors.go`:

```go
package config

import (
	"fmt"
	"strings"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

type Selector interface {
	Find(node *html.Node) []*html.Node
}

type cssSelector struct{ sel cascadia.Selector }

func (s cssSelector) Find(node *html.Node) []*html.Node { return cascadia.QueryAll(node, s.sel) }

type xpathSelector struct{ expr *xpath.Expr }

func (s xpathSelector) Find(node *html.Node) []*html.Node { return htmlquery.QuerySelectorAll(node, s.expr) }

func CompileSelector(raw string) (Selector, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("selector is empty")
	}
	if strings.HasPrefix(raw, "xpath:") {
		expr, err := xpath.Compile(strings.TrimPrefix(raw, "xpath:"))
		if err != nil {
			return nil, err
		}
		return xpathSelector{expr: expr}, nil
	}
	sel, err := cascadia.Compile(raw)
	if err != nil {
		return nil, err
	}
	return cssSelector{sel: sel}, nil
}
```

- [ ] **Step 5: Implement config loading and validation**

Create `internal/config/config.go` with these public types and functions:

```go
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server ServerConfig
	Scrape ScrapeConfig
	AI     AIConfig
	Feeds  []FeedConfig `toml:"feed"`
}

type ServerConfig struct {
	Listen       string
	CacheDir     string `toml:"cache_dir"`
	UserAgent    string `toml:"user_agent"`
	FetchTimeout time.Duration `toml:"fetch_timeout"`
}

type ScrapeConfig struct {
	MaxAttempts  int           `toml:"max_attempts"`
	RetryBackoff time.Duration `toml:"retry_backoff"`
}

type AIConfig struct {
	BaseURL string `toml:"base_url"`
	APIKey  string `toml:"api_key"`
	Model   string
}

type FeedConfig struct {
	ID          string
	URL         string
	Title       string
	Description string
	Link        string
	Interval    time.Duration
	Rule        CompiledRule
}

type rawConfig struct {
	Server rawServerConfig
	Scrape rawScrapeConfig
	AI     AIConfig
	Feeds  []rawFeedConfig `toml:"feed"`
}

type rawServerConfig struct { Listen string; CacheDir string `toml:"cache_dir"`; UserAgent string `toml:"user_agent"`; FetchTimeout string `toml:"fetch_timeout"` }
type rawScrapeConfig struct { MaxAttempts int `toml:"max_attempts"`; RetryBackoff string `toml:"retry_backoff"` }
type rawFeedConfig struct { ID, URL, Title, Description, Link, Interval string; Rule rawRule }
type rawRule struct { Item string; Title rawField; Link rawField; Description rawField; PubDate rawField `toml:"pub_date"` }
type rawField struct { Selector, Attr, Format string; Absolute bool }

type CompiledRule struct { Item Selector; Title CompiledField; Link CompiledField; Description *CompiledField; PubDate *CompiledField }
type CompiledField struct { Selector Selector; Attr string; Absolute bool; Format string }

var feedIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func Load(path string) (*Config, error) {
	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil { return nil, err }
	cfg := applyDefaults(raw)
	var errs []error
	if err := validateScrape(&cfg); err != nil { errs = append(errs, err) }
	seen := map[string]bool{}
	for i := range raw.Feeds {
		feed, err := compileFeed(raw.Feeds[i])
		if err != nil { errs = append(errs, err); continue }
		if seen[feed.ID] { errs = append(errs, fmt.Errorf("duplicate feed id %q", feed.ID)) }
		seen[feed.ID] = true
		cfg.Feeds = append(cfg.Feeds, feed)
	}
	cfg.AI.APIKey = expandEnvLiteral(cfg.AI.APIKey)
	cfg.AI.BaseURL = expandEnvLiteral(cfg.AI.BaseURL)
	cfg.AI.Model = expandEnvLiteral(cfg.AI.Model)
	if len(errs) > 0 { return nil, errors.Join(errs...) }
	return &cfg, nil
}

func LoadRule(path string) (CompiledRule, error) {
	data, err := os.ReadFile(path)
	if err != nil { return CompiledRule{}, err }
	return ParseRuleTOML(data)
}

func ParseRuleTOML(data []byte) (CompiledRule, error) {
	var raw struct { Rule rawRule }
	if _, err := toml.Decode(string(data), &raw); err != nil { return CompiledRule{}, err }
	return compileRule(raw.Rule)
}
```

Complete the file with these functions:

```go
func applyDefaults(raw rawConfig) Config {
	cfg := Config{Server: ServerConfig{Listen: ":8080", CacheDir: "./cache", UserAgent: "rssify/dev", FetchTimeout: 15 * time.Second}, Scrape: ScrapeConfig{MaxAttempts: 3, RetryBackoff: 30 * time.Second}, AI: raw.AI}
	if raw.Server.Listen != "" { cfg.Server.Listen = raw.Server.Listen }
	if raw.Server.CacheDir != "" { cfg.Server.CacheDir = raw.Server.CacheDir }
	if raw.Server.UserAgent != "" { cfg.Server.UserAgent = raw.Server.UserAgent }
	if raw.Server.FetchTimeout != "" { if d, err := time.ParseDuration(raw.Server.FetchTimeout); err == nil { cfg.Server.FetchTimeout = d } }
	if raw.Scrape.MaxAttempts != 0 { cfg.Scrape.MaxAttempts = raw.Scrape.MaxAttempts }
	if raw.Scrape.RetryBackoff != "" { if d, err := time.ParseDuration(raw.Scrape.RetryBackoff); err == nil { cfg.Scrape.RetryBackoff = d } }
	return cfg
}

func validateScrape(cfg *Config) error {
	var errs []error
	if cfg.Scrape.MaxAttempts < 1 { errs = append(errs, fmt.Errorf("scrape.max_attempts must be >= 1")) }
	if cfg.Scrape.RetryBackoff < time.Second { errs = append(errs, fmt.Errorf("scrape.retry_backoff must be >= 1s")) }
	return errors.Join(errs...)
}

func compileFeed(raw rawFeedConfig) (FeedConfig, error) {
	var errs []error
	feed := FeedConfig{ID: raw.ID, URL: raw.URL, Title: raw.Title, Description: raw.Description, Link: raw.Link}
	if !feedIDPattern.MatchString(raw.ID) { errs = append(errs, fmt.Errorf("feed %s id must match %s", raw.ID, feedIDPattern.String())) }
	if raw.Title == "" { errs = append(errs, fmt.Errorf("feed %s title is required", raw.ID)) }
	if u, err := url.ParseRequestURI(raw.URL); err != nil || u.Scheme == "" || u.Host == "" { errs = append(errs, fmt.Errorf("feed %s url must be absolute", raw.ID)) }
	interval, err := time.ParseDuration(raw.Interval)
	if err != nil || interval < time.Minute { errs = append(errs, fmt.Errorf("feed %s interval must be >= 1m", raw.ID)) } else { feed.Interval = interval }
	if feed.Description == "" { feed.Description = feed.Title }
	if feed.Link == "" { feed.Link = feed.URL }
	rule, err := compileRule(raw.Rule)
	if err != nil { errs = append(errs, fmt.Errorf("feed %s rule: %w", raw.ID, err)) } else { feed.Rule = rule }
	return feed, errors.Join(errs...)
}

func compileRule(raw rawRule) (CompiledRule, error) {
	var errs []error
	item, err := CompileSelector(raw.Item); if err != nil { errs = append(errs, fmt.Errorf("item selector: %w", err)) }
	title, err := compileRequiredField("title", raw.Title); if err != nil { errs = append(errs, err) }
	link, err := compileRequiredField("link", raw.Link); if err != nil { errs = append(errs, err) }
	var desc *CompiledField
	if raw.Description.Selector != "" { field, err := compileField("description", raw.Description); if err != nil { errs = append(errs, err) } else { desc = &field } }
	var pub *CompiledField
	if raw.PubDate.Selector != "" { field, err := compileField("pub_date", raw.PubDate); if err != nil { errs = append(errs, err) } else { pub = &field } }
	return CompiledRule{Item: item, Title: title, Link: link, Description: desc, PubDate: pub}, errors.Join(errs...)
}

func compileRequiredField(name string, raw rawField) (CompiledField, error) {
	if strings.TrimSpace(raw.Selector) == "" { return CompiledField{}, fmt.Errorf("%s.selector is required", name) }
	return compileField(name, raw)
}

func compileField(name string, raw rawField) (CompiledField, error) {
	if name == "pub_date" && raw.Absolute { return CompiledField{}, fmt.Errorf("pub_date.absolute is forbidden") }
	if name != "pub_date" && raw.Format != "" { return CompiledField{}, fmt.Errorf("%s.format is only valid for pub_date", name) }
	if raw.Format != "" { if _, err := time.Parse(raw.Format, time.Now().UTC().Format(raw.Format)); err != nil { return CompiledField{}, fmt.Errorf("pub_date.format is invalid: %w", err) } }
	sel, err := CompileSelector(raw.Selector); if err != nil { return CompiledField{}, fmt.Errorf("%s.selector: %w", name, err) }
	return CompiledField{Selector: sel, Attr: raw.Attr, Absolute: raw.Absolute, Format: raw.Format}, nil
}

func expandEnvLiteral(v string) string { if strings.HasPrefix(v, "$") && len(v) > 1 { return os.Getenv(strings.TrimPrefix(v, "$")) }; return v }
```

- [ ] **Step 6: Run tests and commit**

Run:

```bash
go test ./internal/config
```

Expected: PASS.

Commit:

```bash
git add go.mod go.sum internal/config
git commit -m "feat: load and validate config"
```

---

### Task 3: Fetch Package

**Files:**
- Create: `internal/fetch/fetch.go`
- Test: `internal/fetch/fetch_test.go`

- [ ] **Step 1: Write fetch tests**

Create `internal/fetch/fetch_test.go`:

```go
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
	var userAgent string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.UserAgent()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	c := NewClient("rssify-test", time.Second)
	body, finalURL, err := c.Get(context.Background(), ts.URL)
	if err != nil { t.Fatal(err) }
	if string(body) != "hello" { t.Fatalf("body = %q", body) }
	if finalURL.String() != ts.URL { t.Fatalf("finalURL = %s", finalURL) }
	if userAgent != "rssify-test" { t.Fatalf("user agent = %q", userAgent) }
}

func TestClientGetTreatsServerErrorAsRetryable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad", http.StatusBadGateway) }))
	defer ts.Close()
	err := getErr(t, ts.URL)
	if !IsRetryable(err) { t.Fatalf("expected retryable error: %v", err) }
}

func TestClientGetTreatsNotFoundAsNonRetryable(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()
	err := getErr(t, ts.URL)
	if IsRetryable(err) { t.Fatalf("expected non-retryable error: %v", err) }
}

func getErr(t *testing.T, url string) error {
	t.Helper()
	c := NewClient("rssify-test", time.Second)
	_, _, err := c.Get(context.Background(), url)
	if err == nil { t.Fatal("expected error") }
	return err
}

func TestIsRetryableFalseForUnknownError(t *testing.T) {
	if IsRetryable(errors.New("x")) { t.Fatal("plain error should not be retryable") }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/fetch
```

Expected: FAIL because package code is missing.

- [ ] **Step 3: Implement fetch**

Create `internal/fetch/fetch.go`:

```go
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	http      *http.Client
	userAgent string
}

type HTTPError struct { StatusCode int; URL string }

func (e *HTTPError) Error() string { return fmt.Sprintf("GET %s returned HTTP %d", e.URL, e.StatusCode) }

func NewClient(userAgent string, timeout time.Duration) *Client {
	return &Client{http: &http.Client{Timeout: timeout}, userAgent: userAgent}
}

func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil { return nil, nil, err }
	if c.userAgent != "" { req.Header.Set("User-Agent", c.userAgent) }
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.http.Do(req)
	if err != nil { return nil, nil, err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return nil, resp.Request.URL, &HTTPError{StatusCode: resp.StatusCode, URL: resp.Request.URL.String()} }
	body, err := io.ReadAll(resp.Body)
	if err != nil { return nil, nil, err }
	return body, resp.Request.URL, nil
}

func IsRetryable(err error) bool {
	if err == nil { return false }
	var httpErr *HTTPError
	if errors.As(err, &httpErr) { return httpErr.StatusCode == 500 || httpErr.StatusCode == 502 || httpErr.StatusCode == 503 || httpErr.StatusCode == 504 }
	var netErr net.Error
	return errors.As(err, &netErr)
}
```

- [ ] **Step 4: Run tests and commit**

Run:

```bash
go test ./internal/fetch
```

Expected: PASS.

Commit:

```bash
git add internal/fetch
git commit -m "feat: add html fetch client"
```

---

### Task 4: Extract Package

**Files:**
- Create: `internal/extract/extract.go`
- Test: `internal/extract/extract_test.go`

- [ ] **Step 1: Write extraction tests**

Create `internal/extract/extract_test.go`:

```go
package extract

import (
	"net/url"
	"testing"

	"github.com/LeeTeng2001/rssify/internal/config"
)

func TestRunExtractsCSSFieldsAndAbsoluteLinks(t *testing.T) {
	rule := compileRuleForTest(t, ".item", ".title", "a", "href", true, ".date", "Jan 2, 2006")
	body := []byte(`<html><body><div class="item"><a class="title" href="/p/1">One</a><span class="date">May 25, 2026</span></div></body></html>`)
	base, _ := url.Parse("https://example.com/list")
	items, warnings, err := Run(body, rule, base)
	if err != nil { t.Fatal(err) }
	if len(warnings) != 0 { t.Fatalf("warnings = %+v", warnings) }
	if len(items) != 1 { t.Fatalf("items = %+v", items) }
	if items[0].Title != "One" || items[0].Link != "https://example.com/p/1" || items[0].PubDate != "Mon, 25 May 2026 00:00:00 +0000" {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

func TestRunReturnsWarningForInvalidDate(t *testing.T) {
	rule := compileRuleForTest(t, ".item", ".title", "a", "href", false, ".date", "Jan 2, 2006")
	body := []byte(`<div class="item"><a class="title" href="https://e.test/1">One</a><span class="date">today</span></div>`)
	base, _ := url.Parse("https://e.test/")
	items, warnings, err := Run(body, rule, base)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 || items[0].PubDate != "" { t.Fatalf("item = %+v", items) }
	if len(warnings) != 1 || warnings[0].Field != "pub_date" { t.Fatalf("warnings = %+v", warnings) }
}

func compileRuleForTest(t *testing.T, item, title, link, attr string, absolute bool, date, format string) config.CompiledRule {
	t.Helper()
	itemSel, err := config.CompileSelector(item); if err != nil { t.Fatal(err) }
	titleSel, err := config.CompileSelector(title); if err != nil { t.Fatal(err) }
	linkSel, err := config.CompileSelector(link); if err != nil { t.Fatal(err) }
	dateSel, err := config.CompileSelector(date); if err != nil { t.Fatal(err) }
	pub := config.CompiledField{Selector: dateSel, Format: format}
	return config.CompiledRule{Item: itemSel, Title: config.CompiledField{Selector: titleSel}, Link: config.CompiledField{Selector: linkSel, Attr: attr, Absolute: absolute}, PubDate: &pub}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/extract
```

Expected: FAIL because `Run`, `Item`, and `Warning` are undefined.

- [ ] **Step 3: Implement extraction**

Create `internal/extract/extract.go`:

```go
package extract

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type Item struct { Title, Link, Description, PubDate string }
type Warning struct { ItemIndex int; Field, Message string }

func Run(data []byte, rule config.CompiledRule, baseURL *url.URL) ([]Item, []Warning, error) {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil { return nil, nil, err }
	itemNodes := rule.Item.Find(root)
	items := make([]Item, 0, len(itemNodes))
	var warnings []Warning
	for i, node := range itemNodes {
		item := Item{}
		item.Title, warnings = extractField(warnings, i, "title", node, rule.Title, baseURL)
		item.Link, warnings = extractField(warnings, i, "link", node, rule.Link, baseURL)
		if rule.Description != nil { item.Description, warnings = extractField(warnings, i, "description", node, *rule.Description, baseURL) }
		if rule.PubDate != nil {
			raw, next := extractField(warnings, i, "pub_date", node, *rule.PubDate, baseURL)
			warnings = next
			if raw != "" && rule.PubDate.Format != "" {
				parsed, err := time.Parse(rule.PubDate.Format, raw)
				if err != nil { warnings = append(warnings, Warning{i, "pub_date", fmt.Sprintf("parse %q: %v", raw, err)}) } else { item.PubDate = parsed.UTC().Format(time.RFC1123Z) }
			} else { item.PubDate = raw }
		}
		items = append(items, item)
	}
	return items, warnings, nil
}

func extractField(warnings []Warning, index int, name string, node *html.Node, field config.CompiledField, baseURL *url.URL) (string, []Warning) {
	matches := field.Selector.Find(node)
	if len(matches) == 0 { return "", append(warnings, Warning{index, name, "selector matched nothing"}) }
	value := nodeText(matches[0])
	if field.Attr != "" { value = attr(matches[0], field.Attr) }
	value = strings.TrimSpace(value)
	if field.Absolute && value != "" {
		ref, err := url.Parse(value)
		if err != nil { return "", append(warnings, Warning{index, name, fmt.Sprintf("invalid URL %q", value)}) }
		value = baseURL.ResolveReference(ref).String()
	}
	return value, warnings
}

func nodeText(node *html.Node) string {
	doc := goquery.NewDocumentFromNode(node)
	return doc.Text()
}

func attr(node *html.Node, key string) string {
	for _, a := range node.Attr { if a.Key == key { return a.Val } }
	return ""
}
```

- [ ] **Step 4: Run tests and commit**

Run:

```bash
go test ./internal/extract
```

Expected: PASS.

Commit:

```bash
git add internal/extract
git commit -m "feat: extract rss items from html"
```

---

### Task 5: Render Package

**Files:**
- Create: `internal/render/render.go`
- Test: `internal/render/render_test.go`

- [ ] **Step 1: Write render tests**

Create `internal/render/render_test.go`:

```go
package render

import (
	"strings"
	"testing"
	"time"

	"github.com/LeeTeng2001/rssify/internal/extract"
)

func TestRSSRendersEscapedRSS2(t *testing.T) {
	generated := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	out := string(RSS(FeedMeta{Title: "T & C", Link: "https://example.com", Description: "D", SelfURL: "https://feeds.test/feed/x.xml"}, []extract.Item{{Title: "A & B", Link: "https://example.com/a?x=1&y=2", Description: "desc", PubDate: "Mon, 25 May 2026 00:00:00 +0000"}}, generated))
	for _, want := range []string{`<?xml version="1.0" encoding="UTF-8"?>`, `<rss version="2.0"`, `xmlns:atom="http://www.w3.org/2005/Atom"`, `T &amp; C`, `<guid isPermaLink="true">https://example.com/a?x=1&amp;y=2</guid>`, `<pubDate>Mon, 25 May 2026 00:00:00 +0000</pubDate>`} {
		if !strings.Contains(out, want) { t.Fatalf("output missing %q:\n%s", want, out) }
	}
}

func TestRSSOmitsEmptyOptionalItemFields(t *testing.T) {
	out := string(RSS(FeedMeta{Title: "T", Link: "https://e.test", Description: "D"}, []extract.Item{{Title: "A", Link: "https://e.test/a"}}, time.Unix(0, 0).UTC()))
	if strings.Contains(out, "<description></description>") || strings.Contains(out, "<pubDate></pubDate>") {
		t.Fatalf("empty optional fields should be omitted:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/render
```

Expected: FAIL because `RSS` and `FeedMeta` are undefined.

- [ ] **Step 3: Implement render**

Create `internal/render/render.go`:

```go
package render

import (
	"bytes"
	"encoding/xml"
	"time"

	"github.com/LeeTeng2001/rssify/internal/extract"
)

type FeedMeta struct { Title, Link, Description, SelfURL string }

type rss struct { XMLName xml.Name `xml:"rss"`; Version string `xml:"version,attr"`; Atom string `xml:"xmlns:atom,attr"`; Channel channel `xml:"channel"` }
type channel struct { Title string `xml:"title"`; Link string `xml:"link"`; Description string `xml:"description"`; AtomLink atomLink `xml:"atom:link"`; LastBuildDate string `xml:"lastBuildDate"`; Generator string `xml:"generator"`; Items []item `xml:"item"` }
type atomLink struct { Rel string `xml:"rel,attr"`; Href string `xml:"href,attr,omitempty"`; Type string `xml:"type,attr"` }
type item struct { Title string `xml:"title"`; Link string `xml:"link"`; GUID guid `xml:"guid"`; Description *string `xml:"description,omitempty"`; PubDate *string `xml:"pubDate,omitempty"` }
type guid struct { IsPermaLink string `xml:"isPermaLink,attr"`; Value string `xml:",chardata"` }

func RSS(meta FeedMeta, items []extract.Item, generated time.Time) []byte {
	out := rss{Version: "2.0", Atom: "http://www.w3.org/2005/Atom", Channel: channel{Title: meta.Title, Link: meta.Link, Description: meta.Description, AtomLink: atomLink{Rel: "self", Href: meta.SelfURL, Type: "application/rss+xml"}, LastBuildDate: generated.UTC().Format(time.RFC1123Z), Generator: "rssify"}}
	for _, in := range items {
		it := item{Title: in.Title, Link: in.Link, GUID: guid{IsPermaLink: "true", Value: in.Link}}
		if in.Description != "" { v := in.Description; it.Description = &v }
		if in.PubDate != "" { v := in.PubDate; it.PubDate = &v }
		out.Channel.Items = append(out.Channel.Items, it)
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	_ = enc.Encode(out)
	buf.WriteByte('\n')
	return buf.Bytes()
}
```

- [ ] **Step 4: Run tests and commit**

Run:

```bash
go test ./internal/render
```

Expected: PASS.

Commit:

```bash
git add internal/render
git commit -m "feat: render rss xml"
```

---

### Task 6: Cache Package

**Files:**
- Create: `internal/cache/cache.go`
- Test: `internal/cache/cache_test.go`

- [ ] **Step 1: Write cache tests**

Create `internal/cache/cache_test.go`:

```go
package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPutGetAndDiskWrite(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil { t.Fatal(err) }
	if err := c.Put("hn", []byte("<rss/>")); err != nil { t.Fatal(err) }
	got, ok := c.Get("hn")
	if !ok || string(got) != "<rss/>" { t.Fatalf("Get = %q, %v", got, ok) }
	disk, err := os.ReadFile(filepath.Join(dir, "hn.xml"))
	if err != nil { t.Fatal(err) }
	if string(disk) != "<rss/>" { t.Fatalf("disk = %q", disk) }
}

func TestLoadExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hn.xml"), []byte("old"), 0o600); err != nil { t.Fatal(err) }
	c, err := New(dir)
	if err != nil { t.Fatal(err) }
	if err := c.LoadExisting([]string{"hn"}); err != nil { t.Fatal(err) }
	got, ok := c.Get("hn")
	if !ok || string(got) != "old" { t.Fatalf("Get = %q, %v", got, ok) }
}

func TestConcurrentGetPut(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil { t.Fatal(err) }
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = c.Put("hn", []byte("x")); _, _ = c.Get("hn") }()
	}
	wg.Wait()
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/cache
```

Expected: FAIL because `New` is undefined.

- [ ] **Step 3: Implement cache**

Create `internal/cache/cache.go`:

```go
package cache

import (
	"os"
	"path/filepath"
	"sync"
)

type Cache struct { dir string; mu sync.RWMutex; data map[string][]byte }

func New(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil { return nil, err }
	return &Cache{dir: dir, data: map[string][]byte{}}, nil
}

func (c *Cache) Get(id string) ([]byte, bool) {
	c.mu.RLock(); defer c.mu.RUnlock()
	b, ok := c.data[id]
	if !ok { return nil, false }
	out := append([]byte(nil), b...)
	return out, true
}

func (c *Cache) Put(id string, xml []byte) error {
	path := filepath.Join(c.dir, id+".xml")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, xml, 0o600); err != nil { return err }
	if err := os.Rename(tmp, path); err != nil { return err }
	c.mu.Lock(); defer c.mu.Unlock()
	c.data[id] = append([]byte(nil), xml...)
	return nil
}

func (c *Cache) LoadExisting(ids []string) error {
	for _, id := range ids {
		b, err := os.ReadFile(filepath.Join(c.dir, id+".xml"))
		if os.IsNotExist(err) { continue }
		if err != nil { return err }
		c.mu.Lock(); c.data[id] = b; c.mu.Unlock()
	}
	return nil
}
```

- [ ] **Step 4: Run tests with race detector and commit**

Run:

```bash
go test -race ./internal/cache
```

Expected: PASS.

Commit:

```bash
git add internal/cache
git commit -m "feat: add rss cache"
```

---

### Task 7: HTTP Server Package

**Files:**
- Create: `internal/server/server.go`
- Test: `internal/server/server_test.go`

- [ ] **Step 1: Write server tests**

Create `internal/server/server_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeeTeng2001/rssify/internal/cache"
)

func TestFeedHitMissAndUnknown(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil { t.Fatal(err) }
	if err := c.Put("hn", []byte("<rss/>")); err != nil { t.Fatal(err) }
	h := New(c, []string{"hn", "empty"})

	assertStatus(t, h, "/feed/hn.xml", http.StatusOK, "application/rss+xml; charset=utf-8")
	assertStatus(t, h, "/feed/empty.xml", http.StatusServiceUnavailable, "text/plain; charset=utf-8")
	assertStatus(t, h, "/feed/nope.xml", http.StatusNotFound, "text/plain; charset=utf-8")
}

func assertStatus(t *testing.T, h http.Handler, path string, want int, contentType string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != want { t.Fatalf("%s status = %d", path, w.Code) }
	if got := w.Header().Get("Content-Type"); got != contentType { t.Fatalf("%s content-type = %q", path, got) }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/server
```

Expected: FAIL because `New` is undefined.

- [ ] **Step 3: Implement server**

Create `internal/server/server.go`:

```go
package server

import (
	"net/http"
	"strings"

	"github.com/LeeTeng2001/rssify/internal/cache"
)

func New(c *cache.Cache, feedIDs []string) http.Handler {
	known := map[string]bool{}
	for _, id := range feedIDs { known[id] = true }
	mux := http.NewServeMux()
	mux.HandleFunc("/feed/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
		id := strings.TrimPrefix(r.URL.Path, "/feed/")
		id = strings.TrimSuffix(id, ".xml")
		if id == "" || !known[id] { http.NotFound(w, r); return }
		b, ok := c.Get(id)
		if !ok { http.Error(w, "feed has not been scraped yet, try again shortly", http.StatusServiceUnavailable); return }
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(b)
	})
	return mux
}
```

- [ ] **Step 4: Run tests and commit**

Run:

```bash
go test ./internal/server
```

Expected: PASS.

Commit:

```bash
git add internal/server
git commit -m "feat: serve cached feeds"
```

---

### Task 8: Scheduler Package

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Write scheduler tests using fakes**

Create `internal/scheduler/scheduler_test.go`:

```go
package scheduler

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LeeTeng2001/rssify/internal/cache"
	"github.com/LeeTeng2001/rssify/internal/config"
)

func TestScrapeKeepsOldCacheOnFetchFailure(t *testing.T) {
	c, _ := cache.New(t.TempDir())
	_ = c.Put("hn", []byte("old"))
	f := FetchFunc(func(context.Context, string) ([]byte, *url.URL, error) { return nil, nil, errors.New("down") })
	s := New([]config.FeedConfig{{ID: "hn", URL: "https://e.test/", Title: "HN", Description: "HN", Link: "https://e.test/"}}, c, f, Options{MaxAttempts: 1, RetryBackoff: time.Millisecond})
	s.scrape(context.Background(), s.feeds[0])
	got, _ := c.Get("hn")
	if string(got) != "old" { t.Fatalf("cache overwritten: %q", got) }
}

func TestScrapeWritesRSSOnSuccess(t *testing.T) {
	itemSel, _ := config.CompileSelector(".item")
	titleSel, _ := config.CompileSelector(".title")
	linkSel, _ := config.CompileSelector("a")
	rule := config.CompiledRule{Item: itemSel, Title: config.CompiledField{Selector: titleSel}, Link: config.CompiledField{Selector: linkSel, Attr: "href", Absolute: true}}
	c, _ := cache.New(t.TempDir())
	f := FetchFunc(func(context.Context, string) ([]byte, *url.URL, error) { u, _ := url.Parse("https://e.test/list"); return []byte(`<div class="item"><a class="title" href="/1">One</a></div>`), u, nil })
	feed := config.FeedConfig{ID: "hn", URL: "https://e.test/list", Title: "HN", Description: "HN", Link: "https://e.test/", Rule: rule}
	s := New([]config.FeedConfig{feed}, c, f, Options{MaxAttempts: 1, RetryBackoff: time.Millisecond, SelfURL: func(string) string { return "https://rss.test/feed/hn.xml" }})
	s.scrape(context.Background(), feed)
	got, ok := c.Get("hn")
	if !ok { t.Fatal("expected cached RSS") }
	if !strings.Contains(string(got), "<rss") || !strings.Contains(string(got), "One") { t.Fatalf("rss = %s", got) }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/scheduler
```

Expected: FAIL because `New`, `FetchFunc`, and scheduler internals are undefined.

- [ ] **Step 3: Implement scheduler**

Create `internal/scheduler/scheduler.go`:

```go
package scheduler

import (
	"context"
	"log/slog"
	"math/rand"
	"net/url"
	"time"

	"github.com/LeeTeng2001/rssify/internal/cache"
	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/LeeTeng2001/rssify/internal/extract"
	"github.com/LeeTeng2001/rssify/internal/fetch"
	"github.com/LeeTeng2001/rssify/internal/render"
)

type Fetcher interface { Get(context.Context, string) ([]byte, *url.URL, error) }
type FetchFunc func(context.Context, string) ([]byte, *url.URL, error)
func (f FetchFunc) Get(ctx context.Context, rawURL string) ([]byte, *url.URL, error) { return f(ctx, rawURL) }

type Options struct { MaxAttempts int; RetryBackoff time.Duration; Logger *slog.Logger; SelfURL func(feedID string) string }
type Scheduler struct { feeds []config.FeedConfig; cache *cache.Cache; fetcher Fetcher; opts Options }

func New(feeds []config.FeedConfig, c *cache.Cache, f Fetcher, opts Options) *Scheduler {
	if opts.MaxAttempts < 1 { opts.MaxAttempts = 1 }
	if opts.RetryBackoff <= 0 { opts.RetryBackoff = 30 * time.Second }
	if opts.Logger == nil { opts.Logger = slog.Default() }
	if opts.SelfURL == nil { opts.SelfURL = func(id string) string { return "" } }
	return &Scheduler{feeds: feeds, cache: c, fetcher: f, opts: opts}
}

func (s *Scheduler) Start(ctx context.Context) {
	for _, feed := range s.feeds { feed := feed; go s.loop(ctx, feed) }
}

func (s *Scheduler) loop(ctx context.Context, feed config.FeedConfig) {
	jitterMax := minDuration(30*time.Second, feed.Interval/4)
	if jitterMax > 0 { if !sleepCtx(ctx, time.Duration(rand.Int63n(int64(jitterMax)))) { return } }
	s.scrape(ctx, feed)
	ticker := time.NewTicker(feed.Interval)
	defer ticker.Stop()
	for {
		select { case <-ctx.Done(): return; case <-ticker.C: s.scrape(ctx, feed) }
	}
}

func (s *Scheduler) scrape(ctx context.Context, feed config.FeedConfig) {
	start := time.Now()
	var body []byte
	var finalURL *url.URL
	var err error
	attempt := 1
	for attempt = 1; attempt <= s.opts.MaxAttempts; attempt++ {
		body, finalURL, err = s.fetcher.Get(ctx, feed.URL)
		if err == nil { break }
		if !fetch.IsRetryable(err) || attempt == s.opts.MaxAttempts { break }
		s.opts.Logger.Warn("fetch failed", "feed", feed.ID, "attempt", attempt, "max_attempts", s.opts.MaxAttempts, "error", err)
		if !sleepCtx(ctx, s.opts.RetryBackoff) { return }
	}
	if err != nil { s.opts.Logger.Warn("scrape failed, retaining cache", "feed", feed.ID, "attempts", attempt, "error", err); return }
	items, warnings, err := extract.Run(body, feed.Rule, finalURL)
	for _, w := range warnings { s.opts.Logger.Debug("extract warning", "feed", feed.ID, "item", w.ItemIndex, "field", w.Field, "message", w.Message) }
	if err != nil { s.opts.Logger.Warn("extract failed, retaining cache", "feed", feed.ID, "error", err); return }
	if len(items) == 0 { s.opts.Logger.Warn("zero items, retaining cache", "feed", feed.ID); return }
	xml := render.RSS(render.FeedMeta{Title: feed.Title, Link: feed.Link, Description: feed.Description, SelfURL: s.opts.SelfURL(feed.ID)}, items, time.Now().UTC())
	if err := s.cache.Put(feed.ID, xml); err != nil { s.opts.Logger.Error("cache write failed", "feed", feed.ID, "error", err); return }
	s.opts.Logger.Info("scrape ok", "feed", feed.ID, "items", len(items), "attempts", attempt, "duration_ms", time.Since(start).Milliseconds())
}

func sleepCtx(ctx context.Context, d time.Duration) bool { timer := time.NewTimer(d); defer timer.Stop(); select { case <-ctx.Done(): return false; case <-timer.C: return true } }
func minDuration(a, b time.Duration) time.Duration { if a < b { return a }; return b }
```

- [ ] **Step 4: Run tests and commit**

Run:

```bash
go test ./internal/scheduler
```

Expected: PASS.

Commit:

```bash
git add internal/scheduler
git commit -m "feat: schedule feed scrapes"
```

---

### Task 9: Serve Command Wiring

**Files:**
- Create: `cmd/serve/serve.go`
- Modify: `main.go`

- [ ] **Step 1: Add serve command implementation**

Create `cmd/serve/serve.go`:

```go
package serve

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LeeTeng2001/rssify/internal/cache"
	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/LeeTeng2001/rssify/internal/fetch"
	"github.com/LeeTeng2001/rssify/internal/logging"
	"github.com/LeeTeng2001/rssify/internal/scheduler"
	"github.com/LeeTeng2001/rssify/internal/server"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	return &cli.Command{Name: "serve", Usage: "serve configured RSS feeds", Action: run}
}

func run(ctx context.Context, c *cli.Command) error {
	logger, err := logging.New(c.String("log-level"), c.String("log-format"), os.Stderr)
	if err != nil { return err }
	cfg, err := config.Load(c.String("config"))
	if err != nil { return err }
	xmlCache, err := cache.New(cfg.Server.CacheDir)
	if err != nil { return err }
	ids := make([]string, 0, len(cfg.Feeds))
	for _, f := range cfg.Feeds { ids = append(ids, f.ID) }
	if err := xmlCache.LoadExisting(ids); err != nil { return err }

	root, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	fetcher := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)
	s := scheduler.New(cfg.Feeds, xmlCache, fetcher, scheduler.Options{MaxAttempts: cfg.Scrape.MaxAttempts, RetryBackoff: cfg.Scrape.RetryBackoff, Logger: logger, SelfURL: func(id string) string { return fmt.Sprintf("/feed/%s.xml", id) }})
	s.Start(root)

	httpServer := &http.Server{Addr: cfg.Server.Listen, Handler: server.New(xmlCache, ids)}
	go func() { <-root.Done(); shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second); defer cancel(); _ = httpServer.Shutdown(shutdownCtx) }()
	logger.Info("serving", "listen", cfg.Server.Listen)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed { return err }
	return nil
}
```

- [ ] **Step 2: Register serve command**

Modify `main.go` imports to include `cmd/serve`, then register it:

```go
import (
	"context"
	"fmt"
	"os"

	"github.com/LeeTeng2001/rssify/cmd/serve"
	"github.com/LeeTeng2001/rssify/cmd/version"
	"github.com/urfave/cli/v3"
)
```

Change commands list:

```go
Commands: []*cli.Command{
	serve.Command(),
	version.Command(os.Stdout),
},
```

- [ ] **Step 3: Compile and commit**

Run:

```bash
go test ./...
go run . serve --help
```

Expected: tests PASS; help output includes `serve` usage and global flags.

Commit:

```bash
git add main.go cmd/serve
git commit -m "feat: wire serve command"
```

---

### Task 10: Probe Verify Mode

**Files:**
- Create: `cmd/probe/probe.go`
- Modify: `main.go`
- Test: `cmd/probe/probe_test.go`

- [ ] **Step 1: Write output helper tests**

Create `cmd/probe/probe_test.go`:

```go
package probe

import (
	"bytes"
	"strings"
	"testing"

	"github.com/LeeTeng2001/rssify/internal/extract"
)

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	printTable(&buf, []extract.Item{{Title: "One", Link: "https://e.test/1", PubDate: "Mon"}}, 10)
	out := buf.String()
	for _, want := range []string{"#", "Title", "One", "https://e.test/1", "Mon"} {
		if !strings.Contains(out, want) { t.Fatalf("output missing %q:\n%s", want, out) }
	}
}

func TestLimitItems(t *testing.T) {
	items := []extract.Item{{Title: "1"}, {Title: "2"}}
	if got := limitItems(items, 1); len(got) != 1 || got[0].Title != "1" { t.Fatalf("limit = %+v", got) }
	if got := limitItems(items, 0); len(got) != 2 { t.Fatalf("limit all = %+v", got) }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./cmd/probe
```

Expected: FAIL because probe package is missing.

- [ ] **Step 3: Implement verify mode**

Create `cmd/probe/probe.go`:

```go
package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/LeeTeng2001/rssify/internal/extract"
	"github.com/LeeTeng2001/rssify/internal/fetch"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	return &cli.Command{Name: "probe", Usage: "verify or author extraction rules", Flags: []cli.Flag{&cli.StringFlag{Name: "rule", Usage: "standalone TOML rule file for URL mode"}, &cli.BoolFlag{Name: "suggest", Usage: "use AI to suggest a rule"}, &cli.IntFlag{Name: "limit", Value: 10, Usage: "max items to print, 0 for all"}, &cli.BoolFlag{Name: "json", Usage: "print JSON"}, &cli.IntFlag{Name: "html-bytes", Value: 30720, Usage: "bytes of HTML to send to AI"}}, Action: run}
}

func run(ctx context.Context, c *cli.Command) error {
	if c.Args().Len() != 1 { return errors.New("usage: rssify probe <feed-id|url>") }
	if c.Bool("suggest") { return runSuggest(ctx, c, os.Stdout, os.Stderr) }
	cfg, err := config.Load(c.String("config"))
	if err != nil { return err }
	arg := c.Args().First()
	feed, ok := findFeed(cfg, arg)
	if !ok { return runURLRuleMode(ctx, c, arg, cfg) }
	client := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)
	body, finalURL, err := client.Get(ctx, feed.URL)
	if err != nil { return err }
	return outputExtracted(os.Stdout, os.Stderr, c.Bool("json"), c.Int("limit"), body, finalURL, feed.Rule)
}

func findFeed(cfg *config.Config, id string) (config.FeedConfig, bool) { for _, f := range cfg.Feeds { if f.ID == id { return f, true } }; return config.FeedConfig{}, false }

func runURLRuleMode(ctx context.Context, c *cli.Command, rawURL string, cfg *config.Config) error {
	rulePath := c.String("rule")
	if rulePath == "" { return fmt.Errorf("feed %q not found; to probe an arbitrary URL, pass --rule or --suggest", rawURL) }
	rule, err := config.LoadRule(rulePath)
	if err != nil { return err }
	client := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)
	body, finalURL, err := client.Get(ctx, rawURL)
	if err != nil { return err }
	return outputExtracted(os.Stdout, os.Stderr, c.Bool("json"), c.Int("limit"), body, finalURL, rule)
}

func outputExtracted(out, errOut io.Writer, asJSON bool, limit int, body []byte, finalURL *url.URL, rule config.CompiledRule) error {
	items, warnings, err := extract.Run(body, rule, finalURL)
	if err != nil { return err }
	for _, w := range warnings { fmt.Fprintf(errOut, "warning: item=%d field=%s %s\n", w.ItemIndex, w.Field, w.Message) }
	items = limitItems(items, limit)
	if asJSON { return json.NewEncoder(out).Encode(items) }
	printTable(out, items, limit)
	return nil
}

func limitItems(items []extract.Item, limit int) []extract.Item { if limit == 0 || limit >= len(items) { return items }; return items[:limit] }

func printTable(out io.Writer, items []extract.Item, limit int) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tTitle\tLink\tPubDate")
	for i, item := range limitItems(items, limit) { fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", i+1, truncate(item.Title, 80), item.Link, item.PubDate) }
	_ = tw.Flush()
}

func truncate(s string, n int) string { s = strings.TrimSpace(s); if len(s) <= n { return s }; if n <= 1 { return s[:n] }; return s[:n-1] + "…" }
```

- [ ] **Step 4: Register probe command**

Modify `main.go` imports:

```go
"github.com/LeeTeng2001/rssify/cmd/probe"
```

Add to command list:

```go
probe.Command(),
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
go test ./...
go run . probe --help
```

Expected: tests PASS; help output includes `--limit`, `--json`, `--suggest`.

Commit:

```bash
git add main.go cmd/probe
git commit -m "feat: add probe verify mode"
```

---

### Task 11: AI Client and Probe Suggest Loop

**Files:**
- Create: `internal/ai/ai.go`
- Modify: `cmd/probe/probe.go`
- Test: `internal/ai/ai_test.go`
- Test: `cmd/probe/probe_test.go`

- [ ] **Step 1: Add OpenAI SDK**

Run:

```bash
go get github.com/openai/openai-go
```

Expected: dependency added.

- [ ] **Step 2: Write AI client test with local HTTP server**

Create `internal/ai/ai_test.go`:

```go
package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteUsesOpenAICompatibleEndpoint(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" { t.Fatalf("path = %s", r.URL.Path) }
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"```toml\n[feed.rule]\nitem = \".item\"\n```"}}]}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "secret", "test-model")
	out, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(out, "[feed.rule]") { t.Fatalf("out = %q", out) }
	if gotAuth != "Bearer secret" { t.Fatalf("auth = %q", gotAuth) }
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
go test ./internal/ai
```

Expected: FAIL because `New`, `Message`, and `Complete` are undefined.

- [ ] **Step 4: Implement AI client**

Create `internal/ai/ai.go`:

```go
package ai

import (
	"context"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type Message struct { Role, Content string }

type Client struct { inner *openai.Client; model string }

func New(baseURL, apiKey, model string) *Client {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" { opts = append(opts, option.WithBaseURL(baseURL)) }
	client := openai.NewClient(opts...)
	return &Client{inner: &client, model: model}
}

func (c *Client) Complete(ctx context.Context, msgs []Message) (string, error) {
	params := openai.ChatCompletionNewParams{Model: c.model, Temperature: openai.Float(0.2)}
	for _, msg := range msgs {
		switch msg.Role {
		case "system": params.Messages = append(params.Messages, openai.SystemMessage(msg.Content))
		case "assistant": params.Messages = append(params.Messages, openai.AssistantMessage(msg.Content))
		default: params.Messages = append(params.Messages, openai.UserMessage(msg.Content))
		}
	}
	completion, err := c.inner.Chat.Completions.New(ctx, params)
	if err != nil { return "", err }
	if len(completion.Choices) == 0 { return "", nil }
	return completion.Choices[0].Message.Content, nil
}
```

- [ ] **Step 5: Add fenced-block parsing tests**

Append to `cmd/probe/probe_test.go`:

```go
func TestExtractFencedTOML(t *testing.T) {
	in := "here\n```toml\n[feed.rule]\nitem = \".item\"\n```\nthere"
	got, err := extractFencedTOML(in)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(got), `[feed.rule]`) { t.Fatalf("got = %q", got) }
}

func TestExtractFencedTOMLRejectsMissingBlock(t *testing.T) {
	if _, err := extractFencedTOML("no fence"); err == nil { t.Fatal("expected error") }
}
```

- [ ] **Step 6: Implement suggest loop with extraction preview**

Modify `cmd/probe/probe.go` by replacing the `runSuggest` missing function with:

```go
func runSuggest(ctx context.Context, c *cli.Command, out, errOut io.Writer) error {
	cfg, err := config.Load(c.String("config"))
	if err != nil { return err }
	baseURL, apiKey, model := resolveAI(cfg)
	if apiKey == "" || model == "" { return errors.New("AI is not configured: set [ai] in config or OPENAI_API_KEY and OPENAI_MODEL") }
	arg := c.Args().First()
	client := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)
	body, finalURL, err := client.Get(ctx, arg)
	if err != nil { return err }
	promptHTML := body
	limit := c.Int("html-bytes")
	if limit > 0 && len(promptHTML) > limit { promptHTML = promptHTML[:limit] }
	aiClient := ai.New(baseURL, apiKey, model)
	msgs := []ai.Message{{Role: "system", Content: suggestSystemPrompt}, {Role: "user", Content: fmt.Sprintf("URL: %s\n\nHTML:\n%s", finalURL.String(), string(promptHTML))}}
	for {
		resp, err := aiClient.Complete(ctx, msgs)
		if err != nil { return err }
		ruleText, err := extractFencedTOML(resp)
		if err != nil {
			fmt.Fprintf(errOut, "model returned invalid response: %v\n", err)
			msgs = append(msgs, ai.Message{Role: "assistant", Content: resp}, ai.Message{Role: "user", Content: "Your last response did not contain a fenced TOML block. Return only a fenced TOML block."})
			continue
		}
		rule, err := config.ParseRuleTOML(ruleText)
		if err != nil {
			fmt.Fprintf(errOut, "model returned invalid TOML rule: %v\n", err)
			msgs = append(msgs, ai.Message{Role: "assistant", Content: resp}, ai.Message{Role: "user", Content: "Your last TOML rule failed validation: " + err.Error()})
			continue
		}
		fmt.Fprintln(out, string(ruleText))
		if err := outputExtracted(out, errOut, false, 5, body, finalURL, rule); err != nil { return err }
		fmt.Fprintln(errOut, "Type 'a' to accept, 'r <feedback>' to refine, or 'q' to quit:")
		line, err := readLine(os.Stdin)
		if err != nil { return err }
		switch {
		case line == "a": return nil
		case line == "q": return errors.New("quit")
		case strings.HasPrefix(line, "r "):
			feedback := strings.TrimSpace(strings.TrimPrefix(line, "r "))
			msgs = append(msgs, ai.Message{Role: "assistant", Content: resp}, ai.Message{Role: "user", Content: "Refine the rule. Feedback: " + feedback})
		default:
			fmt.Fprintln(errOut, "unknown command; use a, r <feedback>, or q")
		}
	}
}

func readLine(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() { return "", scanner.Err() }
	return strings.TrimSpace(scanner.Text()), nil
}

func extractFencedTOML(s string) ([]byte, error) {
	start := strings.Index(s, "```")
	if start < 0 { return nil, errors.New("missing fenced code block") }
	rest := s[start+3:]
	if strings.HasPrefix(rest, "toml") { rest = strings.TrimPrefix(rest, "toml") }
	if strings.HasPrefix(rest, "\n") { rest = rest[1:] }
	end := strings.Index(rest, "```")
	if end < 0 { return nil, errors.New("unterminated fenced code block") }
	return []byte(strings.TrimSpace(rest[:end])), nil
}

func resolveAI(cfg *config.Config) (baseURL, apiKey, model string) {
	baseURL, apiKey, model = cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model
	if baseURL == "" { baseURL = os.Getenv("OPENAI_BASE_URL") }
	if apiKey == "" { apiKey = os.Getenv("OPENAI_API_KEY") }
	if model == "" { model = os.Getenv("OPENAI_MODEL") }
	return baseURL, apiKey, model
}

const suggestSystemPrompt = `You generate rssify TOML rule blocks. Output only a fenced TOML block with [feed.rule], item, title, link, optional description, optional pub_date. CSS selectors are default; prefix xpath: for XPath. Do not include explanation.`
```

Add imports:

```go
"bufio"
"github.com/LeeTeng2001/rssify/internal/ai"
```

- [ ] **Step 7: Run tests and commit**

Run:

```bash
go test ./...
```

Expected: PASS.

Commit:

```bash
git add go.mod go.sum internal/ai cmd/probe/probe.go
git commit -m "feat: add ai-assisted probe mode"
```

---

### Task 12: Example Config, README, Final Verification

**Files:**
- Create: `rssify.toml.example`
- Create: `README.md`

- [ ] **Step 1: Add example config**

Create `rssify.toml.example`:

```toml
[server]
listen = ":8080"
cache_dir = "./cache"
user_agent = "rssify/dev"
fetch_timeout = "15s"

[scrape]
max_attempts = 3
retry_backoff = "30s"

# Optional, only used by `rssify probe --suggest`.
# [ai]
# base_url = "https://api.openai.com/v1"
# api_key = "$OPENAI_API_KEY"
# model = "gpt-4o-mini"

[[feed]]
id = "hn"
url = "https://news.ycombinator.com/"
title = "Hacker News"
description = "Hacker News front page"
interval = "10m"

[feed.rule]
item = "tr.athing"

[feed.rule.title]
selector = "span.titleline > a"

[feed.rule.link]
selector = "span.titleline > a"
attr = "href"
absolute = true
```

- [ ] **Step 2: Add README**

Create `README.md`:

```markdown
# rssify

`rssify` turns configured HTML pages into RSS 2.0 feeds.

## Build

```bash
go build ./...
```

## Configure

Copy `rssify.toml.example` to `rssify.toml` and edit `[[feed]]` blocks.

Rules use CSS selectors by default. Prefix a selector with `xpath:` to use XPath.

## Serve

```bash
go run . serve --config rssify.toml
```

Feeds are available at `/feed/<id>.xml`.

## Probe

```bash
go run . probe hn --config rssify.toml
```

## AI Suggestions

Configure OpenAI-compatible credentials:

```bash
export OPENAI_API_KEY=...
export OPENAI_MODEL=gpt-4o-mini
```

Then run:

```bash
go run . probe https://example.com --suggest
```
```

- [ ] **Step 3: Final verification**

Run:

```bash
go test ./...
go test -race ./...
go run . version
go run . serve --help
go run . probe --help
```

Expected:
- All tests PASS.
- Race tests PASS.
- `version` prints `dev`.
- Help commands render without errors.

- [ ] **Step 4: Commit docs and examples**

Commit:

```bash
git add README.md rssify.toml.example
git commit -m "docs: add usage docs and example config"
```

---

## Self-Review Notes

- Spec coverage: covered module/CLI/logging, config/defaults/validation, fetch, extract, render, cache, server, scheduler/retries, serve command, probe verify including `--rule`, OpenAI-compatible AI client, AI suggestion loop with parse/preview/refine, examples/docs.
- Placeholder scan: plan avoids TODO/TBD placeholders and includes concrete tests, code, commands, and expected outcomes for every task.
- No hot reload, metrics, status page, database, item history, or SPA rendering tasks are included, matching non-goals.

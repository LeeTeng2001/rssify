# rssify — Design Spec

**Date:** 2026-05-25
**Status:** Draft for review

## Goal

Turn arbitrary HTML pages into RSS 2.0 feeds, driven by a static, operator-authored config file. Optimize for low CPU and memory footprint, simple operations, and predictable behavior. No web frontend.

## Non-goals

- Interactive web-based rule authoring
- Per-user accounts, authentication, or multi-tenancy
- Item history beyond what is currently on the source page
- JavaScript-rendered (SPA) page support
- Database storage
- Hot-reload of the config file
- Built-in metrics, dashboards, or admin UI

## Architecture overview

Single Go binary, two subcommands:

- `rssify serve` — long-running HTTP daemon. Loads config, runs scheduled scrapes, serves rendered RSS.
- `rssify probe` — short-lived CLI for verifying rules against live URLs and (optionally) authoring rules with AI assistance.

The server has no AI dependency at runtime. AI code only exists on the `probe --suggest` path.

### Repository layout

```
rssify/
  main.go                 # urfave/cli/v3 root, dispatches subcommands
  cmd/
    serve/serve.go        # serve subcommand
    probe/probe.go        # probe subcommand (verify + --suggest loop)
  internal/
    config/               # TOML load + validate + compile rules
    fetch/                # HTTP GET wrapper
    extract/              # HTML -> []Item (pure)
    render/               # FeedMeta + []Item -> RSS XML (pure)
    cache/                # in-memory map + atomic file writes
    scheduler/            # one goroutine per feed, retry loop
    server/               # http.Handler for /feed/<id>.xml
    ai/                   # OpenAI-compatible client (probe only)
    logging/              # slog + tint setup
  testdata/               # HTML fixtures for extract/render tests
  rssify.toml.example
  README.md
  go.mod
  go.sum
```

`internal/` enforces no external API surface. If `extract` is wanted as a library later, promote it then.

### Package boundaries

| Package | Knows about | Does not know about |
|---|---|---|
| `config` | TOML schema, selector compilation | HTTP, scheduling, AI |
| `fetch` | HTTP, timeouts, redirects, gzip | Selectors, rules, RSS |
| `extract` | HTML, selectors, field semantics, date parsing | HTTP, files, RSS schema |
| `render` | RSS 2.0 schema, XML escaping | HTTP, files, selectors |
| `cache` | In-memory map, atomic file writes | Scrape logic, HTTP |
| `scheduler` | Tickers, contexts, retry policy | Selectors, RSS schema |
| `server` | `net/http`, route → cache lookup | Scrape logic, selectors |
| `ai` | OpenAI-compatible chat completions | Anything else |
| `probe` | Composes fetch/extract/ai for the operator | Server, scheduler, cache |

The seam is small and explicit: `extract` and `render` are pure functions. The same code path runs in both `probe` and `serve`, so `probe` shows what `serve` will produce.

## Configuration

### File format

TOML, default path `./rssify.toml`, override with `--config`. TOML is preferred over YAML because rule values contain CSS selectors with colons and brackets — TOML's quoting is less surprising.

### Schema

```toml
[server]
listen        = ":8080"
cache_dir     = "./cache"
user_agent    = "rssify/0.1 (+https://example.com)"
fetch_timeout = "15s"

[scrape]
max_attempts  = 3      # total attempts including the first; 1 = no retry
retry_backoff = "30s"  # constant delay between attempts

[ai]                    # optional; only read by `probe --suggest`
base_url = "https://api.openai.com/v1"
api_key  = "$OPENAI_API_KEY"   # literal "$VAR" expands from env at load
model    = "gpt-4o-mini"

[[feed]]
id           = "hn-frontpage"   # /feed/hn-frontpage.xml; ^[a-z0-9][a-z0-9-]*$
url          = "https://news.ycombinator.com/"
title        = "Hacker News Front Page"
description  = "Top stories"     # optional; defaults to title if omitted
link         = ""                # optional, defaults to url
interval     = "10m"             # ≥ 1 minute

[feed.rule]
item = "tr.athing"

  [feed.rule.title]
  selector = "td.title > span.titleline > a"

  [feed.rule.link]
  selector = "td.title > span.titleline > a"
  attr     = "href"
  absolute = true

  [feed.rule.description]   # optional; omit or empty selector to skip
  selector = ""

  [feed.rule.pub_date]      # optional
  selector = "span.age"
  format   = "Jan 2, 2006"  # optional Go reference-time layout
```

### Defaults

| Key | Default |
|---|---|
| `server.listen` | `":8080"` |
| `server.cache_dir` | `"./cache"` |
| `server.user_agent` | `"rssify/<version>"` |
| `server.fetch_timeout` | `"15s"` |
| `scrape.max_attempts` | `3` |
| `scrape.retry_backoff` | `"30s"` |

### Rule grammar

A rule has exactly five parts:

| Key | Required | Meaning |
|---|---|---|
| `item` | yes | Selector matching each list element. Each match becomes one `<item>`. |
| `title` | yes | Field selector relative to an item. |
| `link` | yes | Field selector relative to an item. |
| `description` | no | Field selector relative to an item. Omitted/empty → field not emitted. |
| `pub_date` | no | Field selector relative to an item. Omitted/empty → field not emitted. |

Each field selector:

| Key | Default | Notes |
|---|---|---|
| `selector` | required if field present | CSS by default. Prefix with `xpath:` for XPath. |
| `attr` | `""` (text content) | Extract this attribute instead of text. |
| `absolute` | `false` | Resolve as URL relative to feed's URL. URL fields only; rejected on `pub_date`. |
| `format` | `""` (pass-through) | **`pub_date` only.** Go reference-time layout. Parsed value re-emitted as RFC 1123Z. On parse failure, `<pubDate>` omitted from item and a warning logged. |

No transforms, no regex, no scripting. If a page needs more, use a different selector or pass on RSSifying it.

### Validation (at load, fail fast)

- All `[[feed]]` IDs unique, non-empty, match `^[a-z0-9][a-z0-9-]*$`.
- `url`, `title`, `interval`, `rule.item`, `rule.title.selector`, `rule.link.selector` non-empty.
- `interval` parses as `time.Duration`, ≥ 1 minute.
- Every selector compiles successfully.
- `pub_date.format` (if non-empty) round-trips: `time.Parse(format, time.Now().UTC().Format(format))` succeeds.
- `pub_date.absolute` is forbidden (config error).
- `scrape.max_attempts >= 1`, `scrape.retry_backoff >= 1s`.
- `feed.description`: if empty/omitted, defaults to `feed.title` (RSS 2.0 channel description is required by spec).
- `feed.link`: if empty/omitted, defaults to `feed.url`.

On any error, print all errors and exit non-zero. No partial startup.

### Hot-reload

Not supported. Edit file, restart process. Cache files on disk make restart cheap.

## Scheduler & cache

### One goroutine per feed

```
on start:
  if cache_dir/<id>.xml exists -> load into in-memory cache
  jitter := random 0..min(30s, interval/4)
  sleep(jitter)
  scrape()
loop:
  sleep(interval)
  scrape()
```

Jitter prevents thundering-herd against any one upstream and against rssify itself on startup.

### `scrape()` flow

```
for attempt := 1; attempt <= scrape.max_attempts; attempt++ {
    bytes, finalURL, err := fetch.Get(ctx, feed.url)
    if err == nil { break }
    if !isRetryable(err) { break }
    log.Warn("fetch failed", feed=id, attempt, max, err)
    if attempt < max { sleepCtx(ctx, retry_backoff) }
}
if err != nil { return }   // keep previous cached XML

items, warnings, err := extract.Run(bytes, feed.compiledRule, finalURL)
for _, w := range warnings { log.Debug("extract warning", feed=id, w...) }
if err != nil { log.Warn("extract failed", feed=id, err); return }
if len(items) == 0 { log.Warn("zero items, retaining cache", feed=id); return }

xml := render.RSS(feed.meta, items, time.Now().UTC())
cache.Put(feed.id, xml)   // atomic file rename + in-memory map update
log.Info("scrape ok", feed=id, items=len(items), attempts=attempt, duration_ms=...)
```

### Retry policy

- Constant backoff, not exponential. Outer interval is the natural exponential.
- Retry only on transport errors (`net.Error`, timeouts, connection refused) and HTTP 500/502/503/504.
- Do not retry 4xx (URL is wrong), 429 (back off — useless within 30s), parse errors, or zero-items.
- Sleep between attempts honors `ctx`: SIGTERM during backoff aborts immediately.

### Failures are non-destructive

A fetch error, parse error, or zero-items result leaves the previously-cached XML untouched. Readers continue receiving the last-good feed. The next tick retries.

### Cache layers

- **In-memory:** `map[feedID][]byte` of fully-rendered XML, `sync.RWMutex`. Server reads from this on every request — no parsing, no rendering on the read path.
- **On-disk:** `<cache_dir>/<feedID>.xml`. Written after every successful scrape via `os.WriteFile` to `<id>.xml.tmp` + `os.Rename`. Read once at startup.

In-memory is always the latest. Disk is equal-or-stale. Server only reads in-memory. Scheduler is the only writer.

If a feed is removed from config, its cache file is left in place. Operator deletes manually.

### Shutdown

SIGINT/SIGTERM cancels a root context. Each scheduler loop returns at the next check. In-flight fetches honor the context. HTTP server stops accepting connections, drains for up to 5s, exits.

## HTTP server

Routes:

| Path | Handler |
|---|---|
| `/feed/<id>.xml` | Lookup cache. Hit → 200 with `Content-Type: application/rss+xml; charset=utf-8` and the bytes. Miss for known feed → 503 with explanatory text body. Unknown feed → 404. |

That's the entire server surface. No metrics, no health, no admin.

`GET` and `HEAD` only. `If-Modified-Since` and `ETag` are not implemented in v1; readers will refetch on schedule. Adding them later is straightforward (track scrape time per feed, compare).

## Extract package

### Public surface

```go
package extract

type Item struct {
    Title       string
    Link        string
    Description string
    PubDate     string  // RFC 1123Z, or "" if absent / parse failed
}

type Warning struct {
    ItemIndex int
    Field     string
    Message   string
}

func Run(html []byte, rule config.CompiledRule, baseURL *url.URL) ([]Item, []Warning, error)
```

Pure function. Deterministic. Returns error only on parse-tree construction failure. Per-item / per-field problems become `Warning`s; missing fields stay empty rather than dropping the item.

### Internal flow

1. Parse HTML once into `*html.Node` (golang.org/x/net/html).
2. Find all item nodes via `rule.Item.Find(root)`.
3. For each item node:
   - Apply each field selector against the item subtree.
   - `attr == ""` → `goquery` text traversal; otherwise read the attribute.
   - Trim leading/trailing whitespace always.
   - If `absolute`, resolve against `baseURL` (skip if not a valid URL → warning).
   - If `pub_date.format != ""`, `time.Parse` then re-emit as RFC 1123Z. Failure → empty + warning.

### Selector dispatch

A `Selector` interface wraps both backends:

```go
type Selector interface {
    Find(node *html.Node) []*html.Node
}
```

Implementations:
- `cssSelector` wrapping `cascadia.Sel` (used by goquery underneath)
- `xpathSelector` wrapping `xpath.Expr`

Selectors are compiled once at config load and stored in `config.CompiledRule`. `extract` never sees raw selector strings.

A selector string starting with `xpath:` selects the XPath backend; the prefix is stripped before compilation. Anything else is CSS.

## Render package

### Public surface

```go
package render

type FeedMeta struct {
    Title       string
    Link        string
    Description string
    SelfURL     string
}

func RSS(meta FeedMeta, items []extract.Item, generated time.Time) []byte
```

Pure. `generated` is injected (not `time.Now()`) so tests pin output.

### Output

RSS 2.0 with one Atom extension (`<atom:link rel="self">`):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>...</title>
    <link>...</link>
    <description>...</description>
    <atom:link rel="self" href="..." type="application/rss+xml"/>
    <lastBuildDate>...</lastBuildDate>
    <generator>rssify</generator>
    <item>
      <title>...</title>
      <link>...</link>
      <guid isPermaLink="true">...</guid>
      <description>...</description>   <!-- omitted if empty -->
      <pubDate>...</pubDate>            <!-- omitted if empty -->
    </item>
    ...
  </channel>
</rss>
```

GUID = `link`, `isPermaLink="true"`. We have no item history; link is the only stable candidate. Acceptable dedupe behavior.

`encoding/xml` from stdlib handles escaping. Description content is plain text — we don't carry source HTML through; selectors should pick rendered text.

## Logging

`log/slog` with `github.com/lmittmann/tint` as the default handler.

Flags on root command:
- `--log-level` = `debug` | `info` | `warn` | `error` (default `info`)
- `--log-format` = `tint` | `json` (default `tint`)

Levels in practice:

- `info` once per successful scrape: `feed=hn items=30 attempts=1 duration_ms=142`
- `warn` on scrape failure, retry, zero-items: includes `feed`, `attempt`, `error`, `retain_cached`
- `error` only for things the operator must fix: config load failure, port bind failure, cache_dir not writable

No request logging. Reverse proxies handle that.

## Probe subcommand

### Modes

```
rssify probe <feed-id>                          # verify configured rule
rssify probe <url> --rule rule.toml             # verify ad-hoc rule
rssify probe <url> --suggest                    # interactive AI authoring
```

Common flags:
- `--limit N` — max items to print (default 10; 0 = all)
- `--json` — dump full `[]Item` as JSON instead of a table
- `--html-bytes N` — `--suggest` only: bytes of HTML sent to the model (default 30720)

### Disambiguating the first positional argument

`probe`'s first positional arg may be a feed ID or a URL. Resolution rule, in order:

1. If the value matches `^[a-z0-9][a-z0-9-]*$` AND a config is loaded AND a feed with that ID exists → feed-ID mode.
2. Otherwise treat as a URL. If `--rule` is provided, use it. If `--suggest` is provided, enter the AI loop. If neither → error: "to probe an arbitrary URL, pass --rule or --suggest".

This prevents an unconfigured-but-valid-looking ID from silently being treated as a (broken) URL.

### Verify mode

Loads rule (from config or standalone file), fetches URL, runs extract, prints:
- Items as ASCII table (`# | Title | Link | PubDate`); description omitted from table
- Warnings to stderr (yellow via tint when TTY, plain otherwise)
- Exit 0 on success even if items are zero (operator may be debugging)

### `--suggest` interactive loop

```
fetch page once; keep bytes for the session
truncated_html := bytes truncated to --html-bytes (default 30 KB)
conversation := [system_prompt, first_user_message_with_html]
proposed_rule := nil

loop:
    if proposed_rule == nil:
        proposed_rule = ai.Complete(conversation)
        parse fenced TOML block from response
        if parse fails:
            print error, append "your last response was not valid TOML: <err>" to conversation
            continue

    print proposed_rule (highlighted)
    items, warnings := extract.Run(bytes, proposed_rule, baseURL)
    print first 5 items as table
    print warnings if any

    prompt: [a]ccept / [r]efine / [q]uit
        accept -> print final TOML with header comment, exit 0
        refine -> read multi-line feedback, append to conversation, set proposed_rule = nil, loop
        quit   -> exit 1
```

System prompt instructs the model to emit only a fenced TOML block following the rule grammar. Low temperature (0.2). Conversation kept in process memory only, dropped at exit. HTML sent once on first turn; later turns reference it.

### Output on accept

```
# Authored with: rssify probe https://example.com --suggest
# Verified extracting 5 items
[feed.rule]
item = "..."

  [feed.rule.title]
  selector = "..."
...
```

Operator pastes under their own `[[feed]]` block (id, url, title, interval are author decisions).

### AI configuration resolution

1. `[ai]` block in config file
2. Env vars `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `OPENAI_MODEL` fill any unset values
3. If still unset and `--suggest` was passed → exit non-zero with explanatory message

## AI package

Wraps the official `github.com/openai/openai-go` SDK with a configurable `BaseURL` so it works with OpenAI, Ollama, llama.cpp server, OpenRouter, and Anthropic-compatible proxies.

```go
package ai

type Client struct {
    inner openai.Client
    model string
}

func New(baseURL, apiKey, model string) *Client

func (c *Client) Complete(ctx context.Context,
    msgs []openai.ChatCompletionMessageParamUnion) (string, error)
```

Single chat-completions call. No streaming. No tool use. Temperature 0.2.

Imported only by `cmd/probe`. Not imported by `cmd/serve`. Go's dead-code elimination ensures `serve` doesn't pay for the SDK.

## CLI

`github.com/urfave/cli/v3`. Root command with persistent `--config`, `--log-level`, `--log-format`. Subcommands: `serve`, `probe`, `version`.

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/urfave/cli/v3` | CLI framework |
| `github.com/BurntSushi/toml` | Config parsing |
| `github.com/PuerkitoBio/goquery` | CSS selectors |
| `github.com/antchfx/htmlquery` | XPath against HTML |
| `github.com/antchfx/xpath` | XPath compilation (transitive) |
| `github.com/lmittmann/tint` | slog handler |
| `github.com/openai/openai-go` | AI client (probe only) |

Stdlib for everything else: `net/http`, `encoding/xml`, `encoding/json`, `log/slog`, `time`, `context`, `os`, `sync`, `golang.org/x/net/html` (transitive via goquery/htmlquery).

Target binary size: 8–12 MB. Idle RSS: 15–25 MB depending on feed count.

## Testing strategy

- `internal/config`: table-driven tests on TOML fixtures, both valid and every error case.
- `internal/extract`: HTML fixtures in `testdata/`, one per source-page archetype (HN, blog, GitHub releases, etc.). Assert exact `[]Item` output.
- `internal/render`: golden-file tests. Pinned `generated` time, byte-for-byte XML comparison.
- `internal/cache`: concurrent Put/Get under `-race`; atomic write verified by killing a goroutine mid-write in a test.
- `internal/scheduler`: fake `fetch` returning canned bytes/errors, fake clock; assert retry counts, backoff timing, cache invariants.
- `internal/server`: `httptest.Server` against a populated cache.
- `cmd/probe`: smoke tests calling `extract` directly. AI loop covered by manual testing — mocking chat completions has low value vs. cost.
- Integration: `go test ./...` with `-race`. No network in unit tests; all HTML is static fixtures.

## Open questions / deferred

These are intentionally *not* in v1 but the design should not preclude them:

- **Date formats per locale / relative-time parsing.** Adding a `format` enum (`"relative"`, `"unix"`) is additive.
- **HTTP caching response headers** (`If-Modified-Since`, `ETag` from server side). Additive.
- **Rolling window of N items with deduplication.** Would require item-level state on disk (sidecar JSON). Additive but invasive.
- **`allow_empty = true` per-feed flag.** If any operator hits the "legitimately empty source" case.
- **Hot-reload via SIGHUP.** Additive, low-risk; deferred per "bare minimum" choice.
- **Headless browser opt-in for SPA pages.** Additive but heavy; only if a real target site demands it.

## Trade-offs accepted

- **Zero items = soft failure.** A legitimately-empty source page can't reach readers; protects against silent selector breakage. Mitigated by future `allow_empty` flag if needed.
- **No retry within a tick beyond `max_attempts × retry_backoff`.** A long-interval feed with a flaky upstream is up to one interval staler than ideal. Cached XML keeps readers happy in the meantime.
- **GUID = link, no history.** Two items legitimately sharing a link will dedupe in readers. Acceptable.
- **No date parsing for relative times.** Operator picks a different selector or omits `pub_date`.
- **No SPA support.** Operator picks a different page or accepts that JS-rendered sites aren't supported.

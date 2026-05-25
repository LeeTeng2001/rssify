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
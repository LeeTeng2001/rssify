package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
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
	CacheDir     string        `toml:"cache_dir"`
	UserAgent    string        `toml:"user_agent"`
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

type CompiledRule struct {
	Item        Selector
	Title       CompiledField
	Link        CompiledField
	Description *CompiledField
	PubDate     *CompiledField
}

type CompiledField struct {
	Selector Selector
	Attr     string
	Absolute bool
	Format   string
}

type rawConfig struct {
	Server rawServerConfig
	Scrape rawScrapeConfig
	AI     AIConfig
	Feeds  []rawFeedConfig `toml:"feed"`
}

type rawServerConfig struct {
	Listen       string
	CacheDir     string `toml:"cache_dir"`
	UserAgent    string `toml:"user_agent"`
	FetchTimeout string `toml:"fetch_timeout"`
}

type rawScrapeConfig struct {
	MaxAttempts  *int   `toml:"max_attempts"`
	RetryBackoff string `toml:"retry_backoff"`
}

type rawFeedConfig struct {
	ID          string
	URL         string
	Title       string
	Description string
	Link        string
	Interval    string
	Rule        rawRule
}

type rawRuleDocument struct {
	Rule rawRule
}

type rawRule struct {
	Item        string
	Title       rawField
	Link        rawField
	Description rawField
	PubDate     rawField `toml:"pub_date"`
}

type rawField struct {
	Selector string
	Attr     string
	Absolute bool
	Format   string
}

var feedIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw rawConfig
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Listen:       defaultString(raw.Server.Listen, ":8080"),
			CacheDir:     defaultString(raw.Server.CacheDir, "./cache"),
			UserAgent:    defaultString(raw.Server.UserAgent, "rssify/dev"),
			FetchTimeout: 15 * time.Second,
		},
		Scrape: ScrapeConfig{
			MaxAttempts:  defaultInt(raw.Scrape.MaxAttempts),
			RetryBackoff: 30 * time.Second,
		},
		AI: AIConfig{
			BaseURL: expandEnvLiteral(raw.AI.BaseURL),
			APIKey:  expandEnvLiteral(raw.AI.APIKey),
			Model:   expandEnvLiteral(raw.AI.Model),
		},
	}

	var errs []error
	if raw.Server.FetchTimeout != "" {
		duration, err := time.ParseDuration(raw.Server.FetchTimeout)
		if err != nil {
			errs = append(errs, fmt.Errorf("fetch_timeout: %w", err))
		} else {
			cfg.Server.FetchTimeout = duration
		}
	}
	if raw.Scrape.RetryBackoff != "" {
		duration, err := time.ParseDuration(raw.Scrape.RetryBackoff)
		if err != nil {
			errs = append(errs, fmt.Errorf("retry_backoff: %w", err))
		} else {
			cfg.Scrape.RetryBackoff = duration
		}
	}

	if cfg.Scrape.MaxAttempts < 1 {
		errs = append(errs, fmt.Errorf("max_attempts must be at least 1"))
	}
	if cfg.Scrape.RetryBackoff < time.Second {
		errs = append(errs, fmt.Errorf("retry_backoff must be at least 1s"))
	}

	seenIDs := make(map[string]struct{}, len(raw.Feeds))
	for _, rawFeed := range raw.Feeds {
		feed, feedErrs := compileFeed(rawFeed, seenIDs)
		errs = append(errs, feedErrs...)
		cfg.Feeds = append(cfg.Feeds, feed)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return cfg, nil
}

func LoadRule(path string) (CompiledRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CompiledRule{}, err
	}
	return ParseRuleTOML(data)
}

func ParseRuleTOML(data []byte) (CompiledRule, error) {
	var raw rawRuleDocument
	if err := toml.Unmarshal(data, &raw); err != nil {
		return CompiledRule{}, err
	}
	rule, errs := compileRule(raw.Rule, "rule")
	if err := errors.Join(errs...); err != nil {
		return CompiledRule{}, err
	}
	return rule, nil
}

func compileFeed(raw rawFeedConfig, seenIDs map[string]struct{}) (FeedConfig, []error) {
	feed := FeedConfig{
		ID:          raw.ID,
		URL:         raw.URL,
		Title:       raw.Title,
		Description: defaultString(raw.Description, raw.Title),
		Link:        defaultString(raw.Link, raw.URL),
	}
	prefix := fmt.Sprintf("feed %s", raw.ID)
	var errs []error

	if !feedIDPattern.MatchString(raw.ID) {
		errs = append(errs, fmt.Errorf("%s id must match %s", prefix, feedIDPattern.String()))
	}
	if _, ok := seenIDs[raw.ID]; ok {
		errs = append(errs, fmt.Errorf("%s id must be unique", prefix))
	}
	seenIDs[raw.ID] = struct{}{}

	parsedURL, err := url.Parse(raw.URL)
	if err != nil || !parsedURL.IsAbs() || parsedURL.Host == "" {
		errs = append(errs, fmt.Errorf("%s url must be absolute with scheme and host", prefix))
	}
	if raw.Title == "" {
		errs = append(errs, fmt.Errorf("%s title is required", prefix))
	}
	if raw.Interval == "" {
		errs = append(errs, fmt.Errorf("%s interval is required", prefix))
	} else if interval, err := time.ParseDuration(raw.Interval); err != nil {
		errs = append(errs, fmt.Errorf("%s interval: %w", prefix, err))
	} else if interval < time.Minute {
		errs = append(errs, fmt.Errorf("%s interval must be at least 1m", prefix))
	} else {
		feed.Interval = interval
	}

	rule, ruleErrs := compileRule(raw.Rule, prefix+" rule")
	feed.Rule = rule
	errs = append(errs, ruleErrs...)
	return feed, errs
}

func compileRule(raw rawRule, prefix string) (CompiledRule, []error) {
	var rule CompiledRule
	var errs []error

	item, err := CompileSelector(raw.Item)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s item selector: %w", prefix, err))
	} else {
		rule.Item = item
	}

	title, err := compileRequiredField(raw.Title, prefix+" title")
	if err != nil {
		errs = append(errs, err)
	} else {
		rule.Title = title
	}

	link, err := compileRequiredField(raw.Link, prefix+" link")
	if err != nil {
		errs = append(errs, err)
	} else {
		rule.Link = link
	}

	if fieldIsSet(raw.Description) {
		description, err := compileOptionalField(raw.Description, prefix+" description", false)
		if err != nil {
			errs = append(errs, err)
		} else {
			rule.Description = &description
		}
	}

	if fieldIsSet(raw.PubDate) {
		pubDate, err := compileOptionalField(raw.PubDate, prefix+" pub_date", true)
		if err != nil {
			errs = append(errs, err)
		} else {
			rule.PubDate = &pubDate
		}
	}

	return rule, errs
}

func fieldIsSet(raw rawField) bool {
	return raw.Selector != "" || raw.Attr != "" || raw.Absolute || raw.Format != ""
}

func compileRequiredField(raw rawField, prefix string) (CompiledField, error) {
	if raw.Selector == "" {
		return CompiledField{}, fmt.Errorf("%s selector is required", prefix)
	}
	return compileOptionalField(raw, prefix, false)
}

func compileOptionalField(raw rawField, prefix string, allowFormat bool) (CompiledField, error) {
	var errs []error
	selector, err := CompileSelector(raw.Selector)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s selector: %w", prefix, err))
	}
	if raw.Format != "" && !allowFormat {
		errs = append(errs, fmt.Errorf("%s format is only valid for pub_date", prefix))
	}
	if allowFormat && raw.Absolute {
		errs = append(errs, fmt.Errorf("%s absolute is forbidden", prefix))
	}
	if allowFormat && raw.Format != "" {
		formatted := time.Now().UTC().Format(raw.Format)
		if _, err := time.Parse(raw.Format, formatted); err != nil {
			errs = append(errs, fmt.Errorf("%s format: %w", prefix, err))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return CompiledField{}, err
	}
	return CompiledField{Selector: selector, Attr: raw.Attr, Absolute: raw.Absolute, Format: raw.Format}, nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultInt(value *int) int {
	if value == nil {
		return 3
	}
	return *value
}

func expandEnvLiteral(value string) string {
	if len(value) > 1 && value[0] == '$' {
		return os.Getenv(value[1:])
	}
	return value
}

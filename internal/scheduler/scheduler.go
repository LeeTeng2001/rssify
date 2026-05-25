package scheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"time"

	"github.com/LeeTeng2001/rssify/internal/cache"
	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/LeeTeng2001/rssify/internal/extract"
	"github.com/LeeTeng2001/rssify/internal/fetch"
	"github.com/LeeTeng2001/rssify/internal/render"
)

type Fetcher interface {
	Get(ctx context.Context, rawURL string) ([]byte, *url.URL, error)
}

type FetchFunc func(ctx context.Context, rawURL string) ([]byte, *url.URL, error)

func (f FetchFunc) Get(ctx context.Context, rawURL string) ([]byte, *url.URL, error) {
	return f(ctx, rawURL)
}

type Options struct {
	MaxAttempts  int
	RetryBackoff time.Duration
	Logger       *slog.Logger
	SelfURL      func(feedID string) string
}

type Scheduler struct {
	feeds   []config.FeedConfig
	cache   *cache.Cache
	fetcher Fetcher
	opts    Options
}

func New(feeds []config.FeedConfig, c *cache.Cache, fetcher Fetcher, opts Options) *Scheduler {
	if opts.MaxAttempts == 0 {
		opts.MaxAttempts = 1
	}
	if opts.RetryBackoff == 0 {
		opts.RetryBackoff = 30 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.SelfURL == nil {
		opts.SelfURL = func(string) string { return "" }
	}
	return &Scheduler{
		feeds:   feeds,
		cache:   c,
		fetcher: fetcher,
		opts:    opts,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	for i := range s.feeds {
		go s.loop(ctx, s.feeds[i])
	}
}

func (s *Scheduler) loop(ctx context.Context, feed config.FeedConfig) {
	maxJitter := minDuration(30*time.Second, feed.Interval/4)
	if maxJitter > 0 {
		jitter := time.Duration(rand.Int64N(int64(maxJitter)))
		if !sleepCtx(ctx, jitter) {
			return
		}
	}

	s.scrape(ctx, feed)

	ticker := time.NewTicker(feed.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scrape(ctx, feed)
		}
	}
}

func (s *Scheduler) scrape(ctx context.Context, feed config.FeedConfig) {
	start := time.Now()
	var data []byte
	var baseURL *url.URL
	var fetchErr error
	attempts := 0

	for attempt := 1; attempt <= s.opts.MaxAttempts; attempt++ {
		attempts = attempt
		data, baseURL, fetchErr = s.fetcher.Get(ctx, feed.URL)
		if fetchErr == nil {
			break
		}
		if !fetch.IsRetryable(fetchErr) || attempt == s.opts.MaxAttempts {
			break
		}
		s.opts.Logger.Warn("fetch failed, retrying",
			slog.String("feed", feed.ID),
			slog.Int("attempt", attempt),
			slog.String("error", fetchErr.Error()),
		)
		if !sleepCtx(ctx, s.opts.RetryBackoff) {
			return
		}
	}

	if fetchErr != nil {
		s.opts.Logger.Warn("fetch failed, retaining cache",
			slog.String("feed", feed.ID),
			slog.String("error", fetchErr.Error()),
		)
		return
	}

	items, warnings, err := extract.Run(data, feed.Rule, baseURL)
	for _, w := range warnings {
		s.opts.Logger.Debug("extraction warning",
			slog.String("feed", feed.ID),
			slog.Int("item", w.ItemIndex),
			slog.String("field", w.Field),
			slog.String("message", w.Message),
		)
	}
	if err != nil {
		s.opts.Logger.Warn("extraction failed, retaining cache",
			slog.String("feed", feed.ID),
			slog.String("error", err.Error()),
		)
		return
	}
	if len(items) == 0 {
		s.opts.Logger.Warn("zero items extracted, retaining cache",
			slog.String("feed", feed.ID),
		)
		return
	}

	rss := render.RSS(render.FeedMeta{
		Title:       feed.Title,
		Link:        feed.Link,
		Description: feed.Description,
		SelfURL:     s.opts.SelfURL(feed.ID),
	}, items, time.Now().UTC())

	if err := s.cache.Put(feed.ID, rss); err != nil {
		s.opts.Logger.Error("cache write failed",
			slog.String("feed", feed.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	s.opts.Logger.Info("scraped feed",
		slog.String("feed", feed.ID),
		slog.Int("items", len(items)),
		slog.Int("attempts", attempts),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	)
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

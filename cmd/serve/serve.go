package serve

import (
	"context"
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
	return &cli.Command{
		Name:   "serve",
		Usage:  "serve configured RSS feeds",
		Action: run,
	}
}

func run(ctx context.Context, c *cli.Command) error {
	logger, err := logging.New(c.String("log-level"), c.String("log-format"), os.Stderr)
	if err != nil {
		return err
	}

	cfg, err := config.Load(c.String("config"))
	if err != nil {
		return err
	}

	xmlCache, err := cache.New(cfg.Server.CacheDir)
	if err != nil {
		return err
	}

	ids := make([]string, len(cfg.Feeds))
	for i, f := range cfg.Feeds {
		ids[i] = f.ID
	}

	if err := xmlCache.LoadExisting(ids); err != nil {
		return err
	}

	root, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fetcher := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)

	s := scheduler.New(cfg.Feeds, xmlCache, fetcher, scheduler.Options{
		MaxAttempts:  cfg.Scrape.MaxAttempts,
		RetryBackoff: cfg.Scrape.RetryBackoff,
		Logger:       logger,
		SelfURL: func(feedID string) string {
			return "/feed/" + feedID + ".xml"
		},
	})
	s.Start(root)

	httpServer := &http.Server{
		Addr:    cfg.Server.Listen,
		Handler: server.New(xmlCache, ids),
	}

	go func() {
		<-root.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("serving", "listen", cfg.Server.Listen)

	if err := httpServer.ListenAndServe(); err == http.ErrServerClosed {
		return nil
	}
	return err
}

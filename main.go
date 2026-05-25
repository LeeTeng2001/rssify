package main

import (
	"context"
	"fmt"
	"os"

	"github.com/LeeTeng2001/rssify/cmd/serve"
	"github.com/LeeTeng2001/rssify/cmd/version"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "rssify",
		Usage: "turn configured HTML pages into RSS feeds",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Value: "rssify.toml",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Value: "info",
			},
			&cli.StringFlag{
				Name:  "log-format",
				Value: "tint",
			},
		},
		Commands: []*cli.Command{
			serve.Command(),
			version.Command(os.Stdout),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

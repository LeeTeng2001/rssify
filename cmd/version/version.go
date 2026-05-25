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
		Action: func(ctx context.Context, cmd *cli.Command) error {
			_, err := fmt.Fprintln(out, Version)
			return err
		},
	}
}

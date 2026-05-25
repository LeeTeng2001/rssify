package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/LeeTeng2001/rssify/internal/ai"
	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/LeeTeng2001/rssify/internal/extract"
	"github.com/LeeTeng2001/rssify/internal/fetch"
	"github.com/urfave/cli/v3"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:  "probe",
		Usage: "verify or author extraction rules",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "rule",
				Usage: "standalone TOML rule file",
			},
			&cli.BoolFlag{
				Name:  "suggest",
				Usage: "AI assist",
			},
			&cli.IntFlag{
				Name:  "limit",
				Value: 10,
				Usage: "max items, 0 for all",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "print JSON",
			},
			&cli.IntFlag{
				Name:  "html-bytes",
				Value: 30720,
				Usage: "max HTML bytes to fetch",
			},
		},
		Action: run,
	}
}

func run(ctx context.Context, c *cli.Command) error {
	if c.Bool("suggest") {
		return runSuggest(ctx, c)
	}

	args := c.Args().Slice()
	if len(args) != 1 {
		return errors.New("usage: rssify probe <feed-id | url>")
	}

	cfg, err := config.Load(c.String("config"))
	if err != nil {
		return err
	}

	arg := args[0]
	var feed *config.FeedConfig
	for i := range cfg.Feeds {
		if cfg.Feeds[i].ID == arg {
			feed = &cfg.Feeds[i]
			break
		}
	}

	if feed == nil {
		return runURLRuleMode(ctx, c, arg, cfg)
	}

	fetcher := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)
	body, finalURL, err := fetcher.Get(ctx, feed.URL)
	if err != nil {
		return err
	}
	return outputExtracted(c.Writer, c.ErrWriter, c.Bool("json"), c.Int("limit"), body, finalURL, feed.Rule)
}

func runURLRuleMode(ctx context.Context, c *cli.Command, rawURL string, cfg *config.Config) error {
	rulePath := c.String("rule")
	if rulePath == "" {
		return errors.New("feed not found; pass --rule or --suggest")
	}

	rule, err := config.LoadRule(rulePath)
	if err != nil {
		return err
	}

	fetcher := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)
	body, finalURL, err := fetcher.Get(ctx, rawURL)
	if err != nil {
		return err
	}
	return outputExtracted(c.Writer, c.ErrWriter, c.Bool("json"), c.Int("limit"), body, finalURL, rule)
}

func outputExtracted(out, errOut io.Writer, asJSON bool, limit int, body []byte, finalURL *url.URL, rule config.CompiledRule) error {
	items, warnings, err := extract.Run(body, rule, finalURL)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintf(errOut, "warning: item %d %s: %s\n", w.ItemIndex, w.Field, w.Message)
	}
	items = limitItems(items, limit)
	if asJSON {
		return json.NewEncoder(out).Encode(items)
	}
	printTable(out, items, limit)
	return nil
}

func limitItems(items []extract.Item, limit int) []extract.Item {
	if limit == 0 || limit >= len(items) {
		return items
	}
	return items[:limit]
}

func printTable(out io.Writer, items []extract.Item, limit int) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tTitle\tLink\tPubDate")
	for i, item := range items {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", i+1, truncate(item.Title, 80), item.Link, item.PubDate)
	}
	w.Flush()
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

const suggestSystemPrompt = "You generate rssify TOML rule blocks. Output only a fenced TOML block with [feed.rule], item, title, link, optional description, optional pub_date. CSS selectors are default; prefix xpath: for XPath. Do not include explanation."

func runSuggest(ctx context.Context, c *cli.Command) error {
	cfg, err := config.Load(c.String("config"))
	if err != nil {
		return err
	}

	baseURL, apiKey, model := resolveAI(cfg)
	if apiKey == "" || model == "" {
		return errors.New("AI is not configured")
	}

	args := c.Args().Slice()
	if len(args) != 1 {
		return errors.New("usage: rssify probe --suggest <url>")
	}

	fetcher := fetch.NewClient(cfg.Server.UserAgent, cfg.Server.FetchTimeout)
	body, finalURL, err := fetcher.Get(ctx, args[0])
	if err != nil {
		return err
	}

	htmlLimit := c.Int("html-bytes")
	promptHTML := string(body)
	if htmlLimit > 0 && len(body) > htmlLimit {
		promptHTML = string(body[:htmlLimit])
	}

	aiClient := ai.New(baseURL, apiKey, model)
	msgs := []ai.Message{
		{Role: "system", Content: suggestSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("URL: %s\n\nHTML:\n%s", finalURL.String(), promptHTML)},
	}

	for {
		result, err := aiClient.Complete(ctx, msgs)
		if err != nil {
			return fmt.Errorf("AI request failed: %w", err)
		}

		ruleText, err := extractFencedTOML(result)
		if err != nil {
			fmt.Fprintf(c.ErrWriter, "error: %v\n", err)
			msgs = append(msgs,
				ai.Message{Role: "assistant", Content: result},
				ai.Message{Role: "user", Content: "Your response did not contain a fenced TOML block. Output only a fenced TOML block."},
			)
			continue
		}

		rule, err := config.ParseRuleTOML(ruleText)
		if err != nil {
			fmt.Fprintf(c.ErrWriter, "error: %v\n", err)
			msgs = append(msgs,
				ai.Message{Role: "assistant", Content: result},
				ai.Message{Role: "user", Content: fmt.Sprintf("The TOML block has a validation error: %v. Fix the rule.", err)},
			)
			continue
		}

		fmt.Fprintf(c.ErrWriter, "%s\n", ruleText)
		return outputExtracted(c.Writer, c.ErrWriter, c.Bool("json"), c.Int("limit"), body, finalURL, rule)
	}
}

func resolveAI(cfg *config.Config) (baseURL, apiKey, model string) {
	baseURL = cfg.AI.BaseURL
	apiKey = cfg.AI.APIKey
	model = cfg.AI.Model
	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
	}
	return baseURL, apiKey, model
}

func readLine(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func extractFencedTOML(s string) ([]byte, error) {
	openIdx := strings.Index(s, "```")
	if openIdx == -1 {
		return nil, errors.New("no fenced block found")
	}
	afterOpen := s[openIdx+3:]
	if strings.HasPrefix(afterOpen, "toml") {
		afterOpen = afterOpen[4:]
	}
	if strings.HasPrefix(afterOpen, "\n") {
		afterOpen = afterOpen[1:]
	}
	closeIdx := strings.Index(afterOpen, "```")
	if closeIdx == -1 {
		return nil, errors.New("fenced block is unterminated")
	}
	content := strings.TrimSpace(afterOpen[:closeIdx])
	return []byte(content), nil
}
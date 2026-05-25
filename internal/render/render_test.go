package render

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/LeeTeng2001/rssify/internal/extract"
)

func TestRSSRendersEscapedRSS2(t *testing.T) {
	generated := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)

	output := string(RSS(FeedMeta{
		Title:       "T & C",
		Link:        "https://example.com",
		Description: "D",
		SelfURL:     "https://feeds.test/feed/x.xml",
	}, []extract.Item{{
		Title:       "A & B",
		Link:        "https://example.com/a?x=1&y=2",
		Description: "desc",
		PubDate:     "Mon, 25 May 2026 00:00:00 +0000",
	}}, generated))

	wants := []string{
		xml.Header,
		`<rss version="2.0"`,
		`xmlns:atom="http://www.w3.org/2005/Atom"`,
		`T &amp; C`,
		`https://example.com/a?x=1&amp;y=2`,
		`<pubDate>Mon, 25 May 2026 00:00:00 +0000</pubDate>`,
	}
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRSSOmitsEmptyOptionalItemFields(t *testing.T) {
	generated := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)

	output := string(RSS(FeedMeta{
		Title:       "T",
		Link:        "https://example.com",
		Description: "D",
		SelfURL:     "https://feeds.test/feed/x.xml",
	}, []extract.Item{{
		Title: "A",
		Link:  "https://example.com/a",
	}}, generated))

	if strings.Contains(output, "<description></description>") {
		t.Fatalf("output contains empty description tag:\n%s", output)
	}
	if strings.Contains(output, "<pubDate></pubDate>") {
		t.Fatalf("output contains empty pubDate tag:\n%s", output)
	}
}

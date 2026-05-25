package probe

import (
	"bytes"
	"strings"
	"testing"

	"github.com/LeeTeng2001/rssify/internal/extract"
)

func TestPrintTable(t *testing.T) {
	items := []extract.Item{
		{Title: "One", Link: "https://e.test/1", PubDate: "Mon"},
	}
	var buf bytes.Buffer
	printTable(&buf, items, 10)

	out := buf.String()
	for _, want := range []string{"#", "Title", "One", "https://e.test/1", "Mon"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestLimitItems(t *testing.T) {
	items := []extract.Item{
		{Title: "First"},
		{Title: "Second"},
	}

	got := limitItems(items, 1)
	if len(got) != 1 || got[0].Title != "First" {
		t.Errorf("limit 1: got %v", got)
	}

	got = limitItems(items, 0)
	if len(got) != 2 {
		t.Errorf("limit 0: got %d items, want 2", len(got))
	}
}

func TestExtractFencedTOML(t *testing.T) {
	input := "here\n```toml\n[feed.rule]\nitem = \".item\"\n```\nthere"
	out, err := extractFencedTOML(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "[feed.rule]") {
		t.Errorf("output missing [feed.rule], got %q", out)
	}
	if !strings.Contains(string(out), "item = \".item\"") {
		t.Errorf("output missing item selector, got %q", out)
	}
}

func TestExtractFencedTOMLRejectsMissingBlock(t *testing.T) {
	_, err := extractFencedTOML("no fence")
	if err == nil {
		t.Fatal("expected error for missing fenced block")
	}
}
package extract

import (
	"net/url"
	"testing"

	"github.com/LeeTeng2001/rssify/internal/config"
)

func TestRunExtractsCSSFieldsAndAbsoluteLinks(t *testing.T) {
	rule := compileRuleForTest(t, ".item", ".title", "a", "href", true, ".date", "Jan 2, 2006")
	baseURL, err := url.Parse("https://example.com/list")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	items, warnings, err := Run([]byte(`<html><body><div class="item"><a class="title" href="/p/1">One</a><span class="date">May 25, 2026</span></div></body></html>`), rule, baseURL)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	item := items[0]
	if item.Title != "One" {
		t.Fatalf("Title = %q, want One", item.Title)
	}
	if item.Link != "https://example.com/p/1" {
		t.Fatalf("Link = %q, want https://example.com/p/1", item.Link)
	}
	if item.PubDate != "Mon, 25 May 2026 00:00:00 +0000" {
		t.Fatalf("PubDate = %q, want Mon, 25 May 2026 00:00:00 +0000", item.PubDate)
	}
}

func TestRunReturnsWarningForInvalidDate(t *testing.T) {
	rule := compileRuleForTest(t, ".item", ".title", "a", "href", false, ".date", "Jan 2, 2006")
	baseURL, err := url.Parse("https://example.com/list")
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	items, warnings, err := Run([]byte(`<html><body><div class="item"><a class="title" href="/p/1">One</a><span class="date">today</span></div></body></html>`), rule, baseURL)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].PubDate != "" {
		t.Fatalf("PubDate = %q, want empty", items[0].PubDate)
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1: %#v", len(warnings), warnings)
	}
	if warnings[0].Field != "pub_date" {
		t.Fatalf("warning Field = %q, want pub_date", warnings[0].Field)
	}
}

func compileRuleForTest(t *testing.T, item, title, linkSelector, linkAttr string, linkAbsolute bool, dateSelector, dateFormat string) config.CompiledRule {
	t.Helper()
	itemSelector, err := config.CompileSelector(item)
	if err != nil {
		t.Fatalf("compile item selector: %v", err)
	}
	titleSelector, err := config.CompileSelector(title)
	if err != nil {
		t.Fatalf("compile title selector: %v", err)
	}
	link, err := config.CompileSelector(linkSelector)
	if err != nil {
		t.Fatalf("compile link selector: %v", err)
	}
	date, err := config.CompileSelector(dateSelector)
	if err != nil {
		t.Fatalf("compile date selector: %v", err)
	}

	pubDate := config.CompiledField{Selector: date, Format: dateFormat}
	return config.CompiledRule{
		Item:    itemSelector,
		Title:   config.CompiledField{Selector: titleSelector},
		Link:    config.CompiledField{Selector: link, Attr: linkAttr, Absolute: linkAbsolute},
		PubDate: &pubDate,
	}
}

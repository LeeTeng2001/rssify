package extract

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/LeeTeng2001/rssify/internal/config"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type Item struct {
	Title       string
	Link        string
	Description string
	PubDate     string
}

type Warning struct {
	ItemIndex int
	Field     string
	Message   string
}

func Run(data []byte, rule config.CompiledRule, baseURL *url.URL) ([]Item, []Warning, error) {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}

	itemNodes := rule.Item.Find(root)
	items := make([]Item, 0, len(itemNodes))
	var warnings []Warning
	for i, node := range itemNodes {
		item := Item{}
		item.Title = extractField(node, i, "title", rule.Title, baseURL, &warnings)
		item.Link = extractField(node, i, "link", rule.Link, baseURL, &warnings)
		if rule.Description != nil {
			item.Description = extractField(node, i, "description", *rule.Description, baseURL, &warnings)
		}
		if rule.PubDate != nil {
			rawDate := extractField(node, i, "pub_date", *rule.PubDate, baseURL, &warnings)
			item.PubDate = formatPubDate(rawDate, i, *rule.PubDate, &warnings)
		}
		items = append(items, item)
	}

	return items, warnings, nil
}

func extractField(node *html.Node, itemIndex int, fieldName string, field config.CompiledField, baseURL *url.URL, warnings *[]Warning) string {
	matches := field.Selector.Find(node)
	if len(matches) == 0 {
		addWarning(warnings, itemIndex, fieldName, "selector matched nothing")
		return ""
	}

	value := strings.TrimSpace(fieldValue(matches[0], field.Attr))
	if !field.Absolute || value == "" {
		return value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		addWarning(warnings, itemIndex, fieldName, fmt.Sprintf("invalid URL: %v", err))
		return ""
	}
	if baseURL != nil {
		parsed = baseURL.ResolveReference(parsed)
	}
	return parsed.String()
}

func fieldValue(node *html.Node, attr string) string {
	if attr != "" {
		for _, nodeAttr := range node.Attr {
			if nodeAttr.Key == attr {
				return nodeAttr.Val
			}
		}
		return ""
	}
	return goquery.NewDocumentFromNode(node).Text()
}

func formatPubDate(raw string, itemIndex int, field config.CompiledField, warnings *[]Warning) string {
	if field.Format == "" {
		return raw
	}
	if raw == "" {
		return ""
	}

	parsed, err := time.Parse(field.Format, raw)
	if err != nil {
		addWarning(warnings, itemIndex, "pub_date", fmt.Sprintf("parse date: %v", err))
		return ""
	}
	return parsed.UTC().Format(time.RFC1123Z)
}

func addWarning(warnings *[]Warning, itemIndex int, field, message string) {
	*warnings = append(*warnings, Warning{ItemIndex: itemIndex, Field: field, Message: message})
}

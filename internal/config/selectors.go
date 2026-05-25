package config

import (
	"fmt"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
	"golang.org/x/net/html"
)

type Selector interface {
	Find(node *html.Node) []*html.Node
}

type cssSelector struct {
	selector cascadia.Selector
}

func (s cssSelector) Find(node *html.Node) []*html.Node {
	return cascadia.QueryAll(node, s.selector)
}

type xpathSelector struct {
	expr *xpath.Expr
}

func (s xpathSelector) Find(node *html.Node) []*html.Node {
	return htmlquery.QuerySelectorAll(node, s.expr)
}

func CompileSelector(raw string) (Selector, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("selector is required")
	}

	if strings.HasPrefix(trimmed, "xpath:") {
		expr, err := xpath.Compile(strings.TrimPrefix(trimmed, "xpath:"))
		if err != nil {
			return nil, fmt.Errorf("compile xpath selector: %w", err)
		}
		return xpathSelector{expr: expr}, nil
	}

	selector, err := cascadia.Compile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("compile css selector: %w", err)
	}
	return cssSelector{selector: selector}, nil
}

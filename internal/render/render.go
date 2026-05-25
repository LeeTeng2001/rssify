package render

import (
	"bytes"
	"encoding/xml"
	"time"

	"github.com/LeeTeng2001/rssify/internal/extract"
)

type FeedMeta struct {
	Title       string
	Link        string
	Description string
	SelfURL     string
}

type rssRoot struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	AtomNS  string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	AtomLink      atomLink  `xml:"atom:link"`
	LastBuildDate string    `xml:"lastBuildDate"`
	Generator     string    `xml:"generator"`
	Items         []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	Description string  `xml:"description,omitempty"`
	PubDate     string  `xml:"pubDate,omitempty"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func RSS(meta FeedMeta, items []extract.Item, generated time.Time) []byte {
	rssItems := make([]rssItem, 0, len(items))
	for _, item := range items {
		rssItems = append(rssItems, rssItem{
			Title:       item.Title,
			Link:        item.Link,
			GUID:        rssGUID{IsPermaLink: "true", Value: item.Link},
			Description: item.Description,
			PubDate:     item.PubDate,
		})
	}

	root := rssRoot{
		Version: "2.0",
		AtomNS:  "http://www.w3.org/2005/Atom",
		Channel: rssChannel{
			Title:       meta.Title,
			Link:        meta.Link,
			Description: meta.Description,
			AtomLink: atomLink{
				Href: meta.SelfURL,
				Rel:  "self",
				Type: "application/rss+xml",
			},
			LastBuildDate: generated.UTC().Format(time.RFC1123Z),
			Generator:     "rssify",
			Items:         rssItems,
		},
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(root); err != nil {
		return nil
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

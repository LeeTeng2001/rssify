package server

import (
	"net/http"
	"strings"

	"github.com/LeeTeng2001/rssify/internal/cache"
)

func New(c *cache.Cache, feedIDs []string) http.Handler {
	known := make(map[string]bool, len(feedIDs))
	for _, id := range feedIDs {
		known[id] = true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/feed/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Path
		if !strings.HasSuffix(path, ".xml") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		id := strings.TrimPrefix(path, "/feed/")
		id = strings.TrimSuffix(id, ".xml")
		if id == "" || !known[id] {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			return
		}

		xml, ok := c.Get(id)
		if !ok {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("feed has not been scraped yet, try again shortly"))
			return
		}

		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(xml)
	})
	return mux
}
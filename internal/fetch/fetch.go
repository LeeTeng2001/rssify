package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	http      *http.Client
	userAgent string
}

type HTTPError struct {
	StatusCode int
	URL        string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("GET %s returned HTTP %d", e.URL, e.StatusCode)
}

func NewClient(userAgent string, timeout time.Duration) *Client {
	return &Client{
		http:      &http.Client{Timeout: timeout},
		userAgent: userAgent,
	}
}

func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, finalURL, HTTPError{StatusCode: resp.StatusCode, URL: finalURL.String()}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, finalURL, err
	}
	return body, finalURL, nil
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}
